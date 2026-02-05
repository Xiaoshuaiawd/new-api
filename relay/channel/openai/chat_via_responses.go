package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

func responsesStreamIndexKey(itemID string, idx *int) string {
	if itemID == "" {
		return ""
	}
	if idx == nil {
		return itemID
	}
	return fmt.Sprintf("%s:%d", itemID, *idx)
}

func stringDeltaFromPrefix(prev string, next string) string {
	if next == "" {
		return ""
	}
	if prev != "" && strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	return next
}

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	if isResponsesStream(resp) {
		return responsesStreamToChatNonStreamHandler(c, info, resp)
	}

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(&responsesResp, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	chatBody, err := common.Marshal(chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, chatBody)
	return usage, nil
}

func isResponsesStream(resp *http.Response) bool {
	if resp == nil || resp.Body == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "text/event-stream") {
		return true
	}

	reader := bufio.NewReader(resp.Body)
	peek, err := reader.Peek(64)
	if err == nil || err == io.EOF {
		trimmed := strings.TrimLeft(string(peek), " \r\n\t")
		if strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
			resp.Body = io.NopCloser(reader)
			return true
		}
	}
	resp.Body = io.NopCloser(reader)
	return false
}

func responsesStreamToChatNonStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	var (
		usage                = &dto.Usage{}
		completedResponse    *dto.OpenAIResponsesResponse
		completedResponseRaw string
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

		if streamResp.Type == "response.completed" && streamResp.Response != nil {
			completedResponse = streamResp.Response
			if raw := gjson.Get(data, "response"); raw.Exists() && raw.Type == gjson.JSON {
				completedResponseRaw = raw.Raw
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if completedResponse == nil && completedResponseRaw == "" {
		return nil, types.NewOpenAIError(fmt.Errorf("responses stream missing completed event"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if completedResponse == nil && completedResponseRaw != "" {
		if err := common.UnmarshalJsonStr(completedResponseRaw, &completedResponse); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	if completedResponse != nil {
		if oaiError := completedResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
			return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
		}
	}

	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(completedResponse, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(completedResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	chatBody, err := common.Marshal(chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, chatBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName
	serviceTier := ""
	lastObfuscation := ""

	var (
		usage       = &dto.Usage{}
		outputText  strings.Builder
		usageText   strings.Builder
		sentStart   bool
		sentStop    bool
		sawToolCall bool
		streamErr   *types.NewAPIError
	)

	toolCallIndexByID := make(map[string]int)
	toolCallNameByID := make(map[string]string)
	toolCallArgsByID := make(map[string]string)
	toolCallNameSent := make(map[string]bool)
	toolCallCanonicalIDByItemID := make(map[string]string)
	//reasoningSummaryTextByKey := make(map[string]string)

	sendStartIfNeeded := func(obfuscation string) bool {
		if sentStart {
			return true
		}
		start := helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)
		if serviceTier != "" {
			start.ServiceTier = serviceTier
		}
		if obfuscation != "" {
			start.Obfuscation = obfuscation
		}
		if len(start.Choices) > 0 {
			start.Choices[0].Delta.Refusal = json.RawMessage("null")
		}
		if err := helper.ObjectData(c, start); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		sentStart = true
		return true
	}

	//sendReasoningDelta := func(delta string) bool {
	//	if delta == "" {
	//		return true
	//	}
	//	if !sendStartIfNeeded() {
	//		return false
	//	}
	//
	//	usageText.WriteString(delta)
	//	chunk := &dto.ChatCompletionsStreamResponse{
	//		Id:      responseId,
	//		Object:  "chat.completion.chunk",
	//		Created: createAt,
	//		Model:   model,
	//		Choices: []dto.ChatCompletionsStreamResponseChoice{
	//			{
	//				Index: 0,
	//				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
	//					ReasoningContent: &delta,
	//				},
	//			},
	//		},
	//	}
	//	if err := helper.ObjectData(c, chunk); err != nil {
	//		streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	//		return false
	//	}
	//	return true
	//}

	sendReasoningSummaryDelta := func(delta string, obfuscation string) bool {
		if delta == "" {
			return true
		}
		if !sendStartIfNeeded(obfuscation) {
			return false
		}

		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:          responseId,
			Object:      "chat.completion.chunk",
			Created:     createAt,
			Model:       model,
			ServiceTier: serviceTier,
			Obfuscation: obfuscation,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ReasoningContent: &delta,
					},
				},
			},
		}
		if err := helper.ObjectData(c, chunk); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	sendToolCallDelta := func(callID string, name string, argsDelta string, obfuscation string) bool {
		if callID == "" {
			return true
		}
		if outputText.Len() > 0 {
			// Prefer streaming assistant text over tool calls to match non-stream behavior.
			return true
		}
		if !sendStartIfNeeded(obfuscation) {
			return false
		}

		idx, ok := toolCallIndexByID[callID]
		if !ok {
			idx = len(toolCallIndexByID)
			toolCallIndexByID[callID] = idx
		}
		if name != "" {
			toolCallNameByID[callID] = name
		}
		if toolCallNameByID[callID] != "" {
			name = toolCallNameByID[callID]
		}

		tool := dto.ToolCallResponse{
			ID:   callID,
			Type: "function",
			Function: dto.FunctionResponse{
				Arguments: argsDelta,
			},
		}
		tool.SetIndex(idx)
		if name != "" && !toolCallNameSent[callID] {
			tool.Function.Name = name
			toolCallNameSent[callID] = true
		}

		chunk := &dto.ChatCompletionsStreamResponse{
			Id:          responseId,
			Object:      "chat.completion.chunk",
			Created:     createAt,
			Model:       model,
			ServiceTier: serviceTier,
			Obfuscation: obfuscation,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tool},
					},
				},
			},
		}
		if err := helper.ObjectData(c, chunk); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		sawToolCall = true

		// Include tool call data in the local builder for fallback token estimation.
		if tool.Function.Name != "" {
			usageText.WriteString(tool.Function.Name)
		}
		if argsDelta != "" {
			usageText.WriteString(argsDelta)
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if streamErr != nil {
			return false
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			return true
		}

		switch streamResp.Type {
		case "response.created":
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				if streamResp.Response.ServiceTier != "" {
					serviceTier = streamResp.Response.ServiceTier
				}
			}

		//case "response.reasoning_text.delta":
		//if !sendReasoningDelta(streamResp.Delta) {
		//	return false
		//}

		//case "response.reasoning_text.done":

		case "response.reasoning_summary_text.delta":
			if streamResp.Obfuscation != "" {
				lastObfuscation = streamResp.Obfuscation
			}
			if !sendReasoningSummaryDelta(streamResp.Delta, streamResp.Obfuscation) {
				return false
			}

		case "response.reasoning_summary_text.done":

		//case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		//	key := responsesStreamIndexKey(strings.TrimSpace(streamResp.ItemID), streamResp.SummaryIndex)
		//	if key == "" || streamResp.Part == nil {
		//		break
		//	}
		//	// Only handle summary text parts, ignore other part types.
		//	if streamResp.Part.Type != "" && streamResp.Part.Type != "summary_text" {
		//		break
		//	}
		//	prev := reasoningSummaryTextByKey[key]
		//	next := streamResp.Part.Text
		//	delta := stringDeltaFromPrefix(prev, next)
		//	reasoningSummaryTextByKey[key] = next
		//	if !sendReasoningSummaryDelta(delta) {
		//		return false
		//	}

		case "response.output_text.delta":
			if streamResp.Obfuscation != "" {
				lastObfuscation = streamResp.Obfuscation
			}
			if !sendStartIfNeeded(streamResp.Obfuscation) {
				return false
			}

			if streamResp.Delta != "" {
				outputText.WriteString(streamResp.Delta)
				usageText.WriteString(streamResp.Delta)
				delta := streamResp.Delta
				chunk := &dto.ChatCompletionsStreamResponse{
					Id:          responseId,
					Object:      "chat.completion.chunk",
					Created:     createAt,
					Model:       model,
					ServiceTier: serviceTier,
					Obfuscation: streamResp.Obfuscation,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
								Content: &delta,
							},
						},
					},
				}
				if err := helper.ObjectData(c, chunk); err != nil {
					streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
					return false
				}
			}

		case "response.output_item.added", "response.output_item.done":
			if streamResp.Item == nil {
				break
			}
			if streamResp.Item.Type != "function_call" {
				break
			}

			itemID := strings.TrimSpace(streamResp.Item.ID)
			callID := strings.TrimSpace(streamResp.Item.CallId)
			if callID == "" {
				callID = itemID
			}
			if itemID != "" && callID != "" {
				toolCallCanonicalIDByItemID[itemID] = callID
			}
			name := strings.TrimSpace(streamResp.Item.Name)
			if name != "" {
				toolCallNameByID[callID] = name
			}

			newArgs := streamResp.Item.Arguments
			prevArgs := toolCallArgsByID[callID]
			argsDelta := ""
			if newArgs != "" {
				if strings.HasPrefix(newArgs, prevArgs) {
					argsDelta = newArgs[len(prevArgs):]
				} else {
					argsDelta = newArgs
				}
				toolCallArgsByID[callID] = newArgs
			}

			if !sendToolCallDelta(callID, name, argsDelta, streamResp.Obfuscation) {
				return false
			}

		case "response.function_call_arguments.delta":
			itemID := strings.TrimSpace(streamResp.ItemID)
			callID := toolCallCanonicalIDByItemID[itemID]
			if callID == "" {
				callID = itemID
			}
			if callID == "" {
				break
			}
			toolCallArgsByID[callID] += streamResp.Delta
			if !sendToolCallDelta(callID, "", streamResp.Delta, streamResp.Obfuscation) {
				return false
			}

		case "response.function_call_arguments.done":

		case "response.completed":
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				if streamResp.Response.ServiceTier != "" {
					serviceTier = streamResp.Response.ServiceTier
				}
				if streamResp.Response.Usage != nil {
					if streamResp.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResp.Response.Usage.InputTokens
						usage.InputTokens = streamResp.Response.Usage.InputTokens
					}
					if streamResp.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResp.Response.Usage.OutputTokens
						usage.OutputTokens = streamResp.Response.Usage.OutputTokens
					}
					if streamResp.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResp.Response.Usage.TotalTokens
					} else {
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					}
					if streamResp.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResp.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.ImageTokens = streamResp.Response.Usage.InputTokensDetails.ImageTokens
						usage.PromptTokensDetails.AudioTokens = streamResp.Response.Usage.InputTokensDetails.AudioTokens
					}
					if streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
						usage.CompletionTokenDetails.ReasoningTokens = streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens
					}
				}
			}

			if !sendStartIfNeeded(lastObfuscation) {
				return false
			}
			if !sentStop {
				finishReason := "stop"
				if sawToolCall && outputText.Len() == 0 {
					finishReason = "tool_calls"
				}
				stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
				if serviceTier != "" {
					stop.ServiceTier = serviceTier
				}
				if lastObfuscation != "" {
					stop.Obfuscation = lastObfuscation
				}
				if err := helper.ObjectData(c, stop); err != nil {
					streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
					return false
				}
				sentStop = true
			}

		case "response.error", "response.failed":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					return false
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false

		default:
		}

		return true
	})

	if streamErr != nil {
		return nil, streamErr
	}

	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, usageText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	if !sentStart {
		start := helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)
		if serviceTier != "" {
			start.ServiceTier = serviceTier
		}
		if lastObfuscation != "" {
			start.Obfuscation = lastObfuscation
		}
		if len(start.Choices) > 0 {
			start.Choices[0].Delta.Refusal = json.RawMessage("null")
		}
		if err := helper.ObjectData(c, start); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	if !sentStop {
		finishReason := "stop"
		if sawToolCall && outputText.Len() == 0 {
			finishReason = "tool_calls"
		}
		stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
		if serviceTier != "" {
			stop.ServiceTier = serviceTier
		}
		if lastObfuscation != "" {
			stop.Obfuscation = lastObfuscation
		}
		if err := helper.ObjectData(c, stop); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	if info.ShouldIncludeUsage && usage != nil {
		finalUsage := helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)
		if serviceTier != "" {
			finalUsage.ServiceTier = serviceTier
		}
		if lastObfuscation != "" {
			finalUsage.Obfuscation = lastObfuscation
		}
		if err := helper.ObjectData(c, finalUsage); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	helper.Done(c)
	return usage, nil
}
