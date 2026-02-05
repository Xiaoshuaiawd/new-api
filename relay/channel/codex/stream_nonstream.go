package codex

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func responsesStreamToNonStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	var (
		usage               = &dto.Usage{}
		outputFromCompleted []dto.ResponsesOutput
		outputByIndex       = make(map[int]dto.ResponsesOutput)
		outputNoIndex       = make([]dto.ResponsesOutput, 0)
		outputText          strings.Builder
		lastMessageID       string
		lastMessageRole     string
	)

	scanner := bufio.NewScanner(resp.Body)
	maxBuf := helper.DefaultMaxScannerBufferSize
	if constant.StreamScannerMaxBufferMB > 0 {
		maxBuf = constant.StreamScannerMaxBufferMB << 20
	}
	scanner.Buffer(make([]byte, helper.InitialScannerBufferSize), maxBuf)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 {
			continue
		}
		if !strings.HasPrefix(line, "data:") && !strings.HasPrefix(line, "[DONE]") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		data = strings.TrimSuffix(data, "\r")
		if strings.HasPrefix(data, "[DONE]") {
			break
		}
		if data == "" {
			continue
		}

		if info != nil {
			info.SetFirstResponseTime()
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			continue
		}

		switch streamResp.Type {
		case "response.completed":
			if streamResp.Response != nil {
				if oaiError := streamResp.Response.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
					return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
				}
				outputFromCompleted = streamResp.Response.Output
				if streamResp.Response.Usage != nil {
					usage.PromptTokens = streamResp.Response.Usage.InputTokens
					usage.CompletionTokens = streamResp.Response.Usage.OutputTokens
					usage.TotalTokens = streamResp.Response.Usage.TotalTokens
					if streamResp.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResp.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				if streamResp.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResp.Response.GetQuality())
					c.Set("image_generation_call_size", streamResp.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			outputText.WriteString(streamResp.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResp.Item == nil {
				break
			}
			if streamResp.Item.Type == "message" {
				if streamResp.Item.ID != "" {
					lastMessageID = streamResp.Item.ID
				}
				if streamResp.Item.Role != "" {
					lastMessageRole = streamResp.Item.Role
				}
			}
			if streamResp.OutputIndex != nil {
				outputByIndex[*streamResp.OutputIndex] = *streamResp.Item
			} else {
				outputNoIndex = append(outputNoIndex, *streamResp.Item)
			}
			if streamResp.Item.Type == dto.BuildInCallWebSearchCall {
				if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
					if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
						webSearchTool.CallCount++
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	finalOutput := outputFromCompleted
	if len(finalOutput) == 0 && (len(outputByIndex) > 0 || len(outputNoIndex) > 0) {
		indices := make([]int, 0, len(outputByIndex))
		for idx := range outputByIndex {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			finalOutput = append(finalOutput, outputByIndex[idx])
		}
		finalOutput = append(finalOutput, outputNoIndex...)
	}

	if len(finalOutput) == 0 && outputText.Len() > 0 {
		role := "assistant"
		if lastMessageRole != "" {
			role = lastMessageRole
		}
		id := lastMessageID
		if id == "" {
			id = "msg_" + common.GetUUID()
		}
		finalOutput = []dto.ResponsesOutput{
			{
				Type: "message",
				ID:   id,
				Role: role,
				Content: []dto.ResponsesOutputContent{
					{
						Type:        "output_text",
						Text:        outputText.String(),
						Annotations: []interface{}{},
					},
				},
			},
		}
	}

	finalOutput = filterMessageOutputs(finalOutput)

	jsonData, err := common.Marshal(finalOutput)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonData)

	if usage.CompletionTokens == 0 && outputText.Len() > 0 {
		usage.CompletionTokens = service.CountTextToken(outputText.String(), info.UpstreamModelName)
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}
	if usage.TotalTokens == 0 && (usage.PromptTokens != 0 || usage.CompletionTokens != 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage, nil
}

func filterMessageOutputs(outputs []dto.ResponsesOutput) []dto.ResponsesOutput {
	if len(outputs) == 0 {
		return outputs
	}
	messages := make([]dto.ResponsesOutput, 0, len(outputs))
	for _, item := range outputs {
		if item.Type == "message" {
			messages = append(messages, item)
		}
	}
	if len(messages) > 0 {
		return messages
	}
	return outputs
}
