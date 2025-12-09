package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

// MESHelper 为 MES（消息/对话历史）提供便捷的操作方法
type MESHelper struct{}

// NewMESHelper 创建一个新的 MES 辅助器实例
func NewMESHelper() *MESHelper {
	return &MESHelper{}
}

// SaveChatCompletion 将完整的聊天对话上下文保存到 MES 数据库
func (h *MESHelper) SaveChatCompletion(c *gin.Context, conversationId string, messages []map[string]interface{},
	response map[string]interface{}, modelName string, userId int, tokenId int, channelId int) error {

	if !common.MESEnabled {
		return nil // MES 未启用，跳过保存
	}

	ip := c.ClientIP()

	// 构建完整的对话上下文
	fullConversation := make([]map[string]interface{}, 0, len(messages)+1)

	// 添加所有输入消息
	fullConversation = append(fullConversation, messages...)

	// 添加AI响应（如果有的话）
	if response != nil {
		assistantMessage := h.buildAssistantMessage(response)
		if assistantMessage != nil {
			fullConversation = append(fullConversation, assistantMessage)
		}
	}

	// 构建要保存的消息结构
	conversationContent := map[string]interface{}{
		"messages": fullConversation,
	}

	// 将完整对话序列化为JSON
	contentJSON, err := json.Marshal(conversationContent)
	if err != nil {
		return fmt.Errorf("序列化对话内容失败: %v", err)
	}

	// 计算token使用情况
	var promptTokens, completionTokens, totalTokens int
	if response != nil {
		if usage, ok := response["usage"].(map[string]interface{}); ok {
			promptTokens = h.getIntFromInterface(usage["prompt_tokens"])
			completionTokens = h.getIntFromInterface(usage["completion_tokens"])
			totalTokens = h.getIntFromInterface(usage["total_tokens"])
		}
	}

	// 保存为单个对话记录
	history := &ConversationHistory{
		ConversationId:   conversationId,
		MessageId:        conversationId + "_complete", // 完整对话的消息ID
		UserId:           userId,
		CreatedAt:        common.GetTimestamp(),
		UpdatedAt:        common.GetTimestamp(),
		Role:             "conversation",      // 标识这是完整对话
		Content:          string(contentJSON), // 完整的JSON对话内容
		ModelName:        modelName,
		TokenId:          tokenId,
		ChannelId:        channelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		IsStream:         false, // 后续会根据实际情况更新
		Ip:               ip,
	}

	// 添加额外的元数据到Other字段
	otherData := map[string]interface{}{
		"message_count": len(fullConversation),
		"request_time":  common.GetTimestamp(),
	}
	if response != nil {
		otherData["response_id"] = response["id"]
		otherData["response_model"] = response["model"]
		if created, ok := response["created"]; ok {
			otherData["response_created"] = created
		}
		otherData["response_raw"] = response
	}

	otherBytes, _ := json.Marshal(otherData)
	history.Other = string(otherBytes)

	// 保存到数据库
	err = h.saveConversationHistory(history)
	if err != nil {
		return fmt.Errorf("保存完整对话失败: %v", err)
	}

	return nil
}

