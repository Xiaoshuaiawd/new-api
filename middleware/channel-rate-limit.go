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
// When rate limited, it automatically disables the model for this channel for 1 minute.
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
	} else {
		channelModelRateLimiter.Init(10 * time.Minute)
		rateLimited = !channelModelRateLimiter.Request(key, rpm, channelModelRateLimitWindowSeconds)
	}

	if rateLimited {
		// Immediately disable the model BEFORE returning error
		// This ensures the model is disabled synchronously before retry
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」触发 RPM 限流（限制 %d RPM），立即禁用", channelId, matchName, rpm))

		// Use special prefix to mark this as RPM-limit-triggered disable (for auto-recovery)
		reason := fmt.Sprintf("%s 触发 RPM 限流（限制 %d RPM），自动禁用 1 分钟", rateLimitDisablePrefix, rpm)
		changed, err := model.DisableChannelModel(channelId, matchName, reason)
		if err != nil {
			common.SysLog(fmt.Sprintf("禁用模型失败：渠道 #%d 模型「%s」，错误：%v", channelId, matchName, err))
			// Continue to return error even if disable failed
		} else if changed {
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

		// Return error to trigger retry with other channels
		// The model is already disabled, so next retry will skip this channel
		msg := fmt.Sprintf("渠道模型触发 RPM 限流（限制 %d RPM），已切换到其他渠道", rpm)
		return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeChannelModelRateLimitExceeded, http.StatusTooManyRequests, types.ErrOptionWithNoRecordErrorLog())
	}

	return nil
}
