package model

import (
	"context"
	"fmt"
	"one-api/common"
	"one-api/logger"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	Other            string `json:"other"`
	// 价格显示字段 (不存储到数据库，仅用于API返回)
	InputPriceDisplay   string `json:"input_price_display" gorm:"-"`
	OutputPriceDisplay  string `json:"output_price_display" gorm:"-"`
	InputAmountDisplay  string `json:"input_amount_display" gorm:"-"`
	OutputAmountDisplay string `json:"output_amount_display" gorm:"-"`
}

const (
	LogTypeUnknown = iota
	LogTypeTopup
	LogTypeConsume
	LogTypeManage
	LogTypeSystem
	LogTypeError
)

// calculatePriceFields 计算并设置价格显示字段
func calculatePriceFields(log *Log) {
	// 默认倍率
	var modelRatio float64 = 1.0
	var completionRatio float64 = 2.0
	var groupRatio float64 = 1.0

	// 尝试从 other 字段中解析倍率信息
	if log.Other != "" {
		otherMap, err := common.StrToMap(log.Other)
		if err == nil && otherMap != nil {
			if mr, ok := otherMap["model_ratio"]; ok {
				if mrFloat, ok := mr.(float64); ok {
					modelRatio = mrFloat
				}
			}
			if cr, ok := otherMap["completion_ratio"]; ok {
				if crFloat, ok := cr.(float64); ok {
					completionRatio = crFloat
				}
			}
			if gr, ok := otherMap["group_ratio"]; ok {
				if grFloat, ok := gr.(float64); ok {
					groupRatio = grFloat
				}
			}
		}
	}

	// 如果 other 字段中没有倍率信息，尝试从系统配置获取
	if modelRatio == 1.0 {
		if mr, success, _ := ratio_setting.GetModelRatio(log.ModelName); success {
			modelRatio = mr
		}
	}
	if completionRatio == 2.0 {
		if cr := ratio_setting.GetCompletionRatio(log.ModelName); cr > 0 {
			completionRatio = cr
		}
	}
	if groupRatio == 1.0 {
		if gr := ratio_setting.GetGroupRatio(log.Group); gr > 0 {
			groupRatio = gr
		}
	}

	// 根据实际数据分析：
	// model_ratio=1.5, input_price=$0.5/1M, output_price=$2.5/1M, completion_ratio=5
	// 输入价格 = model_ratio * (1/3) = 1.5 * (1/3) = 0.5 ✓
	// 输出价格 = 输入价格 * completion_ratio = 0.5 * 5 = 2.5 ✓
	inputRatioPrice := modelRatio * (1.0 / 3.0) // model_ratio / 3
	outputRatioPrice := inputRatioPrice * completionRatio

	// 计算输入金额和输出金额
	promptTokens := float64(log.PromptTokens)
	completionTokens := float64(log.CompletionTokens)

	// (tokens / 1,000,000) * price per 1M tokens * group ratio
	inputAmount := (promptTokens / 1000000) * inputRatioPrice * groupRatio
	outputAmount := (completionTokens / 1000000) * outputRatioPrice * groupRatio

	// 格式化显示字符串
	log.InputPriceDisplay = fmt.Sprintf("$%.3f / 1M", inputRatioPrice)
	log.OutputPriceDisplay = fmt.Sprintf("$%.3f / 1M", outputRatioPrice)
	log.InputAmountDisplay = fmt.Sprintf("$%.6f", inputAmount)
	log.OutputAmountDisplay = fmt.Sprintf("$%.6f", outputAmount)
}

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// delete admin
			delete(otherMap, "admin_info")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = logs[i].Id % 1024

		// 计算价格字段
		calculatePriceFields(logs[i])
	}
}

func GetLogByKey(key string, logType int, startTimestamp int64, endTimestamp int64, modelName string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB

	if os.Getenv("LOG_SQL_DSN") != "" {
		// 有单独的日志数据库
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, 0, err
		}

		if logType == LogTypeUnknown {
			tx = LOG_DB.Where("token_id = ?", tk.Id)
		} else {
			tx = LOG_DB.Where("token_id = ? and type = ?", tk.Id, logType)
		}
	} else {
		// 使用主数据库
		if logType == LogTypeUnknown {
			tx = LOG_DB.Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ?", strings.TrimPrefix(key, "sk-"))
		} else {
			tx = LOG_DB.Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ? and logs.type = ?", strings.TrimPrefix(key, "sk-"), logType)
		}
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}

	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	// 批量加载channel信息 - 与GetAllLogs相同的逻辑
	channelIdsMap := make(map[int]struct{})
	channelMap := make(map[int]string)
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIdsMap[log.ChannelId] = struct{}{}
		}
	}

	channelIds := make([]int, 0, len(channelIdsMap))
	for channelId := range channelIdsMap {
		channelIds = append(channelIds, channelId)
	}
	if len(channelIds) > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	formatUserLogs(logs)
	return logs, total, err
}