// SaveFullConversation 保存完整的对话到 MES 数据库（新方法）
func (h *MESHelper) SaveFullConversation(c *gin.Context, conversationId string, fullConversation []map[string]interface{},
	response *dto.OpenAITextResponse, modelName string, userId int, tokenId int, channelId int) error {

	if !common.MESEnabled {
		return nil // MES 未启用，跳过保存
	}

	ip := c.ClientIP()

	// 构建要保存的消息结构
	conversationContent := map[string]interface{}{
		"messages": fullConversation,
	}

	// 将完整对话序列化为JSON
	contentJSON, err := json.Marshal(conversationContent)
	if err != nil {
		return fmt.Errorf("序列化完整对话内容失败: %v", err)
	}

	// 计算token使用情况
	var promptTokens, completionTokens, totalTokens int
	if response != nil && response.Usage.TotalTokens > 0 {
		promptTokens = response.Usage.PromptTokens
		completionTokens = response.Usage.CompletionTokens
		totalTokens = response.Usage.TotalTokens
	}

	// 保存为单个对话记录
	history := &ConversationHistory{
		ConversationId:   conversationId,
		MessageId:        conversationId + "_full", // 完整对话的消息ID
		UserId:           userId,
		CreatedAt:        common.GetTimestamp(),
		UpdatedAt:        common.GetTimestamp(),
		Role:             "conversation",      // 标识这是完整对话
		Content:          string(contentJSON), // 完整的JSON对话内容
		ModelName:        modelName,
		TokenId:          tokenId,
		ChannelId:        channelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		IsStream:         false,
		Ip:               ip,
	}

	// 添加额外的元数据到Other字段
	otherData := map[string]interface{}{
		"message_count": len(fullConversation),
		"request_time":  common.GetTimestamp(),
	}
	if response != nil {
		otherData["response_id"] = response.Id
		otherData["response_model"] = response.Model
		otherData["response_created"] = response.Created
	}

	otherBytes, _ := json.Marshal(otherData)
	history.Other = string(otherBytes)

	// 保存到数据库
	err = h.saveConversationHistory(history)
	if err != nil {
		return fmt.Errorf("保存完整对话失败: %v", err)
	}

	return nil
}

// SaveErrorConversation 保存导致错误的完整对话上下文
func (h *MESHelper) SaveErrorConversation(c *gin.Context, conversationId string, messages []map[string]interface{},
	errorCode int, errorMessage string, modelName string, userId int, tokenId int, channelId int) error {

	if !common.MESEnabled {
		return nil
	}

	ip := c.ClientIP()

	// 构建完整的对话上下文（只包含用户消息，因为出错了没有AI响应）
	conversationContent := map[string]interface{}{
		"messages": messages,
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": errorMessage,
		},
	}

	// 将完整对话序列化为JSON
	contentJSON, err := json.Marshal(conversationContent)
	if err != nil {
		return fmt.Errorf("序列化错误对话内容失败: %v", err)
	}

	// 保存为单个错误对话记录
	errorHistory := &ErrorConversationHistory{
		ConversationId: conversationId,
		MessageId:      conversationId + "_error", // 错误对话的消息ID
		UserId:         userId,
		CreatedAt:      common.GetTimestamp(),
		Role:           "error_conversation", // 标识这是错误的完整对话
		Content:        string(contentJSON),  // 完整的JSON对话内容
		ModelName:      modelName,
		TokenId:        tokenId,
		ChannelId:      channelId,
		ErrorCode:      errorCode,
		ErrorMessage:   errorMessage,
		Ip:             ip,
	}

	// 添加额外的元数据到Other字段
	otherData := map[string]interface{}{
		"message_count": len(messages),
		"error_time":    common.GetTimestamp(),
		"error_type":    "api_error",
	}

	otherBytes, _ := json.Marshal(otherData)
	errorHistory.Other = string(otherBytes)

	return SaveErrorConversationHistory(errorHistory)
}

