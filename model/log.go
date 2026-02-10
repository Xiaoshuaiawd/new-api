package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/hints"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:2"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type,priority:1;index:idx_type_created_at,priority:2;index:idx_type_created_quota,priority:2;index:idx_user_id_created_at,priority:2"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type,priority:2;index:idx_type_created_at,priority:1;index:idx_type_created_quota,priority:1;index:idx_type_username_created_quota,priority:1"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index:idx_logs_username;index:index_username_model_name,priority:2;index:idx_type_username_created_quota,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index:idx_logs_token_name;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0;index:idx_type_created_quota,priority:3;index:idx_type_username_created_quota,priority:4"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0;index:idx_type_created_quota,priority:4"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0;index:idx_type_created_quota,priority:5"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index:idx_logs_channel_id"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index:idx_logs_token_id"`
	Group            string `json:"group" gorm:"index:idx_logs_group"`
	Ip               string `json:"ip" gorm:"index:idx_logs_ip;default:''"`
	RequestId        string `json:"request_id" gorm:"index"`
	Other            string `json:"other"`
	// 价格显示字段 (不存储到数据库，仅用于API返回)
	InputPriceDisplay   string `json:"input_price_display" gorm:"-"`
	OutputPriceDisplay  string `json:"output_price_display" gorm:"-"`
	InputAmountDisplay  string `json:"input_amount_display" gorm:"-"`
	OutputAmountDisplay string `json:"output_amount_display" gorm:"-"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

// PricingModelData 定义模型价格数据结构
type PricingModelData struct {
	ModelName       string  `json:"model_name"`
	ModelRatio      float64 `json:"model_ratio"`
	CompletionRatio float64 `json:"completion_ratio"`
}

// PricingData 定义完整的pricing数据结构
type PricingData struct {
	Data       []PricingModelData `json:"data"`
	GroupRatio map[string]float64 `json:"group_ratio"`
}

// 全局缓存变量
var (
	pricingCache      *PricingData
	pricingCacheMutex sync.RWMutex
	pricingCacheTime  time.Time
)

// getPricingData 获取pricing数据，带缓存机制
func getPricingData() (*PricingData, error) {
	pricingCacheMutex.RLock()
	// 检查缓存是否有效（5分钟内）
	if pricingCache != nil && time.Since(pricingCacheTime) < 5*time.Minute {
		defer pricingCacheMutex.RUnlock()
		return pricingCache, nil
	}
	pricingCacheMutex.RUnlock()

	// 需要更新缓存
	pricingCacheMutex.Lock()
	defer pricingCacheMutex.Unlock()

	// 双重检查，防止并发更新
	if pricingCache != nil && time.Since(pricingCacheTime) < 5*time.Minute {
		return pricingCache, nil
	}

	// 获取pricing数据 - 调用model.GetPricing()获取实际的定价数据
	pricings := GetPricing()
	var pricingModels []PricingModelData

	// 转换为我们需要的格式
	for _, pricing := range pricings {
		pricingModels = append(pricingModels, PricingModelData{
			ModelName:       pricing.ModelName,
			ModelRatio:      pricing.ModelRatio,
			CompletionRatio: pricing.CompletionRatio,
		})
	}

	groupRatio := ratio_setting.GetGroupRatioCopy()

	pricingData := &PricingData{
		Data:       pricingModels,
		GroupRatio: groupRatio,
	}

	pricingCache = pricingData
	pricingCacheTime = time.Now()

	return pricingCache, nil
}

// findModelRatio 从pricing数据中查找指定模型的倍率信息
func findModelRatio(pricingData *PricingData, modelName string) (modelRatio float64, completionRatio float64, found bool) {
	for _, model := range pricingData.Data {
		if model.ModelName == modelName {
			return model.ModelRatio, model.CompletionRatio, true
		}
	}
	return 0, 0, false
}

// calculatePriceFields 计算并设置价格显示字段
func calculatePriceFields(log *Log) {
	// 默认倍率
	var modelRatio float64 = 1.0
	var completionRatio float64 = 2.0
	var groupRatio float64 = 1.0

	// 直接从 /api/pricing 获取倍率信息
	if pricingData, err := getPricingData(); err == nil {
		// 查找模型倍率
		if mr, cr, found := findModelRatio(pricingData, log.ModelName); found {
			modelRatio = mr
			completionRatio = cr
		}

		// 获取分组倍率
		if gr, ok := pricingData.GroupRatio[log.Group]; ok {
			groupRatio = gr
		}
	}

	// 如果从 pricing 获取失败，回退到系统配置
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

	// 正确的计算公式：
	// 输入价格 = 输入倍率(model_ratio) × 2
	// 输出价格 = 输入价格 × 输出倍率(completion_ratio)
	// 例如：model_ratio=1.5, completion_ratio=5
	// 输入价格 = 1.5 × 2 = 3.0 → "$3.000 / 1M"
	// 输出价格 = 3.0 × 5 = 15.0 → "$15.000 / 1M"
	inputRatioPrice := modelRatio * 2.0
	outputRatioPrice := inputRatioPrice * completionRatio

	// 计算输入金额和输出金额
	promptTokens := float64(log.PromptTokens)
	completionTokens := float64(log.CompletionTokens)

	// 应用分组倍率到金额计算
	inputAmount := (promptTokens / 1000000) * inputRatioPrice * groupRatio
	outputAmount := (completionTokens / 1000000) * outputRatioPrice * groupRatio

	// 格式化显示字符串 - 价格和金额都只返回数值，前端负责格式化显示
	log.InputPriceDisplay = fmt.Sprintf("%.0f", inputRatioPrice)
	log.OutputPriceDisplay = fmt.Sprintf("%.0f", outputRatioPrice)
	log.InputAmountDisplay = fmt.Sprintf("%.6f", inputAmount)
	log.OutputAmountDisplay = fmt.Sprintf("%.6f", outputAmount)
}

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			delete(otherMap, "reject_reason")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = logs[i].Id % 1024

		// 计算价格字段
		calculatePriceFields(logs[i])
	}
}

// applyLogRangeIndexHints adds index hints for time-range queries on MySQL to avoid full scans.
func applyLogRangeIndexHints(tx *gorm.DB, startTimestamp int64, endTimestamp int64, hasTypeFilter bool) *gorm.DB {
	if common.LogSqlType != common.DatabaseTypeMySQL {
		return tx
	}
	if startTimestamp == 0 && endTimestamp == 0 {
		return tx
	}
	if hasTypeFilter {
		return tx.Clauses(hints.UseIndex("idx_type_created_quota"))
	}
	return tx.Clauses(hints.UseIndex("idx_created_at_type"))
}

func GetLogByKey(key string, logType int, startTimestamp int64, endTimestamp int64, modelName string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	hasTypeFilter := logType != LogTypeUnknown
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

	tx = applyLogRangeIndexHints(tx, startTimestamp, endTimestamp, hasTypeFilter)

	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
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
	hasTypeFilter := logType != LogTypeUnknown
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

	tx = applyLogRangeIndexHints(tx, startTimestamp, endTimestamp, hasTypeFilter)

	// 先统计总数
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
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
	hasTypeFilter := logType != LogTypeUnknown
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

	tx = applyLogRangeIndexHints(tx, startTimestamp, endTimestamp, hasTypeFilter)

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
	requestId := c.GetString(common.RequestIdKey)
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
		RequestId: requestId,
		Other:     otherStr,
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
	requestId := c.GetString(common.RequestIdKey)
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
		RequestId: requestId,
		Other:     otherStr,
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

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string) (logs []*Log, total int64, err error) {
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
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
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
	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
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

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	hasTypeFilter := logType != LogTypeUnknown
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if modelName != "" {
		modelNamePattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return nil, 0, err
		}
		tx = tx.Where("logs.model_name LIKE ? ESCAPE '!'", modelNamePattern)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
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
	tx = applyLogRangeIndexHints(tx, startTimestamp, endTimestamp, hasTypeFilter)
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs)
	return logs, total, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	filterType := LogTypeConsume
	if logType != LogTypeUnknown {
		filterType = logType
	}

	applyFilters := func(db *gorm.DB) *gorm.DB {
		db = db.Where("type = ?", filterType)
		if username != "" {
			db = db.Where("username = ?", username)
		}
		if tokenName != "" {
			db = db.Where("token_name = ?", tokenName)
		}
		if modelName != "" {
			db = db.Where("model_name like ?", modelName)
		}
		if channel != 0 {
			db = db.Where("channel_id = ?", channel)
		}
		if group != "" {
			db = db.Where(logGroupCol+" = ?", group)
		}
		return db
	}

	baseFiltered := applyFilters(LOG_DB.Table("logs"))
	baseFiltered = applyLogRangeIndexHints(baseFiltered, startTimestamp, endTimestamp, true)

	quotaQuery := baseFiltered
	if startTimestamp != 0 {
		quotaQuery = quotaQuery.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		quotaQuery = quotaQuery.Where("created_at <= ?", endTimestamp)
	}
	var quotaResult struct {
		Quota int
	}
	if err = quotaQuery.Select("COALESCE(sum(quota),0) as quota").Scan(&quotaResult).Error; err != nil {
		return stat, err
	}

	cutoff := time.Now().Add(-60 * time.Second).Unix()
	rpmTpmQuery := applyFilters(LOG_DB.Table("logs"))
	rpmTpmQuery = applyLogRangeIndexHints(rpmTpmQuery, cutoff, 0, true).Where("created_at >= ?", cutoff)
	var rpmTpmResult struct {
		Rpm int
		Tpm int
	}
	if err = rpmTpmQuery.Select("count(*) as rpm, COALESCE(sum(prompt_tokens) + sum(completion_tokens),0) as tpm").Scan(&rpmTpmResult).Error; err != nil {
		return stat, err
	}

	stat.Quota = quotaResult.Quota
	stat.Rpm = rpmTpmResult.Rpm
	stat.Tpm = rpmTpmResult.Tpm

	return stat, nil
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
	tx = applyLogRangeIndexHints(tx, startTimestamp, endTimestamp, true)
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

// ensureLogIndexes ensures critical composite indexes exist to keep log queries fast.
func ensureLogIndexes(db *gorm.DB) error {
	// These names must match the gorm index tags on Log.
	indexes := []string{
		"idx_created_at_id",
		"idx_created_at_type",
		"idx_type_created_at",
		"idx_type_created_quota",
		"idx_type_username_created_quota",
		"idx_user_id_created_at",
		"idx_created_at_desc_id_desc",
		"index_username_model_name",
		"idx_logs_channel_id",
		"idx_logs_ip",
		"idx_logs_user_id",
		"idx_logs_username",
		"idx_logs_token_name",
		"idx_logs_model_name",
		"idx_logs_token_id",
		"idx_logs_group",
	}
	for _, idx := range indexes {
		if db.Migrator().HasIndex(&Log{}, idx) {
			continue
		}
		if err := db.Migrator().CreateIndex(&Log{}, idx); err != nil {
			return fmt.Errorf("create index %s failed: %w", idx, err)
		}
	}
	return nil
}
