package openai

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"os"
	"path/filepath"
	"strings"

	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return helper.ObjectData(c, lastStreamResponse)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		common.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer common.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var streamItems []string // store stream items
	var forceFormat bool
	var thinkToContent bool

	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	if info.ChannelSetting.ThinkingToContent {
		thinkToContent = true
	}

	var (
		lastStreamData string
	)

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if lastStreamData != "" {
			err := handleStreamFormat(c, info, lastStreamData, forceFormat, thinkToContent)
			if err != nil {
				common.SysError("error handling stream format: " + err.Error())
			}
		}
		lastStreamData = data
		streamItems = append(streamItems, data)
		return true
	})

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		common.SysError("error handling last response: " + err.Error())
	}

	if shouldSendLastResp && info.RelayFormat == relaycommon.RelayFormatOpenAI {
		_ = sendStreamData(c, info, lastStreamData, forceFormat, thinkToContent)
	}

	// 处理token计算
	if err := processTokens(info.RelayMode, streamItems, &responseTextBuilder, &toolCount); err != nil {
		common.SysError("error processing tokens: " + err.Error())
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(responseTextBuilder.String(), info.UpstreamModelName, info.PromptTokens)
		usage.CompletionTokens += toolCount * 7
	} else {
		if info.ChannelType == constant.ChannelTypeDeepSeek {
			if usage.PromptCacheHitTokens != 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	}

	handleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	// 保存流式聊天历史到 MES 数据库
	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError("MES流式记录出现panic: " + fmt.Sprintf("%v", r))
			}
		}()
		if info.RelayMode == relayconstant.RelayModeChatCompletions {
			saveStreamChatCompletionToMES(c, info, responseTextBuilder.String(), usage, responseId, createAt, model)
		}
	}()

	return usage, nil
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer common.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if simpleResponse.Error != nil && simpleResponse.Error.Type != "" {
		return nil, types.WithOpenAIError(*simpleResponse.Error, resp.StatusCode)
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	if simpleResponse.Usage.TotalTokens == 0 || (simpleResponse.Usage.PromptTokens == 0 && simpleResponse.Usage.CompletionTokens == 0) {
		completionTokens := 0
		for _, choice := range simpleResponse.Choices {
			ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.ReasoningContent+choice.Message.Reasoning, info.UpstreamModelName)
			completionTokens += ctkm
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.PromptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      info.PromptTokens + completionTokens,
		}
	}

	switch info.RelayFormat {
	case relaycommon.RelayFormatOpenAI:
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case relaycommon.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(&simpleResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	}

	// 保存聊天历史到 MES 数据库
	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError("MES记录出现panic: " + fmt.Sprintf("%v", r))
			}
		}()
		if info.RelayMode == relayconstant.RelayModeChatCompletions {
			saveChatCompletionToMES(c, info, &simpleResponse)
		}
	}()

	common.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func OpenaiTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) *dto.Usage {
	// the status code has been judged before, if there is a body reading failure,
	// it should be regarded as a non-recoverable error, so it should not return err for external retry.
	// Analogous to nginx's load balancing, it will only retry if it can't be requested or
	// if the upstream returns a specific status code, once the upstream has already written the header,
	// the subsequent failure of the response body should be regarded as a non-recoverable error,
	// and can be terminated directly.
	defer common.CloseResponseBodyGracefully(resp)
	usage := &dto.Usage{}
	usage.PromptTokens = info.PromptTokens
	usage.TotalTokens = info.PromptTokens
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		common.LogError(c, err.Error())
	}
	return usage
}

func OpenaiSTTHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*types.NewAPIError, *dto.Usage) {
	defer common.CloseResponseBodyGracefully(resp)

	// count tokens by audio file duration
	audioTokens, err := countAudioTokens(c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed), nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed), nil
	}
	// 写入新的 response body
	common.IOCopyBytesGracefully(c, resp, responseBody)

	usage := &dto.Usage{}
	usage.PromptTokens = audioTokens
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}

