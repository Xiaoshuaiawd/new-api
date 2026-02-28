package openai

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type chatCompletionsNonStreamResponse struct {
	ID                string                             `json:"id"`
	Object            string                             `json:"object"`
	Created           int64                              `json:"created"`
	Model             string                             `json:"model"`
	Choices           []chatCompletionsNonStreamChoice   `json:"choices"`
	Usage             chatCompletionsNonStreamUsage      `json:"usage"`
	ServiceTier       string                             `json:"service_tier"`
	SystemFingerprint *string                            `json:"system_fingerprint"`
}

type chatCompletionsNonStreamChoice struct {
	Index        int                             `json:"index"`
	Message      chatCompletionsNonStreamMessage `json:"message"`
	FinishReason string                          `json:"finish_reason"`
}

type chatCompletionsNonStreamMessage struct {
	Role        string                             `json:"role"`
	Content     string                             `json:"content"`
	Refusal     any                                `json:"refusal"`
	Annotations []any                              `json:"annotations"`
	ToolCalls   []chatCompletionsNonStreamToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionsNonStreamToolCall struct {
	ID       string                               `json:"id,omitempty"`
	Type     string                               `json:"type"`
	Function chatCompletionsNonStreamToolFunction `json:"function"`
}

type chatCompletionsNonStreamToolFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type chatCompletionsNonStreamUsage struct {
	PromptTokens            int                                             `json:"prompt_tokens"`
	CompletionTokens        int                                             `json:"completion_tokens"`
	TotalTokens             int                                             `json:"total_tokens"`
	PromptTokensDetails     chatCompletionsNonStreamPromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails chatCompletionsNonStreamCompletionTokensDetails `json:"completion_tokens_details"`
}

type chatCompletionsNonStreamPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

type chatCompletionsNonStreamCompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
}

