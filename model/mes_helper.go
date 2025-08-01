package model

import (
	"encoding/json"
	"fmt"
	"one-api/common"
	"time"

	"github.com/gin-gonic/gin"
)

// MESHelper 为 MES（消息/对话历史）提供便捷的操作方法
type MESHelper struct{}

// NewMESHelper 创建一个新的 MES 辅助器实例
func NewMESHelper() *MESHelper {
	return &MESHelper{}
}

// SaveChatCompletion 将聊天补全对话保存到 MES 数据库
func (h *MESHelper) SaveChatCompletion(c *gin.Context, conversationId string, messages []map[string]interface{},
	response map[string]interface{}, modelName string, userId int, tokenId int, channelId int) error {

	if !common.MESEnabled {
		return nil // MES 未启用，跳过保存
	}

	ip := c.ClientIP()

	// 保存用户消息
	for i, message := range messages {
		role, _ := message["role"].(string)
		content := h.extractContent(message["content"])

		history := &ConversationHistory{
			ConversationId: conversationId,
			MessageId:      fmt.Sprintf("%s_user_%d", conversationId, i),
			Role:           role,
			Content:        content,
			ModelName:      modelName,
			UserId:         userId,
			TokenId:        tokenId,
			ChannelId:      channelId,
			Ip:             ip,
		}

		// Add metadata
		otherData := make(map[string]interface{})
		for key, value := range message {
			if key != "role" && key != "content" {
				otherData[key] = value
			}
		}
		if len(otherData) > 0 {
			otherBytes, _ := json.Marshal(otherData)
			history.Other = string(otherBytes)
		}

		err := SaveConversationHistory(history)
		if err != nil {
			common.SysError(fmt.Sprintf("Failed to save user message: %v", err))
			// Continue to save other messages
		}
	}

	// Save assistant response
	if response != nil {
		err := h.saveAssistantResponse(conversationId, response, modelName, userId, tokenId, channelId, ip)
		if err != nil {
			common.SysError(fmt.Sprintf("Failed to save assistant response: %v", err))
		}
	}

	return nil
}

// SaveErrorConversation saves a conversation that resulted in an error
func (h *MESHelper) SaveErrorConversation(c *gin.Context, conversationId string, messages []map[string]interface{},
	errorCode int, errorMessage string, modelName string, userId int, tokenId int, channelId int) error {

	if !common.MESEnabled {
		return nil
	}

	ip := c.ClientIP()

	// Save the last user message that caused the error
	if len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		role, _ := lastMessage["role"].(string)
		content := h.extractContent(lastMessage["content"])

		errorHistory := &ErrorConversationHistory{
			ConversationId: conversationId,
			MessageId:      fmt.Sprintf("%s_error_%d", conversationId, time.Now().Unix()),
			Role:           role,
			Content:        content,
			ModelName:      modelName,
			UserId:         userId,
			TokenId:        tokenId,
			ChannelId:      channelId,
			ErrorCode:      errorCode,
			ErrorMessage:   errorMessage,
			Ip:             ip,
		}

		// Add metadata
		otherData := make(map[string]interface{})
		for key, value := range lastMessage {
			if key != "role" && key != "content" {
				otherData[key] = value
			}
		}
		if len(otherData) > 0 {
			otherBytes, _ := json.Marshal(otherData)
			errorHistory.Other = string(otherBytes)
		}

		return SaveErrorConversationHistory(errorHistory)
	}

	return nil
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