func countAudioTokens(c *gin.Context) (int, error) {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var reqBody struct {
		File *multipart.FileHeader `form:"file" binding:"required"`
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err = c.ShouldBind(&reqBody); err != nil {
		return 0, errors.WithStack(err)
	}
	ext := filepath.Ext(reqBody.File.Filename) // 获取文件扩展名
	reqFp, err := reqBody.File.Open()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer reqFp.Close()

	tmpFp, err := os.CreateTemp("", "audio-*"+ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer os.Remove(tmpFp.Name())

	_, err = io.Copy(tmpFp, reqFp)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if err = tmpFp.Close(); err != nil {
		return 0, errors.WithStack(err)
	}

	duration, err := common.GetAudioDuration(c.Request.Context(), tmpFp.Name(), ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(math.Round(math.Ceil(duration) / 60.0 * 1000)), nil // 1 minute 相当于 1k tokens
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken

				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						err := preConsumeUsage(c, info, usage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						usage = &dto.RealtimeUsage{}

						localUsage = &dto.RealtimeUsage{}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						err = preConsumeUsage(c, info, localUsage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						localUsage = &dto.RealtimeUsage{}
						// print now usage
					}
					common.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
					common.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
					common.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					common.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		common.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
	}

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	// clear usage
	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}

func OpenaiHandlerWithUsage(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer common.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	// 写入新的 response body
	common.IOCopyBytesGracefully(c, resp, responseBody)

	// Once we've written to the client, we should not return errors anymore
	// because the upstream has already consumed resources and returned content
	// We should still perform billing even if parsing fails
	// format
	if usageResp.InputTokens > 0 {
		usageResp.PromptTokens += usageResp.InputTokens
	}
	if usageResp.OutputTokens > 0 {
		usageResp.CompletionTokens += usageResp.OutputTokens
	}
	if usageResp.InputTokensDetails != nil {
		usageResp.PromptTokensDetails.ImageTokens += usageResp.InputTokensDetails.ImageTokens
		usageResp.PromptTokensDetails.TextTokens += usageResp.InputTokensDetails.TextTokens
	}
	return &usageResp.Usage, nil
}

// saveChatCompletionToMES 保存聊天补全到MES数据库
func saveChatCompletionToMES(c *gin.Context, info *relaycommon.RelayInfo, response *dto.OpenAITextResponse) {
	// 检查是否启用MES
	if !common.MESEnabled {
		return
	}

	// 获取原始请求体
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		common.SysError("MES: 获取请求体失败: " + err.Error())
		return
	}

	// 解析请求
	var chatRequest dto.GeneralOpenAIRequest
	if err := common.Unmarshal(requestBody, &chatRequest); err != nil {
		common.SysError("MES: 解析请求失败: " + err.Error())
		return
	}

	// 生成对话ID - 使用请求ID + 时间戳
	requestId := c.GetString(common.RequestIdKey)
	conversationId := generateConversationId(requestId, &chatRequest)

	// 转换消息格式
	messages := convertDTOMessagesToMESFormat(chatRequest.Messages)

	// 构建更健壮的助手响应消息
	var assistantMessage map[string]interface{}
	if len(response.Choices) > 0 {
		choice := response.Choices[0]
		assistantMessage = map[string]interface{}{
			"role":    "assistant",
			"content": choice.Message.StringContent(),
		}

		// 添加其他字段（如果有的话）
		if len(choice.Message.ToolCalls) > 0 {
			assistantMessage["tool_calls"] = choice.Message.ToolCalls
		}

		if choice.FinishReason != "" {
			assistantMessage["finish_reason"] = choice.FinishReason
		}
	}

	// 构建完整的对话
	fullConversation := make([]map[string]interface{}, 0, len(messages)+1)
	fullConversation = append(fullConversation, messages...)

	if assistantMessage != nil {
		fullConversation = append(fullConversation, assistantMessage)
	}

	// 调试日志
	if common.DebugEnabled {
		conversationJSON, _ := common.Marshal(fullConversation)
		common.SysLog("MES调试: 完整对话 = " + string(conversationJSON))
	}

	// 保存到MES
	mesHelper := model.GetMESHelper()
	err = mesHelper.SaveFullConversation(
		c,
		conversationId,
		fullConversation,
		response,
		info.OriginModelName,
		info.UserId,
		info.TokenId,
		info.ChannelId,
	)

	if err != nil {
		common.SysError("MES: 保存聊天补全失败: " + err.Error())
	} else {
		common.SysLog("MES: 成功保存聊天补全, 对话ID: " + conversationId)
	}
}

// generateConversationId 生成对话ID
func generateConversationId(requestId string, request *dto.GeneralOpenAIRequest) string {
	// 暂时简化，直接生成新的对话ID
	// TODO: 未来可以从请求中提取自定义的conversation_id
	return "conv_" + requestId + "_" + common.GetTimeString()
}

// convertMessagesToMESFormat 转换消息格式为MES格式
func convertMessagesToMESFormat(messages []map[string]interface{}) []map[string]interface{} {
	var mesMessages []map[string]interface{}

	for _, message := range messages {
		mesMessage := make(map[string]interface{})

		// 复制基本字段
		for key, value := range message {
			mesMessage[key] = value
		}

		mesMessages = append(mesMessages, mesMessage)
	}

	return mesMessages
}

// convertDTOMessagesToMESFormat 转换DTO消息格式为MES格式
func convertDTOMessagesToMESFormat(messages []dto.Message) []map[string]interface{} {
	var mesMessages []map[string]interface{}

	for _, message := range messages {
		mesMessage := make(map[string]interface{})

		// 转换基本字段
		mesMessage["role"] = message.Role
		mesMessage["content"] = message.StringContent()

		// 添加其他字段（如果存在）
		if message.Name != nil && *message.Name != "" {
			mesMessage["name"] = *message.Name
		}

		if len(message.ToolCalls) > 0 {
			mesMessage["tool_calls"] = message.ToolCalls
		}

		if message.ToolCallId != "" {
			mesMessage["tool_call_id"] = message.ToolCallId
		}

		mesMessages = append(mesMessages, mesMessage)
	}

	return mesMessages
}

// buildMESResponseData 构建MES响应数据
func buildMESResponseData(response *dto.OpenAITextResponse) map[string]interface{} {
	responseData := make(map[string]interface{})

	// 基本响应信息
	responseData["id"] = response.Id
	responseData["object"] = response.Object
	responseData["created"] = response.Created
	responseData["model"] = response.Model

	// 选择项
	responseData["choices"] = response.Choices

	// 使用情况
	if response.Usage.TotalTokens > 0 {
		responseData["usage"] = map[string]interface{}{
			"prompt_tokens":     response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
			"total_tokens":      response.Usage.TotalTokens,
		}
	}

	// 系统指纹 (如果OpenAITextResponse有这个字段的话)
	// Note: dto.OpenAITextResponse可能没有SystemFingerprint字段，所以暂时注释掉
	// if response.SystemFingerprint != "" {
	//	responseData["system_fingerprint"] = response.SystemFingerprint
	// }

	return responseData
}

// saveStreamChatCompletionToMES 保存流式聊天补全到MES数据库
func saveStreamChatCompletionToMES(c *gin.Context, info *relaycommon.RelayInfo, responseText string, usage *dto.Usage, responseId string, createAt int64, modelName string) {
	// 检查是否启用MES
	if !common.MESEnabled {
		return
	}

	// 获取原始请求体
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		common.SysError("MES流式: 获取请求体失败: " + err.Error())
		return
	}

	// 解析请求
	var chatRequest dto.GeneralOpenAIRequest
	if err := common.Unmarshal(requestBody, &chatRequest); err != nil {
		common.SysError("MES流式: 解析请求失败: " + err.Error())
		return
	}

	// 生成对话ID
	requestId := c.GetString(common.RequestIdKey)
	conversationId := generateConversationId(requestId, &chatRequest)

	// 转换消息格式
	messages := convertDTOMessagesToMESFormat(chatRequest.Messages)

	// 构建助手响应消息
	assistantMessage := map[string]interface{}{
		"role":    "assistant",
		"content": responseText,
	}

	// 构建完整的对话
	fullConversation := make([]map[string]interface{}, 0, len(messages)+1)
	fullConversation = append(fullConversation, messages...)
	fullConversation = append(fullConversation, assistantMessage)

	// 构建假的response对象用于传递usage信息
	var fakeResponse *dto.OpenAITextResponse
	if usage != nil {
		fakeResponse = &dto.OpenAITextResponse{
			Id:      responseId,
			Model:   modelName,
			Created: createAt,
			Usage:   *usage,
		}
	}

	// 获取MES辅助器并保存
	mesHelper := model.GetMESHelper()
	err = mesHelper.SaveFullConversation(
		c,
		conversationId,
		fullConversation,
		fakeResponse,
		info.OriginModelName,
		info.UserId,
		info.TokenId,
		info.ChannelId,
	)

	if err != nil {
		common.SysError("MES流式: 保存聊天补全失败: " + err.Error())
	} else {
		common.SysLog("MES流式: 成功保存聊天补全, 对话ID: " + conversationId)
	}
}

// buildStreamMESResponseData 构建流式MES响应数据
func buildStreamMESResponseData(responseText string, usage *dto.Usage, responseId string, createAt int64, modelName string) map[string]interface{} {
	responseData := make(map[string]interface{})

	// 基本响应信息
	responseData["id"] = responseId
	responseData["object"] = "chat.completion"
	responseData["created"] = createAt
	responseData["model"] = modelName

	// 构建选择项（从流式输出重建）
	choices := []map[string]interface{}{
		{
			"index": 0,
			"message": map[string]interface{}{
				"role":    "assistant",
				"content": responseText,
			},
			"finish_reason": "stop",
		},
	}
	responseData["choices"] = choices

	// 使用情况
	if usage != nil && usage.TotalTokens > 0 {
		responseData["usage"] = map[string]interface{}{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}

	return responseData
}
