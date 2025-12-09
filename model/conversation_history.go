package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ConversationHistory 存储正常的聊天历史记录
type ConversationHistory struct {
	Id               int    `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint"`
	ConversationId   string `json:"conversation_id" gorm:"type:varchar(255);index"` // 唯一对话标识符
	MessageId        string `json:"message_id" gorm:"type:varchar(255);index"`      // 消息标识符
	Role             string `json:"role" gorm:"type:varchar(20);index"`             // 角色：user, assistant, system
	Content          string `json:"content" gorm:"type:text"`                       // 消息内容
	ModelName        string `json:"model_name" gorm:"type:varchar(100);index"`      // 使用的AI模型
	TokenId          int    `json:"token_id" gorm:"index"`                          // 本次对话使用的令牌
	ChannelId        int    `json:"channel_id" gorm:"index"`                        // 使用的渠道
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`                 // 提示词令牌数
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`             // 补全令牌数
	TotalTokens      int    `json:"total_tokens" gorm:"default:0"`                  // 总令牌数
	IsStream         bool   `json:"is_stream" gorm:"default:false"`                 // 是否为流式输出
	FinishReason     string `json:"finish_reason" gorm:"type:varchar(50)"`          // 完成原因
	Usage            string `json:"usage" gorm:"type:text"`                         // 详细使用情况的JSON字符串
	Other            string `json:"other" gorm:"type:text"`                         // 其他元数据的JSON格式
	Ip               string `json:"ip" gorm:"type:varchar(45);index"`               // 客户端IP地址
}