// GetLogByKeyLightweight 轻量级查询，只返回核心字段，用于大数据量场景
func GetLogByKeyLightweight(key string, logType int, startTimestamp int64, endTimestamp int64, modelName string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB

	if os.Getenv("LOG_SQL_DSN") != "" {
		// 有单独的日志数据库
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, 0, err
		}

		if logType == LogTypeUnknown {
			tx = LOG_DB.Select("id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream").Where("token_id = ?", tk.Id)
		} else {
			tx = LOG_DB.Select("id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream").Where("token_id = ? and type = ?", tk.Id, logType)
		}
	} else {
		// 使用主数据库
		if logType == LogTypeUnknown {
			tx = LOG_DB.Select("logs.id, logs.created_at, logs.type, logs.content, logs.model_name, logs.quota, logs.prompt_tokens, logs.completion_tokens, logs.use_time, logs.is_stream").Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ?", strings.TrimPrefix(key, "sk-"))
		} else {
			tx = LOG_DB.Select("logs.id, logs.created_at, logs.type, logs.content, logs.model_name, logs.quota, logs.prompt_tokens, logs.completion_tokens, logs.use_time, logs.is_stream").Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ? and logs.type = ?", strings.TrimPrefix(key, "sk-"), logType)
		}
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}

	// 先统计总数
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	// 为日志添加价格计算字段
	for i := range logs {
		calculatePriceFields(logs[i])
	}

	return logs, total, err
}

// GetLogByKeyCursor 基于游标的分页查询，适用于超大数据量（100万+）
func GetLogByKeyCursor(key string, logType int, startTimestamp int64, endTimestamp int64, modelName string, pageSize int, group string, cursor string, lightweight bool) (logs []*Log, nextCursor string, err error) {
	var tx *gorm.DB
	var cursorTimestamp int64
	var cursorId int64

	// 解析游标
	if cursor != "" {
		parts := strings.Split(cursor, "_")
		if len(parts) == 2 {
			cursorTimestamp, _ = strconv.ParseInt(parts[0], 10, 64)
			cursorId, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}

	// 选择要查询的字段
	selectFields := "*"
	if lightweight {
		selectFields = "id, created_at, type, content, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream"
	}

	if os.Getenv("LOG_SQL_DSN") != "" {
		// 有单独的日志数据库
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, "", err
		}

		if logType == LogTypeUnknown {
			tx = LOG_DB.Select(selectFields).Where("token_id = ?", tk.Id)
		} else {
			tx = LOG_DB.Select(selectFields).Where("token_id = ? and type = ?", tk.Id, logType)
		}
	} else {
		// 使用主数据库
		if lightweight {
			selectFields = "logs.id, logs.created_at, logs.type, logs.content, logs.model_name, logs.quota, logs.prompt_tokens, logs.completion_tokens, logs.use_time, logs.is_stream"
		} else {
			selectFields = "logs.*"
		}

		if logType == LogTypeUnknown {
			tx = LOG_DB.Select(selectFields).Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ?", strings.TrimPrefix(key, "sk-"))
		} else {
			tx = LOG_DB.Select(selectFields).Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ? and logs.type = ?", strings.TrimPrefix(key, "sk-"), logType)
		}
	}

	// 添加筛选条件
	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}

	// 游标分页：使用 created_at 和 id 的组合进行分页
	if cursor != "" {
		// 查询比游标更早的记录 (created_at < cursor_timestamp OR (created_at = cursor_timestamp AND id < cursor_id))
		tx = tx.Where("(logs.created_at < ?) OR (logs.created_at = ? AND logs.id < ?)", cursorTimestamp, cursorTimestamp, cursorId)
	}

	// 按时间倒序，ID倒序，多查询一条用于判断是否还有更多数据
	err = tx.Order("logs.created_at DESC, logs.id DESC").Limit(pageSize + 1).Find(&logs).Error
	if err != nil {
		return nil, "", err
	}

	// 判断是否还有更多数据
	hasMore := len(logs) > pageSize
	if hasMore {
		logs = logs[:pageSize] // 移除多查询的那一条
	}

	// 生成下一页的游标
	if len(logs) > 0 && hasMore {
		lastLog := logs[len(logs)-1]
		nextCursor = fmt.Sprintf("%d_%d", lastLog.CreatedAt, lastLog.Id)
	}

	// 如果不是轻量级查询，需要处理channel信息和格式化
	if !lightweight && len(logs) > 0 {
		// 批量加载channel信息
		channelIdsMap := make(map[int]struct{})
		channelMap := make(map[int]string)
		for _, log := range logs {
			if log.ChannelId != 0 {
				channelIdsMap[log.ChannelId] = struct{}{}
			}
		}

		channelIds := make([]int, 0, len(channelIdsMap))
		for channelId := range channelIdsMap {
			channelIds = append(channelIds, channelId)
		}
		if len(channelIds) > 0 {
			var channels []struct {
				Id   int    `gorm:"column:id"`
				Name string `gorm:"column:name"`
			}
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
				return logs, nextCursor, err
			}
			for _, channel := range channels {
				channelMap[channel.Id] = channel.Name
			}
			for i := range logs {
				logs[i].ChannelName = channelMap[logs[i].ChannelId]
			}
		}
		formatUserLogs(logs)
	} else {
		// 即使是轻量级查询，也需要计算价格字段
		for i := range logs {
			calculatePriceFields(logs[i])
		}
	}

	return logs, nextCursor, nil
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if username != "" {
		tx = tx.Where("logs.username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	formatUserLogs(logs)
	return logs, total, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	if username != "" {
		tx = tx.Where("username = ?", username)
		rpmTpmQuery = rpmTpmQuery.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	tx.Scan(&stat)
	rpmTpmQuery.Scan(&stat)

	return stat
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
