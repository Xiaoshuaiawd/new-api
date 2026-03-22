package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

var redisSlidingWindowAllowAndRecordScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local duration = tonumber(ARGV[2])
local max_count = tonumber(ARGV[3])
local entry = ARGV[4]
local expire_seconds = tonumber(ARGV[5])

if max_count <= 0 then
	return 1
end

while true do
	local oldest = redis.call('LINDEX', key, -1)
	if not oldest then
		break
	end

	local ts = tonumber(string.match(oldest, '^(%d+)'))
	if not ts then
		redis.call('RPOP', key)
	elseif now - ts >= duration then
		redis.call('RPOP', key)
	else
		break
	end
end

local current = redis.call('LLEN', key)
if current >= max_count then
	redis.call('EXPIRE', key, expire_seconds)
	return 0
end

redis.call('LPUSH', key, entry)
redis.call('LTRIM', key, 0, max_count - 1)
redis.call('EXPIRE', key, expire_seconds)
return 1
`)

func getModelRateLimitIdentifier(c *gin.Context) string {
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenId > 0 {
		return fmt.Sprintf("token:%d", tokenId)
	}

	tokenKey := common.GetContextKeyString(c, constant.ContextKeyTokenKey)
	if tokenKey != "" {
		return "token:" + common.GenerateHMAC(tokenKey)
	}

	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userId > 0 {
		return fmt.Sprintf("user:%d", userId)
	}

	return ""
}

func makeRateLimitEntry(c *gin.Context) string {
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d|%s", time.Now().Unix(), requestID)
}

func redisAllowAndRecordRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64, entry string) (bool, error) {
	if maxCount == 0 {
		return true, nil
	}
	expireSeconds := duration + 60
	if expireSeconds < 60 {
		expireSeconds = 60
	}
	result, err := redisSlidingWindowAllowAndRecordScript.Run(
		ctx,
		rdb,
		[]string{key},
		time.Now().Unix(),
		duration,
		maxCount,
		entry,
		expireSeconds,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func redisRollbackRateLimitRecord(ctx context.Context, rdb *redis.Client, key, entry string) {
	if entry == "" {
		return
	}
	_, _ = rdb.LRem(ctx, key, 1, entry).Result()
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		subjectKey := getModelRateLimitIdentifier(c)
		if subjectKey == "" {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "invalid_rate_limit_subject")
			return
		}
		ctx := context.Background()
		rdb := common.RDB
		requestEntry := makeRateLimitEntry(c)

		// 1. 检查总请求数限制并记录总请求（包含失败请求）
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitCountMark, subjectKey)
			allowed, err := redisAllowAndRecordRateLimit(ctx, rdb, totalKey, totalMaxCount, duration, requestEntry)
			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
				return
			}
		}

		// 2. 检查成功请求数限制（预占位，失败后回滚）
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, subjectKey)
		allowed, err := redisAllowAndRecordRateLimit(ctx, rdb, successKey, successMaxCount, duration, requestEntry)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 请求失败则回滚成功请求预占位
		if c.Writer.Status() >= 400 {
			redisRollbackRateLimitRecord(ctx, rdb, successKey, requestEntry)
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		subjectKey := getModelRateLimitIdentifier(c)
		if subjectKey == "" {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		totalKey := ModelRequestRateLimitCountMark + ":" + subjectKey
		successKey := ModelRequestRateLimitSuccessCountMark + ":" + subjectKey

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		if !inMemoryRateLimiter.Allow(successKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