// GetConversationMessages retrieves conversation messages in OpenAI format
func (h *MESHelper) GetConversationMessages(conversationId string, limit int) ([]map[string]interface{}, error) {
	if !common.MESEnabled {
		return nil, fmt.Errorf("MES not enabled")
	}

	histories, err := GetConversationHistory(conversationId, limit, 0)
	if err != nil {
		return nil, err
	}

	messages := make([]map[string]interface{}, 0, len(histories))
	for _, history := range histories {
		message := map[string]interface{}{
			"role": history.Role,
		}

		// Try to parse content as JSON, if it fails, use as string
		var content interface{}
		if err := json.Unmarshal([]byte(history.Content), &content); err != nil {
			message["content"] = history.Content
		} else {
			message["content"] = content
		}

		// Add other metadata if present
		if history.Other != "" {
			var otherData map[string]interface{}
			if err := json.Unmarshal([]byte(history.Other), &otherData); err == nil {
				for key, value := range otherData {
					message[key] = value
				}
			}
		}

		messages = append(messages, message)
	}

	// Reverse to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// GetUserConversations retrieves a user's conversation list
func (h *MESHelper) GetUserConversations(userId int, limit int, offset int) ([]*ConversationHistory, error) {
	if !common.MESEnabled {
		return nil, fmt.Errorf("MES not enabled")
	}

	return GetUserConversationHistory(userId, limit, offset)
}

// DeleteUserConversation deletes a conversation (only if it belongs to the user)
func (h *MESHelper) DeleteUserConversation(userId int, conversationId string) error {
	if !common.MESEnabled {
		return fmt.Errorf("MES not enabled")
	}

	// First check if the conversation belongs to the user
	histories, err := GetConversationHistory(conversationId, 1, 0)
	if err != nil {
		return err
	}

	if len(histories) == 0 {
		return fmt.Errorf("conversation not found")
	}

	if histories[0].UserId != userId {
		return fmt.Errorf("permission denied: conversation does not belong to user")
	}

	return DeleteConversationHistory(conversationId)
}

// GetConversationStats returns statistics about conversations
func (h *MESHelper) GetConversationStats(userId int) (map[string]interface{}, error) {
	if !common.MESEnabled {
		return nil, fmt.Errorf("MES not enabled")
	}

	stats := map[string]interface{}{
		"total_conversations": 0,
		"total_messages":      0,
		"total_tokens":        0,
		"models_used":         make(map[string]int),
		"daily_message_count": make(map[string]int),
	}

	// Get user conversations (limit to recent ones for performance)
	histories, err := GetUserConversationHistory(userId, 1000, 0)
	if err != nil {
		return stats, err
	}

	conversationSet := make(map[string]bool)
	modelsUsed := make(map[string]int)
	dailyCount := make(map[string]int)
	totalTokens := 0

	for _, history := range histories {
		conversationSet[history.ConversationId] = true
		modelsUsed[history.ModelName]++
		totalTokens += history.TotalTokens

		// Count by date
		date := time.Unix(history.CreatedAt, 0).Format("2006-01-02")
		dailyCount[date]++
	}

	stats["total_conversations"] = len(conversationSet)
	stats["total_messages"] = len(histories)
	stats["total_tokens"] = totalTokens
	stats["models_used"] = modelsUsed
	stats["daily_message_count"] = dailyCount

	return stats, nil
}

// extractContent extracts content from various content formats
func (h *MESHelper) extractContent(content interface{}) string {
	if contentStr, ok := content.(string); ok {
		return contentStr
	}

	// Handle array or object content by converting to JSON
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprintf("%v", content)
	}
	return string(contentBytes)
}

// saveAssistantResponse saves the assistant's response to conversation history
func (h *MESHelper) saveAssistantResponse(conversationId string, response map[string]interface{},
	modelName string, userId int, tokenId int, channelId int, ip string) error {

	// Extract response content
	var content string
	var finishReason string
	var usage map[string]interface{}

	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				content = h.extractContent(message["content"])
			}
			if reason, ok := choice["finish_reason"].(string); ok {
				finishReason = reason
			}
		}
	}

	if usageData, ok := response["usage"].(map[string]interface{}); ok {
		usage = usageData
	}

	history := &ConversationHistory{
		ConversationId: conversationId,
		MessageId:      fmt.Sprintf("%s_assistant_%d", conversationId, time.Now().Unix()),
		Role:           "assistant",
		Content:        content,
		ModelName:      modelName,
		UserId:         userId,
		TokenId:        tokenId,
		ChannelId:      channelId,
		FinishReason:   finishReason,
		Ip:             ip,
	}

	// Extract token usage
	if usage != nil {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			history.PromptTokens = int(promptTokens)
		}
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			history.CompletionTokens = int(completionTokens)
		}
		if totalTokens, ok := usage["total_tokens"].(float64); ok {
			history.TotalTokens = int(totalTokens)
		}

		usageBytes, _ := json.Marshal(usage)
		history.Usage = string(usageBytes)
	}

	// Add other metadata
	otherData := make(map[string]interface{})
	for key, value := range response {
		if key != "choices" && key != "usage" {
			otherData[key] = value
		}
	}
	if len(otherData) > 0 {
		otherBytes, _ := json.Marshal(otherData)
		history.Other = string(otherBytes)
	}

	return SaveConversationHistory(history)
}

// GetGlobalMESHelper returns a global instance of MES helper
var globalMESHelper *MESHelper

func GetMESHelper() *MESHelper {
	if globalMESHelper == nil {
		globalMESHelper = NewMESHelper()
	}
	return globalMESHelper
}

