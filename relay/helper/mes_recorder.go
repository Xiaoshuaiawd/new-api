package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// GenerateConversationID builds a conversation id based on request id and current time.
func GenerateConversationID(c *gin.Context) string {
	requestId := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("conv_%s_%s", requestId, common.GetTimeString())
}

// ConvertOpenAIMessagesToMES converts OpenAI compatible messages to MES format.
func ConvertOpenAIMessagesToMES(messages []dto.Message) []map[string]interface{} {
	var mesMessages []map[string]interface{}

	for _, message := range messages {
		mesMessage := map[string]interface{}{
			"role":    message.Role,
			"content": message.StringContent(),
		}

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

// ConvertClaudeMessagesToMES converts Claude request messages (including system prompt) to MES format.
func ConvertClaudeMessagesToMES(request *dto.ClaudeRequest) []map[string]interface{} {
	if request == nil {
		return nil
	}

	messages := make([]map[string]interface{}, 0, len(request.Messages)+1)

	if request.System != nil {
		systemContent := extractClaudeContent(request.System)
		if systemContent != "" {
			messages = append(messages, map[string]interface{}{
				"role":    "system",
				"content": systemContent,
			})
		}
	}

	for _, msg := range request.Messages {
		content := msg.GetStringContent()
		if content == "" && msg.Content != nil {
			if raw, err := common.Marshal(msg.Content); err == nil {
				content = string(raw)
			}
		}
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": content,
		})
	}

	return messages
}

func extractClaudeContent(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []dto.ClaudeMediaMessage:
		builder := strings.Builder{}
		for _, item := range v {
			builder.WriteString(item.GetStringContent())
		}
		return builder.String()
	default:
		if raw, err := common.Marshal(v); err == nil {
			return string(raw)
		}
	}
	return ""
}

// ConvertGeminiMessagesToMES converts Gemini request messages to MES format.
func ConvertGeminiMessagesToMES(request *dto.GeminiChatRequest) []map[string]interface{} {
	if request == nil {
		return nil
	}

	messages := make([]map[string]interface{}, 0, len(request.Contents)+1)

	if request.SystemInstructions != nil && len(request.SystemInstructions.Parts) > 0 {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": stringifyGeminiParts(request.SystemInstructions.Parts),
		})
	}

	for _, content := range request.Contents {
		role := content.Role
		if role == "" {
			role = "user"
		}

		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": stringifyGeminiParts(content.Parts),
		})
	}

	return messages
}

// ConvertImageRequestToMES converts image generation request to MES format.
func ConvertImageRequestToMES(request *dto.ImageRequest) []map[string]interface{} {
	if request == nil {
		return nil
	}

	content := map[string]interface{}{
		"prompt": request.Prompt,
	}
	if request.Model != "" {
		content["model"] = request.Model
	}
	if request.Size != "" {
		content["size"] = request.Size
	}
	if request.Quality != "" {
		content["quality"] = request.Quality
	}
	if request.N > 0 {
		content["n"] = request.N
	}

	return []map[string]interface{}{
		{
			"role":    "user",
			"content": content,
		},
	}
}

func stringifyGeminiParts(parts []dto.GeminiPart) string {
	chunks := make([]string, 0, len(parts))
	for _, part := range parts {
		switch {
		case part.Text != "":
			chunks = append(chunks, part.Text)
		case part.InlineData != nil:
			chunks = append(chunks, fmt.Sprintf("[inline %s]", part.InlineData.MimeType))
		case part.FunctionCall != nil:
			if raw, err := common.Marshal(part.FunctionCall); err == nil {
				chunks = append(chunks, string(raw))
			}
		case part.FunctionResponse != nil:
			if raw, err := common.Marshal(part.FunctionResponse); err == nil {
				chunks = append(chunks, string(raw))
			}
		case part.FileData != nil:
			if part.FileData.FileUri != "" {
				chunks = append(chunks, fmt.Sprintf("[file %s]", part.FileData.FileUri))
			}
		case part.ExecutableCode != nil:
			if part.ExecutableCode.Code != "" {
				chunks = append(chunks, part.ExecutableCode.Code)
			}
		case part.CodeExecutionResult != nil:
			if raw, err := common.Marshal(part.CodeExecutionResult); err == nil {
				chunks = append(chunks, string(raw))
			}
		}
	}
	return strings.Join(chunks, "\n")
}