func normalizeChatCompletionCreated(created any) int64 {
	switch v := created.(type) {
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return int64(^uint64(0) >> 1)
		}
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

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

	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		responseBody, err = marshalOpenAIChatCompletionsNonStreamResponse(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func marshalOpenAIChatCompletionsNonStreamResponse(chatResp *dto.OpenAITextResponse) ([]byte, error) {
	if chatResp == nil {
		return nil, fmt.Errorf("chat response is nil")
	}
	out := chatCompletionsNonStreamResponse{
		ID:                chatResp.Id,
		Object:            chatResp.Object,
		Created:           normalizeChatCompletionCreated(chatResp.Created),
		Model:             chatResp.Model,
		Choices:           make([]chatCompletionsNonStreamChoice, 0, len(chatResp.Choices)),
		ServiceTier:       "default",
		SystemFingerprint: nil,
		Usage: chatCompletionsNonStreamUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
			PromptTokensDetails: chatCompletionsNonStreamPromptTokensDetails{
				CachedTokens: chatResp.Usage.PromptTokensDetails.CachedTokens,
				AudioTokens:  chatResp.Usage.PromptTokensDetails.AudioTokens,
			},
			CompletionTokensDetails: chatCompletionsNonStreamCompletionTokensDetails{
				ReasoningTokens:          chatResp.Usage.CompletionTokenDetails.ReasoningTokens,
				AudioTokens:              chatResp.Usage.CompletionTokenDetails.AudioTokens,
				AcceptedPredictionTokens: 0,
				RejectedPredictionTokens: 0,
			},
		},
	}

	if out.Object == "" {
		out.Object = "chat.completion"
	}
	if out.Created == 0 {
		out.Created = time.Now().Unix()
	}

	for i := range chatResp.Choices {
		choice := chatResp.Choices[i]
		msg := choice.Message
		normalizedMessage := chatCompletionsNonStreamMessage{
			Role:        msg.Role,
			Content:     msg.StringContent(),
			Refusal:     nil,
			Annotations: make([]any, 0),
		}
		toolCalls := msg.ParseToolCalls()
		if len(toolCalls) > 0 {
			normalizedToolCalls := make([]chatCompletionsNonStreamToolCall, 0, len(toolCalls))
			for _, tc := range toolCalls {
				if tc.Type == "" {
					tc.Type = "function"
				}
				normalizedToolCalls = append(normalizedToolCalls, chatCompletionsNonStreamToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: chatCompletionsNonStreamToolFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
			normalizedMessage.ToolCalls = normalizedToolCalls
			normalizedMessage.Content = ""
		}
		out.Choices = append(out.Choices, chatCompletionsNonStreamChoice{
			Index:        choice.Index,
			Message:      normalizedMessage,
			FinishReason: choice.FinishReason,
		})
	}

	if len(out.Choices) == 0 {
		out.Choices = []chatCompletionsNonStreamChoice{
			{
				Index: 0,
				Message: chatCompletionsNonStreamMessage{
					Role:        "assistant",
					Content:     "",
					Refusal:     nil,
					Annotations: make([]any, 0),
				},
				FinishReason: "stop",
			},
		}
	}

	return common.Marshal(out)
}

func mergeUsageFromResponses(dst *dto.Usage, src *dto.Usage) {
	if dst == nil || src == nil {
		return
	}
	if src.InputTokens != 0 {
		dst.PromptTokens = src.InputTokens
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens != 0 {
		dst.CompletionTokens = src.OutputTokens
		dst.OutputTokens = src.OutputTokens
	}
	if src.TotalTokens != 0 {
		dst.TotalTokens = src.TotalTokens
	} else if dst.PromptTokens != 0 || dst.CompletionTokens != 0 {
		dst.TotalTokens = dst.PromptTokens + dst.CompletionTokens
	}
	if src.InputTokensDetails != nil {
		dst.PromptTokensDetails.CachedTokens = src.InputTokensDetails.CachedTokens
		dst.PromptTokensDetails.ImageTokens = src.InputTokensDetails.ImageTokens
		dst.PromptTokensDetails.AudioTokens = src.InputTokensDetails.AudioTokens
	}
	if src.CompletionTokenDetails.ReasoningTokens != 0 {
		dst.CompletionTokenDetails.ReasoningTokens = src.CompletionTokenDetails.ReasoningTokens
	}
}

func OaiResponsesStreamToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

	usage := &dto.Usage{}
	var outputText strings.Builder
	var usageText strings.Builder
	var streamErr *types.NewAPIError
	var completedResponse *dto.OpenAIResponsesResponse

	toolCallNameByID := make(map[string]string)
	toolCallArgsByID := make(map[string]string)
	toolCallCanonicalIDByItemID := make(map[string]string)
	toolCallOrder := make([]string, 0)
	toolCallSeen := make(map[string]struct{})

	appendToolCallOrder := func(callID string) {
		if callID == "" {
			return
		}
		if _, exists := toolCallSeen[callID]; exists {
			return
		}
		toolCallSeen[callID] = struct{}{}
		toolCallOrder = append(toolCallOrder, callID)
	}

	// Parse SSE in non-stream mode and aggregate into one chat completion payload.
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
		if data == "" {
			continue
		}
		if strings.HasPrefix(data, "[DONE]") {
			break
		}
		info.SetFirstResponseTime()
		info.ReceivedResponseCount++

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			continue
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
			}

		case "response.output_text.delta":
			if streamResp.Delta != "" {
				outputText.WriteString(streamResp.Delta)
				usageText.WriteString(streamResp.Delta)
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
			appendToolCallOrder(callID)

			name := strings.TrimSpace(streamResp.Item.Name)
			if name != "" {
				toolCallNameByID[callID] = name
				usageText.WriteString(name)
			}

			newArgs := streamResp.Item.Arguments
			if newArgs != "" {
				prevArgs := toolCallArgsByID[callID]
				if strings.HasPrefix(newArgs, prevArgs) {
					usageText.WriteString(newArgs[len(prevArgs):])
				} else {
					usageText.WriteString(newArgs)
				}
				toolCallArgsByID[callID] = newArgs
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
			appendToolCallOrder(callID)
			toolCallArgsByID[callID] += streamResp.Delta
			usageText.WriteString(streamResp.Delta)

		case "response.completed":
			if streamResp.Response != nil {
				completedResponse = streamResp.Response
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				mergeUsageFromResponses(usage, streamResp.Response.Usage)
			}

		case "response.error", "response.failed":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
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

	responsesResp := completedResponse
	if responsesResp == nil {
		responsesResp = &dto.OpenAIResponsesResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: int(createAt),
			Model:     model,
			Usage:     usage,
		}
	}
	if responsesResp.Model == "" {
		responsesResp.Model = model
	}
	if responsesResp.CreatedAt == 0 {
		responsesResp.CreatedAt = int(createAt)
	}
	if responsesResp.Usage == nil {
		responsesResp.Usage = usage
	}
	if len(responsesResp.Output) == 0 {
		text := outputText.String()
		if text != "" {
			responsesResp.Output = []dto.ResponsesOutput{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []dto.ResponsesOutputContent{
						{
							Type:        "output_text",
							Text:        text,
							Annotations: make([]interface{}, 0),
						},
					},
				},
			}
		} else if len(toolCallOrder) > 0 {
			outs := make([]dto.ResponsesOutput, 0, len(toolCallOrder))
			for i, callID := range toolCallOrder {
				callName := toolCallNameByID[callID]
				callArgs := toolCallArgsByID[callID]
				if callName == "" && callArgs == "" {
					continue
				}
				outs = append(outs, dto.ResponsesOutput{
					Type:      "function_call",
					ID:        fmt.Sprintf("call_%d", i),
					Status:    "completed",
					CallId:    callID,
					Name:      callName,
					Arguments: callArgs,
				})
			}
			responsesResp.Output = outs
		}
	}

	chatResp, finalUsage, err := service.ResponsesResponseToChatCompletionsResponse(responsesResp, responseID)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if finalUsage == nil {
		finalUsage = usage
	}
	if finalUsage == nil || finalUsage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		if text == "" {
			text = usageText.String()
		}
		finalUsage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *finalUsage
	}

	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		responseBody, err = marshalOpenAIChatCompletionsNonStreamResponse(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json", responseBody)
	return finalUsage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

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
	hasSentReasoningSummary := false
	needsReasoningSummarySeparator := false
	//reasoningSummaryTextByKey := make(map[string]string)

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendChatChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		if chunk == nil {
			return true
		}
		if info.RelayFormat == types.RelayFormatOpenAI {
			if err := helper.ObjectData(c, chunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		}

		chunkData, err := common.Marshal(chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := HandleStreamFormat(c, info, string(chunkData), false, false); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	sendStartIfNeeded := func() bool {
		if sentStart {
			return true
		}
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
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

	sendReasoningSummaryDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		if needsReasoningSummarySeparator {
			if strings.HasPrefix(delta, "\n\n") {
				needsReasoningSummarySeparator = false
			} else if strings.HasPrefix(delta, "\n") {
				delta = "\n" + delta
				needsReasoningSummarySeparator = false
			} else {
				delta = "\n\n" + delta
				needsReasoningSummarySeparator = false
			}
		}
		if !sendStartIfNeeded() {
			return false
		}

		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ReasoningContent: &delta,
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		hasSentReasoningSummary = true
		return true
	}

	sendToolCallDelta := func(callID string, name string, argsDelta string) bool {
		if callID == "" {
			return true
		}
		if outputText.Len() > 0 {
			// Prefer streaming assistant text over tool calls to match non-stream behavior.
			return true
		}
		if !sendStartIfNeeded() {
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
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tool},
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
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
			}

		//case "response.reasoning_text.delta":
		//if !sendReasoningDelta(streamResp.Delta) {
		//	return false
		//}

		//case "response.reasoning_text.done":

		case "response.reasoning_summary_text.delta":
			if !sendReasoningSummaryDelta(streamResp.Delta) {
				return false
			}

		case "response.reasoning_summary_text.done":
			if hasSentReasoningSummary {
				needsReasoningSummarySeparator = true
			}

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
			if !sendStartIfNeeded() {
				return false
			}

			if streamResp.Delta != "" {
				outputText.WriteString(streamResp.Delta)
				usageText.WriteString(streamResp.Delta)
				delta := streamResp.Delta
				chunk := &dto.ChatCompletionsStreamResponse{
					Id:      responseId,
					Object:  "chat.completion.chunk",
					Created: createAt,
					Model:   model,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
								Content: &delta,
							},
						},
					},
				}
				if !sendChatChunk(chunk) {
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

			if !sendToolCallDelta(callID, name, argsDelta) {
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
			if !sendToolCallDelta(callID, "", streamResp.Delta) {
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

			if !sendStartIfNeeded() {
				return false
			}
			if !sentStop {
				if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
					info.ClaudeConvertInfo.Usage = usage
				}
				finishReason := "stop"
				if sawToolCall && outputText.Len() == 0 {
					finishReason = "tool_calls"
				}
				stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
				if !sendChatChunk(stop) {
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
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return nil, streamErr
		}
	}
	if !sentStop {
		if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
			info.ClaudeConvertInfo.Usage = usage
		}
		finishReason := "stop"
		if sawToolCall && outputText.Len() == 0 {
			finishReason = "tool_calls"
		}
		stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
		if !sendChatChunk(stop) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
