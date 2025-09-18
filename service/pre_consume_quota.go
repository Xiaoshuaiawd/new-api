package service

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo.FinalPreConsumedQuota != 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s", relayInfo.UserId, logger.FormatQuota(relayInfo.FinalPreConsumedQuota)))
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := PostConsumeQuota(&relayInfoCopy, -relayInfoCopy.FinalPreConsumedQuota, 0, false)
			if err != nil {
				common.SysLog("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It returns the pre-consumed quota if successful, or an error if not.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	// 使用统一额度服务检查用户额度
	sufficient, quotaInfo, err := CheckUserQuotaAvailability(relayInfo.UserId, preConsumedQuota)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}

	if quotaInfo.AvailableQuota <= 0 {
		quotaTypeStr := "传统额度"
		if quotaInfo.HasSubscription {
			quotaTypeStr = fmt.Sprintf("套餐额度(%s)", quotaInfo.PackageName)
		}
		return types.NewErrorWithStatusCode(fmt.Errorf("用户%s不足, 剩余额度: %s", quotaTypeStr, logger.FormatQuota(quotaInfo.AvailableQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	if !sufficient {
		quotaTypeStr := "传统额度"
		if quotaInfo.HasSubscription {
			quotaTypeStr = fmt.Sprintf("套餐额度(%s)", quotaInfo.PackageName)
		}
		return types.NewErrorWithStatusCode(fmt.Errorf("预扣费额度失败, 用户剩余%s: %s, 需要预扣费额度: %s", quotaTypeStr, logger.FormatQuota(quotaInfo.AvailableQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	trustQuota := common.GetTrustQuota()

	// 为了向后兼容，设置UserQuota字段为可用额度
	relayInfo.UserQuota = quotaInfo.AvailableQuota

	if quotaInfo.AvailableQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				quotaTypeStr := "传统额度"
				if quotaInfo.HasSubscription {
					quotaTypeStr = fmt.Sprintf("套餐额度(%s)", quotaInfo.PackageName)
				}
				logger.LogInfo(c, fmt.Sprintf("用户 %d 剩余%s %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费", relayInfo.UserId, quotaTypeStr, logger.FormatQuota(quotaInfo.AvailableQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			quotaTypeStr := "传统额度"
			if quotaInfo.HasSubscription {
				quotaTypeStr = fmt.Sprintf("套餐额度(%s)", quotaInfo.PackageName)
			}
			logger.LogInfo(c, fmt.Sprintf("用户 %d %s充足且为无限额度令牌, 信任且不需要预扣费", relayInfo.UserId, quotaTypeStr))
		}
	}

	if preConsumedQuota > 0 {
		err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}

		// 使用统一额度服务消费用户额度
		err = ConsumeUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}

		quotaTypeStr := "传统额度"
		if quotaInfo.HasSubscription {
			quotaTypeStr = fmt.Sprintf("套餐额度(%s)", quotaInfo.PackageName)
		}
		logger.LogInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余%s: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), quotaTypeStr, logger.FormatQuota(quotaInfo.AvailableQuota-preConsumedQuota)))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	return nil
}
