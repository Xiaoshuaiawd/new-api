package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

const channelModelRateLimitWindowSeconds int64 = 60

var channelModelRateLimiter common.InMemoryRateLimiter

// CheckChannelModelRateLimit enforces a global per-(channel, model) RPM limit.
// It returns a *types.NewAPIError with HTTP 429 when rate limited.
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
		if allowed {
			return nil
		}
		msg := fmt.Sprintf("当前渠道模型请求过于频繁（限制 %d RPM），请稍后重试", rpm)
		return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeChannelModelRateLimitExceeded, http.StatusTooManyRequests, types.ErrOptionWithNoRecordErrorLog())
	}

	channelModelRateLimiter.Init(10 * time.Minute)
	if channelModelRateLimiter.Request(key, rpm, channelModelRateLimitWindowSeconds) {
		return nil
	}
	msg := fmt.Sprintf("当前渠道模型请求过于频繁（限制 %d RPM），请稍后重试", rpm)
	return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeChannelModelRateLimitExceeded, http.StatusTooManyRequests, types.ErrOptionWithNoRecordErrorLog())
}
