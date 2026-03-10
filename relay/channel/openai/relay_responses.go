package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type responsesStreamEventRaw struct {
	Type     string          `json:"type"`
	Response json.RawMessage `json:"response,omitempty"`
}

func setUsageFromResponses(usage *dto.Usage, responsesResponse *dto.OpenAIResponsesResponse) {
	if usage == nil || responsesResponse == nil || responsesResponse.Usage == nil {
		return
	}
	if responsesResponse.Usage.InputTokens != 0 {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
	}
	if responsesResponse.Usage.OutputTokens != 0 {
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
	}
	if responsesResponse.Usage.TotalTokens != 0 {
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
	}
	if responsesResponse.Usage.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
	}
}

func countBuiltInToolsFromResponsesObject(c *gin.Context, info *relaycommon.RelayInfo, responsesResponse *dto.OpenAIResponsesResponse) {
	if c == nil || info == nil || responsesResponse == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return
	}
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
}

func countBuiltInToolFromStreamItem(info *relaycommon.RelayInfo, item *dto.ResponsesOutput) {
	if info == nil || item == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return
	}
	switch item.Type {
	case dto.BuildInCallWebSearchCall:
		if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
			webSearchTool.CallCount++
		}
	}
}

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	if mappedModel, ok := mappedResponseModel(info); ok {
		responsesResponse.Model = mappedModel
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(responseBody, &bodyMap); err == nil {
			bodyMap["model"] = mappedModel
			if jsonBytes, err := common.Marshal(bodyMap); err == nil {
				responseBody = jsonBytes
			}
		} else if jsonBytes, err := common.Marshal(responsesResponse); err == nil {
			responseBody = jsonBytes
		}
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	setUsageFromResponses(&usage, &responsesResponse)
	countBuiltInToolsFromResponsesObject(c, info, &responsesResponse)
	return &usage, nil
}

func OaiResponsesStreamToNonStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	usage := &dto.Usage{}
	var responseTextBuilder strings.Builder
	var completedResponseRaw []byte
	var streamErr *types.NewAPIError

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[DONE]") {
			break
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || strings.HasPrefix(data, "[DONE]") {
			if strings.HasPrefix(data, "[DONE]") {
				break
			}
			continue
		}

		info.SetFirstResponseTime()
		info.ReceivedResponseCount++

		var rawEvent responsesStreamEventRaw
		if err := common.UnmarshalJsonStr(data, &rawEvent); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream raw event: "+err.Error())
			continue
		}

		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream response event: "+err.Error())
			continue
		}

		switch streamResponse.Type {
		case "response.output_text.delta":
			responseTextBuilder.WriteString(streamResponse.Delta)
		case "response.completed":
			if len(rawEvent.Response) > 0 {
				completedResponseRaw = append(completedResponseRaw[:0], rawEvent.Response...)
			}
			if streamResponse.Response != nil {
				setUsageFromResponses(usage, streamResponse.Response)
				countBuiltInToolsFromResponsesObject(c, info, streamResponse.Response)
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.error", "response.failed":
			if streamResponse.Response != nil {
				if oaiError := streamResponse.Response.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
					streamErr = types.WithOpenAIError(*oaiError, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResponse.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}

		if streamErr != nil {
			break
		}
	}

	if scanErr := scanner.Err(); scanErr != nil && streamErr == nil {
		streamErr = types.NewOpenAIError(scanErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if len(completedResponseRaw) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("responses stream missing response.completed"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if mappedModel, ok := mappedResponseModel(info); ok {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(completedResponseRaw, &bodyMap); err == nil {
			bodyMap["model"] = mappedModel
			if jsonBytes, err := common.Marshal(bodyMap); err == nil {
				completedResponseRaw = jsonBytes
			}
		} else {
			var respObj dto.OpenAIResponsesResponse
			if err := common.Unmarshal(completedResponseRaw, &respObj); err == nil {
				respObj.Model = mappedModel
				if jsonBytes, err := common.Marshal(respObj); err == nil {
					completedResponseRaw = jsonBytes
				}
			}
		}
	}

	c.Data(http.StatusOK, "application/json", completedResponseRaw)

	if usage.CompletionTokens == 0 {
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			usage.CompletionTokens = service.CountTextToken(tempStr, info.UpstreamModelName)
		}
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err == nil {
			sendResponsesStreamData(c, info, streamResponse, data)
			switch streamResponse.Type {
			case "response.completed":
				if streamResponse.Response != nil {
					setUsageFromResponses(usage, streamResponse.Response)
					if streamResponse.Response.HasImageGenerationCall() {
						c.Set("image_generation_call", true)
						c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
						c.Set("image_generation_call_size", streamResponse.Response.GetSize())
					}
				}
			case "response.output_text.delta":
				// 处理输出文本
				responseTextBuilder.WriteString(streamResponse.Delta)
			case dto.ResponsesOutputTypeItemDone:
				countBuiltInToolFromStreamItem(info, streamResponse.Item)
			}
		} else {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		}
		return true
	})

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}
