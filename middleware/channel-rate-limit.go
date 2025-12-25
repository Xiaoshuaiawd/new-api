package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
)

const channelModelRateLimitWindowSeconds int64 = 60

// Special prefix to identify rate-limit-triggered disables (for auto-recovery)
const rateLimitDisablePrefix = "[AUTO_RPM_LIMIT]"

var channelModelRateLimiter common.InMemoryRateLimiter

// CheckChannelModelRateLimit enforces a global per-(channel, model) RPM limit.
// It returns a *types.NewAPIError with HTTP 429 when rate limited.
// When approaching the limit, it proactively disables the model to prevent future requests.
func CheckChannelModelRateLimit(channelId int, modelName string) *types.NewAPIError {
	rpm := setting.ChannelModelRateLimitRPM
	if rpm <= 0 {
		return nil
	}
	if channelId <= 0 || modelName == "" {
		return nil
	}

	matchName := ratio_setting.FormatMatchingModelName(modelName)
	if matchName == "" {
		matchName = modelName
	}

	// Keep key namespace distinct from other rate limiters.
	key := fmt.Sprintf("rateLimit:CM:%d:%s", channelId, matchName)

	var rateLimited bool
	var shouldDisable bool // Whether we should proactively disable the model

	if common.RedisEnabled {
		ctx := context.Background()

		tb := limiter.New(ctx, common.RDB)
		allowed, err := tb.Allow(
			ctx,
			key,
			limiter.WithCapacity(int64(rpm)*channelModelRateLimitWindowSeconds),
			limiter.WithRate(int64(rpm)),
			limiter.WithRequested(channelModelRateLimitWindowSeconds),
		)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError, types.ErrOptionWithNoRecordErrorLog())
		}
		rateLimited = !allowed

		// If request was allowed, check if tokens are now insufficient for next request
		// This allows us to proactively disable BEFORE the next request hits rate limit
		if allowed {
			tokensAfterRequest, err := common.RDB.HGet(ctx, key, "tokens").Float64()
			if err == nil {
				// If remaining tokens are less than what's needed for next request, disable now
				// This ensures the (rpm+1)th request never sees a rate limit error
				if tokensAfterRequest < float64(channelModelRateLimitWindowSeconds) {
					shouldDisable = true
					// Calculate how many requests were made (capacity - remaining tokens) / request size
					requestsMade := int((float64(int64(rpm)*channelModelRateLimitWindowSeconds) - tokensAfterRequest) / float64(channelModelRateLimitWindowSeconds))
					common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」已达到 RPM 限制（%d/%d 次请求），主动禁用以避免下次触发限流", channelId, matchName, requestsMade, rpm))
				}
			}
		}
	} else {
		channelModelRateLimiter.Init(10 * time.Minute)
		rateLimited = !channelModelRateLimiter.Request(key, rpm, channelModelRateLimitWindowSeconds)

		// For in-memory limiter, we can't easily check remaining tokens
		// So we only disable on rate limit, not proactively
	}

	// If already rate limited, return error immediately
	// The actual disable will be handled by processChannelError synchronously
	if rateLimited {
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」触发 RPM 限流（限制 %d RPM）", channelId, matchName, rpm))

		// Return error to trigger retry with other channels
		// processChannelError will handle the synchronous disable
		msg := fmt.Sprintf("渠道模型触发 RPM 限流（限制 %d RPM），已切换到其他渠道", rpm)
		return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeChannelModelRateLimitExceeded, http.StatusTooManyRequests, types.ErrOptionWithNoRecordErrorLog())
	}

	// If we should proactively disable (tokens exhausted but request still allowed)
	if shouldDisable {
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」RPM 配额已用完，主动禁用 1 分钟", channelId, matchName))

		// Disable SYNCHRONOUSLY to ensure it takes effect before next request
		// This is critical to prevent the next request from seeing rate limit error
		disableModelForRateLimit(channelId, matchName, rpm)
	}

	return nil
}

// disableModelForRateLimit disables a model due to rate limit and schedules auto-recovery
func disableModelForRateLimit(channelId int, matchName string, rpm int) {
	// Use special prefix to mark this as RPM-limit-triggered disable (for auto-recovery)
	reason := fmt.Sprintf("%s 触发 RPM 限流（限制 %d RPM），自动禁用 1 分钟", rateLimitDisablePrefix, rpm)
	changed, err := model.DisableChannelModel(channelId, matchName, reason)
	if err != nil {
		common.SysLog(fmt.Sprintf("禁用模型失败：渠道 #%d 模型「%s」，错误：%v", channelId, matchName, err))
		return
	}

	if !changed {
		// Model already disabled, no need to schedule recovery again
		return
	}

	common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」已被自动禁用，将在 1 分钟后自动恢复", channelId, matchName))

	// Schedule automatic re-enable after 1 minute
	gopool.Go(func() {
		time.Sleep(60 * time.Second)

		// Only re-enable if the disable reason still has the RPM limit prefix
		// This prevents re-enabling models that were manually disabled or disabled by keyword errors
		channel, err := model.CacheGetChannel(channelId)
		if err != nil {
			common.SysLog(fmt.Sprintf("自动恢复模型失败：无法获取渠道 #%d，错误：%v", channelId, err))
			return
		}

		if channel.ChannelInfo.DisabledModels != nil {
			if disabledInfo, exists := channel.ChannelInfo.DisabledModels[matchName]; exists {
				// Only auto-enable if the disable reason starts with the RPM limit prefix
				if len(disabledInfo.Reason) >= len(rateLimitDisablePrefix) &&
					disabledInfo.Reason[:len(rateLimitDisablePrefix)] == rateLimitDisablePrefix {
					enabled, err := model.EnableChannelModel(channelId, matchName)
					if err != nil {
						common.SysLog(fmt.Sprintf("自动恢复模型失败：渠道 #%d 模型「%s」，错误：%v", channelId, matchName, err))
						return
					}
					if enabled {
						common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」已自动恢复启用", channelId, matchName))
					}
				} else {
					common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」禁用原因已变更（可能被手动禁用或关键词触发），跳过自动恢复", channelId, matchName))
				}
			}
		}
	})
}