// buildAssistantMessage 从响应数据构建助手消息
func (h *MESHelper) buildAssistantMessage(response map[string]interface{}) map[string]interface{} {
	if response == nil {
		return nil
	}

	assistantMessage := map[string]interface{}{
		"role": "assistant",
	}

	// 调试日志
	if common.DebugEnabled {
		responseJSON, _ := json.Marshal(response)
		common.SysLog("MES调试: 原始响应数据 = " + string(responseJSON))
	}

	// 从choices中提取内容
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("MES调试: 找到 %d 个choices", len(choices)))
		}

		if firstChoice, ok := choices[0].(map[string]interface{}); ok {
			if common.DebugEnabled {
				choiceJSON, _ := json.Marshal(firstChoice)
				common.SysLog("MES调试: 第一个choice = " + string(choiceJSON))
			}

			if message, ok := firstChoice["message"].(map[string]interface{}); ok {
				// 提取内容
				if content, exists := message["content"]; exists {
					assistantMessage["content"] = content
					if common.DebugEnabled {
						common.SysLog("MES调试: 提取到assistant content = " + fmt.Sprintf("%v", content))
					}
				}

				// 提取工具调用（如果有）
				if toolCalls, exists := message["tool_calls"]; exists {
					assistantMessage["tool_calls"] = toolCalls
				}

				// 提取其他字段
				for key, value := range message {
					if key != "role" && key != "content" && key != "tool_calls" {
						assistantMessage[key] = value
					}
				}
			} else {
				if common.DebugEnabled {
					common.SysLog("MES调试: choices[0]中没有找到message字段")
				}
			}
		}
	} else {
		if common.DebugEnabled {
			common.SysLog("MES调试: 没有找到choices字段或choices为空")
		}
	}

	// Claude 原生: content 数组或 completion 字段
	if _, hasContent := assistantMessage["content"]; !hasContent {
		if contents, ok := response["content"].([]interface{}); ok && len(contents) > 0 {
			var sb strings.Builder
			for _, item := range contents {
				if m, ok := item.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						sb.WriteString(text)
					}
					if thinking, ok := m["thinking"].(string); ok {
						sb.WriteString(thinking)
					}
				}
			}
			if sb.Len() > 0 {
				assistantMessage["content"] = sb.String()
			}
		} else if completion, ok := response["completion"].(string); ok && completion != "" {
			assistantMessage["content"] = completion
		}
	}

	// Gemini 原生: candidates[].content.parts[].text
	if _, hasContent := assistantMessage["content"]; !hasContent {
		if candidates, ok := response["candidates"].([]interface{}); ok && len(candidates) > 0 {
			if first, ok := candidates[0].(map[string]interface{}); ok {
				if content, ok := first["content"].(map[string]interface{}); ok {
					if parts, ok := content["parts"].([]interface{}); ok {
						var sb strings.Builder
						for _, p := range parts {
							if pm, ok := p.(map[string]interface{}); ok {
								if text, ok := pm["text"].(string); ok {
									sb.WriteString(text)
								}
							}
						}
						if sb.Len() > 0 {
							assistantMessage["content"] = sb.String()
						}
					}
				}
			}
		}
	}

	// 如果没有content字段，尝试直接从response中获取
	if _, hasContent := assistantMessage["content"]; !hasContent {
		if content, exists := response["content"]; exists {
			assistantMessage["content"] = content
		}

		if common.DebugEnabled {
			common.SysLog("MES调试: 最终assistant消息 = " + fmt.Sprintf("%v", assistantMessage))
		}
	}

	return assistantMessage
}

// getIntFromInterface 安全地从interface{}中获取int值
func (h *MESHelper) getIntFromInterface(val interface{}) int {
	if val == nil {
		return 0
	}

	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

// saveConversationHistory 保存对话历史到数据库
func (h *MESHelper) saveConversationHistory(history *ConversationHistory) error {
	// 创建表（如果需要的话）
	tableName := getConversationHistoryTableName()
	err := createTableIfNotExists(tableName, &ConversationHistory{})
	if err != nil {
		return err
	}

	// 保存到数据库
	return MES_DB.Table(tableName).Create(history).Error
}