// BuildAssistantMessageFromResponse extracts the first assistant message from an OpenAI text response.
func BuildAssistantMessageFromResponse(response *dto.OpenAITextResponse) map[string]interface{} {
	if response == nil || len(response.Choices) == 0 {
		return nil
	}

	choice := response.Choices[0]
	assistant := map[string]interface{}{
		"role":    "assistant",
		"content": choice.Message.StringContent(),
	}

	if len(choice.Message.ToolCalls) > 0 {
		assistant["tool_calls"] = choice.Message.ToolCalls
	}
	if choice.FinishReason != "" {
		assistant["finish_reason"] = choice.FinishReason
	}

	return assistant
}

// BuildStreamTextResponse builds a minimal OpenAITextResponse for stream aggregation so MES can persist it.
func BuildStreamTextResponse(responseText string, usage *dto.Usage, responseId string, createdAt int64, modelName string) *dto.OpenAITextResponse {
	resp := &dto.OpenAITextResponse{
		Id:      responseId,
		Object:  "chat.completion",
		Created: createdAt,
		Model:   modelName,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: responseText,
				},
				FinishReason: "stop",
			},
		},
	}
	if usage != nil {
		resp.Usage = *usage
	}
	return resp
}

// GetMESMessagesFromContext parses the original request body back into a list of MES-friendly messages.
func GetMESMessagesFromContext(c *gin.Context, info *relaycommon.RelayInfo) ([]map[string]interface{}, error) {
	if info == nil || info.Request == nil {
		return nil, fmt.Errorf("request not available for MES")
	}

	body, err := common.GetRequestBody(c)
	if err != nil {
		return nil, err
	}

	switch info.Request.(type) {
	case *dto.GeneralOpenAIRequest:
		var req dto.GeneralOpenAIRequest
		if err := common.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		return ConvertOpenAIMessagesToMES(req.Messages), nil
	case *dto.ClaudeRequest:
		var req dto.ClaudeRequest
		if err := common.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		return ConvertClaudeMessagesToMES(&req), nil
	case *dto.GeminiChatRequest:
		var req dto.GeminiChatRequest
		if err := common.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		return ConvertGeminiMessagesToMES(&req), nil
	case *dto.ImageRequest:
		var req dto.ImageRequest
		if err := common.Unmarshal(body, &req); err != nil {
			// multipart/form-data 请求无法直接反序列化时，记录占位内容
			return []map[string]interface{}{
				{
					"role":    "user",
					"content": "[binary image request]",
				},
			}, nil
		}
		return ConvertImageRequestToMES(&req), nil
	default:
		return nil, fmt.Errorf("unsupported request type %T for MES", info.Request)
	}
}

// SaveMESWithTextResponseAsync saves a chat response (OpenAI text format) to MES asynchronously.
func SaveMESWithTextResponseAsync(c *gin.Context, info *relaycommon.RelayInfo, response *dto.OpenAITextResponse) {
	if !common.MESEnabled || response == nil {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError(fmt.Sprintf("MES记录出现panic: %v", r))
			}
		}()

		messages, err := GetMESMessagesFromContext(c, info)
		if err != nil {
			common.SysError("MES: 解析请求失败: " + err.Error())
			return
		}

		fullConversation := make([]map[string]interface{}, 0, len(messages)+1)
		fullConversation = append(fullConversation, messages...)

		if assistant := BuildAssistantMessageFromResponse(response); assistant != nil {
			fullConversation = append(fullConversation, assistant)
		}

		conversationId := GenerateConversationID(c)
		mesHelper := model.GetMESHelper()
		if err := mesHelper.SaveFullConversation(c, conversationId, fullConversation, response, info.OriginModelName, info.UserId, info.TokenId, info.ChannelId); err != nil {
			common.SysError("MES: 保存聊天补全失败: " + err.Error())
			return
		}
		common.SysLog("MES: 成功保存聊天补全, 对话ID: " + conversationId)
	}()
}

// SaveMESWithGenericResponseAsync saves non-text responses (e.g., images) to MES asynchronously.
func SaveMESWithGenericResponseAsync(c *gin.Context, info *relaycommon.RelayInfo, response map[string]interface{}) {
	if !common.MESEnabled || response == nil {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError(fmt.Sprintf("MES记录出现panic: %v", r))
			}
		}()

		messages, err := GetMESMessagesFromContext(c, info)
		if err != nil {
			common.SysError("MES: 解析请求失败: " + err.Error())
			return
		}

		conversationId := GenerateConversationID(c)
		mesHelper := model.GetMESHelper()
		if err := mesHelper.SaveChatCompletion(c, conversationId, messages, response, info.OriginModelName, info.UserId, info.TokenId, info.ChannelId); err != nil {
			common.SysError("MES: 保存对话失败: " + err.Error())
			return
		}
		common.SysLog("MES: 成功保存聊天记录, 对话ID: " + conversationId)
	}()
}