// ErrorConversationHistory 存储导致错误的聊天历史记录
type ErrorConversationHistory struct {
	Id               int    `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
	ConversationId   string `json:"conversation_id" gorm:"type:varchar(255);index"` // 对话标识符
	MessageId        string `json:"message_id" gorm:"type:varchar(255);index"`      // 消息标识符
	Role             string `json:"role" gorm:"type:varchar(20);index"`             // 角色
	Content          string `json:"content" gorm:"type:text"`                       // 消息内容
	ModelName        string `json:"model_name" gorm:"type:varchar(100);index"`      // AI模型
	TokenId          int    `json:"token_id" gorm:"index"`                          // 令牌ID
	ChannelId        int    `json:"channel_id" gorm:"index"`                        // 渠道ID
	ErrorCode        int    `json:"error_code" gorm:"index"`                        // 错误代码
	ErrorMessage     string `json:"error_message" gorm:"type:text"`                 // 错误消息
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`                 // 提示词令牌数
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`             // 补全令牌数
	TotalTokens      int    `json:"total_tokens" gorm:"default:0"`                  // 总令牌数
	Other            string `json:"other" gorm:"type:text"`                         // 其他元数据
	Ip               string `json:"ip" gorm:"type:varchar(45);index"`               // 客户端IP
}

// MES数据库全局连接
var MES_DB *gorm.DB

// getConversationHistoryTableName 返回对话历史的表名
// 如果启用了日期分表，则返回基于日期的表名
func getConversationHistoryTableName(date ...time.Time) string {
	if !common.MESDailyPartition {
		return "conversation_histories"
	}

	var targetDate time.Time
	if len(date) > 0 {
		targetDate = date[0]
	} else {
		targetDate = time.Now()
	}

	return fmt.Sprintf("conversation_histories_%s", targetDate.Format("2006_01_02"))
}

// getErrorConversationHistoryTableName returns the table name for error conversation history
func getErrorConversationHistoryTableName(date ...time.Time) string {
	if !common.MESDailyPartition {
		return "error_conversation_histories"
	}

	var targetDate time.Time
	if len(date) > 0 {
		targetDate = date[0]
	} else {
		targetDate = time.Now()
	}

	return fmt.Sprintf("error_conversation_histories_%s", targetDate.Format("2006_01_02"))
}

// createTableIfNotExists creates the table if it doesn't exist (for daily partitioning)
func createTableIfNotExists(tableName string, model interface{}) error {
	if MES_DB == nil {
		return fmt.Errorf("MES database not initialized")
	}

	// Check if table exists
	if MES_DB.Migrator().HasTable(tableName) {
		return nil
	}

	// Create table based on model
	err := MES_DB.Table(tableName).AutoMigrate(model)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %v", tableName, err)
	}

	common.SysLog(fmt.Sprintf("Created MES table: %s", tableName))
	return nil
}

// SaveConversationHistory saves a conversation history record
func SaveConversationHistory(history *ConversationHistory) error {
	if MES_DB == nil {
		return fmt.Errorf("MES database not initialized")
	}

	history.CreatedAt = time.Now().Unix()
	history.UpdatedAt = history.CreatedAt

	tableName := getConversationHistoryTableName()

	// Create table if using daily partition
	if common.MESDailyPartition {
		err := createTableIfNotExists(tableName, &ConversationHistory{})
		if err != nil {
			return err
		}
	}

	return MES_DB.Table(tableName).Create(history).Error
}

// SaveErrorConversationHistory saves an error conversation history record
func SaveErrorConversationHistory(history *ErrorConversationHistory) error {
	if MES_DB == nil {
		return fmt.Errorf("MES database not initialized")
	}

	history.CreatedAt = time.Now().Unix()

	tableName := getErrorConversationHistoryTableName()

	// Create table if using daily partition
	if common.MESDailyPartition {
		err := createTableIfNotExists(tableName, &ErrorConversationHistory{})
		if err != nil {
			return err
		}
	}

	return MES_DB.Table(tableName).Create(history).Error
}

// GetConversationHistory retrieves conversation history by conversation ID
func GetConversationHistory(conversationId string, limit int, offset int) ([]*ConversationHistory, error) {
	if MES_DB == nil {
		return nil, fmt.Errorf("MES database not initialized")
	}

	var histories []*ConversationHistory
	var err error

	if common.MESDailyPartition {
		// Query across all possible tables
		err = queryAcrossPartitionedTables(conversationId, limit, offset, &histories, false)
	} else {
		// Query single table
		query := MES_DB.Where("conversation_id = ?", conversationId).
			Order("created_at DESC")

		if limit > 0 {
			query = query.Limit(limit)
		}
		if offset > 0 {
			query = query.Offset(offset)
		}

		err = query.Find(&histories).Error
	}

	return histories, err
}

// GetUserConversationHistory retrieves conversation history by user ID
func GetUserConversationHistory(userId int, limit int, offset int) ([]*ConversationHistory, error) {
	if MES_DB == nil {
		return nil, fmt.Errorf("MES database not initialized")
	}

	var histories []*ConversationHistory
	var err error

	if common.MESDailyPartition {
		// Query across all possible tables for user
		err = queryUserHistoryAcrossPartitions(userId, limit, offset, &histories)
	} else {
		// Query single table
		query := MES_DB.Where("user_id = ?", userId).
			Order("created_at DESC")

		if limit > 0 {
			query = query.Limit(limit)
		}
		if offset > 0 {
			query = query.Offset(offset)
		}

		err = query.Find(&histories).Error
	}

	return histories, err
}

// queryAcrossPartitionedTables queries conversation history across partitioned tables
func queryAcrossPartitionedTables(conversationId string, limit int, offset int, histories *[]*ConversationHistory, isError bool) error {
	// Get all existing partition tables
	var tablePrefix string
	if isError {
		tablePrefix = "error_conversation_histories_"
	} else {
		tablePrefix = "conversation_histories_"
	}

	tables, err := getExistingPartitionTables(tablePrefix)
	if err != nil {
		return err
	}

	// Query each table and combine results
	var allHistories []*ConversationHistory
	for _, tableName := range tables {
		var tableHistories []*ConversationHistory
		err := MES_DB.Table(tableName).
			Where("conversation_id = ?", conversationId).
			Order("created_at DESC").
			Find(&tableHistories).Error
		if err != nil {
			common.SysError(fmt.Sprintf("Error querying table %s: %v", tableName, err))
			continue
		}
		allHistories = append(allHistories, tableHistories...)
	}

	// Sort by created_at DESC
	if len(allHistories) > 1 {
		for i := 0; i < len(allHistories)-1; i++ {
			for j := i + 1; j < len(allHistories); j++ {
				if allHistories[i].CreatedAt < allHistories[j].CreatedAt {
					allHistories[i], allHistories[j] = allHistories[j], allHistories[i]
				}
			}
		}
	}

	// Apply limit and offset
	start := offset
	if start >= len(allHistories) {
		*histories = []*ConversationHistory{}
		return nil
	}

	end := start + limit
	if limit <= 0 || end > len(allHistories) {
		end = len(allHistories)
	}

	*histories = allHistories[start:end]
	return nil
}

// queryUserHistoryAcrossPartitions queries user's conversation history across partitioned tables
func queryUserHistoryAcrossPartitions(userId int, limit int, offset int, histories *[]*ConversationHistory) error {
	tables, err := getExistingPartitionTables("conversation_histories_")
	if err != nil {
		return err
	}

	// Query each table and combine results
	var allHistories []*ConversationHistory
	for _, tableName := range tables {
		var tableHistories []*ConversationHistory
		err := MES_DB.Table(tableName).
			Where("user_id = ?", userId).
			Order("created_at DESC").
			Find(&tableHistories).Error
		if err != nil {
			common.SysError(fmt.Sprintf("Error querying table %s: %v", tableName, err))
			continue
		}
		allHistories = append(allHistories, tableHistories...)
	}

	// Sort by created_at DESC
	if len(allHistories) > 1 {
		for i := 0; i < len(allHistories)-1; i++ {
			for j := i + 1; j < len(allHistories); j++ {
				if allHistories[i].CreatedAt < allHistories[j].CreatedAt {
					allHistories[i], allHistories[j] = allHistories[j], allHistories[i]
				}
			}
		}
	}

	// Apply limit and offset
	start := offset
	if start >= len(allHistories) {
		*histories = []*ConversationHistory{}
		return nil
	}

	end := start + limit
	if limit <= 0 || end > len(allHistories) {
		end = len(allHistories)
	}

	*histories = allHistories[start:end]
	return nil
}

// getExistingPartitionTables returns a list of existing partition tables with the given prefix
func getExistingPartitionTables(prefix string) ([]string, error) {
	var tables []string
	var rows []map[string]interface{}

	var query string
	switch common.MesSqlType {
	case common.DatabaseTypeMySQL:
		query = "SHOW TABLES LIKE ?"
		err := MES_DB.Raw(query, prefix+"%").Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			for _, value := range row {
				if tableName, ok := value.(string); ok && strings.HasPrefix(tableName, prefix) {
					tables = append(tables, tableName)
				}
			}
		}
	case common.DatabaseTypePostgreSQL:
		query = "SELECT tablename FROM pg_tables WHERE tablename LIKE $1 AND schemaname = 'public'"
		err := MES_DB.Raw(query, prefix+"%").Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if tableName, ok := row["tablename"].(string); ok {
				tables = append(tables, tableName)
			}
		}
	case common.DatabaseTypeSQLite:
		query = "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?"
		err := MES_DB.Raw(query, prefix+"%").Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if tableName, ok := row["name"].(string); ok {
				tables = append(tables, tableName)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", common.MesSqlType)
	}

	return tables, nil
}

// DeleteConversationHistory deletes conversation history by conversation ID
func DeleteConversationHistory(conversationId string) error {
	if MES_DB == nil {
		return fmt.Errorf("MES database not initialized")
	}

	if common.MESDailyPartition {
		// Delete from all partition tables
		tables, err := getExistingPartitionTables("conversation_histories_")
		if err != nil {
			return err
		}

		for _, tableName := range tables {
			err := MES_DB.Table(tableName).Where("conversation_id = ?", conversationId).Delete(&ConversationHistory{}).Error
			if err != nil {
				common.SysError(fmt.Sprintf("Error deleting from table %s: %v", tableName, err))
			}
		}
		return nil
	} else {
		return MES_DB.Where("conversation_id = ?", conversationId).Delete(&ConversationHistory{}).Error
	}
}

// CreateConversationFromMessages creates a conversation history from OpenAI messages format
func CreateConversationFromMessages(conversationId string, messages []map[string]interface{}, modelName string, userId int, tokenId int, channelId int, ip string) error {
	if MES_DB == nil {
		return fmt.Errorf("MES database not initialized")
	}

	for i, message := range messages {
		role, _ := message["role"].(string)
		content := ""

		// Handle different content types
		if contentStr, ok := message["content"].(string); ok {
			content = contentStr
		} else if contentArray, ok := message["content"].([]interface{}); ok {
			// Handle array content (like OpenAI format with images)
			contentBytes, _ := json.Marshal(contentArray)
			content = string(contentBytes)
		} else if contentObj, ok := message["content"].(map[string]interface{}); ok {
			// Handle object content
			contentBytes, _ := json.Marshal(contentObj)
			content = string(contentBytes)
		}

		history := &ConversationHistory{
			ConversationId: conversationId,
			MessageId:      fmt.Sprintf("%s_%d", conversationId, i),
			Role:           role,
			Content:        content,
			ModelName:      modelName,
			UserId:         userId,
			TokenId:        tokenId,
			ChannelId:      channelId,
			Ip:             ip,
		}

		// Add any additional metadata to Other field
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
			return fmt.Errorf("failed to save conversation history: %v", err)
		}
	}

	return nil
}
