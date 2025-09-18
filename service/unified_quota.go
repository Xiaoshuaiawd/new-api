package service

import (
	"errors"
	"one-api/model"
)

// UnifiedQuotaInfo 统一额度信息
type UnifiedQuotaInfo struct {
	UserId            int    `json:"user_id"`
	HasSubscription   bool   `json:"has_subscription"`
	QuotaType         string `json:"quota_type"`         // "subscription" or "traditional"
	AvailableQuota    int    `json:"available_quota"`    // 可用额度
	TotalQuota        int    `json:"total_quota"`        // 总额度
	UsedQuota         int    `json:"used_quota"`         // 已用额度
	PackageId         int    `json:"package_id"`         // 套餐ID (如果是订阅类型)
	PackageName       string `json:"package_name"`       // 套餐名称
	SubscriptionId    int    `json:"subscription_id"`    // 用户订阅ID
}

// GetUserUnifiedQuota 获取用户统一额度信息 (优先使用订阅套餐额度)
func GetUserUnifiedQuota(userId int) (*UnifiedQuotaInfo, error) {
	quotaInfo := &UnifiedQuotaInfo{
		UserId: userId,
	}

	// 首先检查用户是否有有效的订阅套餐
	activeSubscription, err := model.GetActiveUserSubscription(userId)
	if err == nil && activeSubscription != nil {
		// 用户有有效订阅，使用订阅套餐额度
		quotaInfo.HasSubscription = true
		quotaInfo.QuotaType = "subscription"
		quotaInfo.SubscriptionId = activeSubscription.Id
		quotaInfo.PackageId = activeSubscription.PackageId

		// 获取套餐信息
		subscriptionPackage, err := model.GetSubscriptionPackageById(activeSubscription.PackageId)
		if err != nil {
			return nil, errors.New("获取订阅套餐信息失败")
		}
		quotaInfo.PackageName = subscriptionPackage.Name

		// 计算可用额度
		availableQuota, err := model.CalculateAvailableQuota(activeSubscription)
		if err != nil {
			return nil, err
		}

		quotaInfo.AvailableQuota = availableQuota
		quotaInfo.TotalQuota = int(subscriptionPackage.PermanentQuota + activeSubscription.MonthlyQuotaUsed + activeSubscription.DailyQuotaUsed)
		quotaInfo.UsedQuota = int(activeSubscription.PermanentQuotaUsed + activeSubscription.MonthlyQuotaUsed + activeSubscription.DailyQuotaUsed)

		return quotaInfo, nil
	}

	// 用户没有有效订阅，使用传统额度系统
	user, err := model.GetUserById(userId, true)
	if err != nil {
		return nil, err
	}

	quotaInfo.HasSubscription = false
	quotaInfo.QuotaType = "traditional"
	quotaInfo.AvailableQuota = user.Quota - user.UsedQuota
	quotaInfo.TotalQuota = user.Quota
	quotaInfo.UsedQuota = user.UsedQuota

	return quotaInfo, nil
}

// CheckUserQuotaAvailability 检查用户额度是否足够 (统一接口)
func CheckUserQuotaAvailability(userId int, requiredQuota int) (bool, *UnifiedQuotaInfo, error) {
	quotaInfo, err := GetUserUnifiedQuota(userId)
	if err != nil {
		return false, nil, err
	}

	// 检查可用额度是否足够
	if quotaInfo.AvailableQuota >= requiredQuota {
		return true, quotaInfo, nil
	}

	return false, quotaInfo, nil
}

// ConsumeUserQuota 消费用户额度 (统一接口)
func ConsumeUserQuota(userId int, quotaToConsume int) error {
	quotaInfo, err := GetUserUnifiedQuota(userId)
	if err != nil {
		return err
	}

	// 检查额度是否足够
	if quotaInfo.AvailableQuota < quotaToConsume {
		return errors.New("额度不足")
	}

	if quotaInfo.HasSubscription {
		// 使用订阅套餐额度消费
		activeSubscription, err := model.GetActiveUserSubscription(userId)
		if err != nil {
			return err
		}
		return model.ConsumeSubscriptionQuota(activeSubscription, quotaToConsume)
	} else {
		// 使用传统额度消费
		return model.DecreaseUserQuota(userId, quotaToConsume)
	}
}

// GetUserQuotaForDisplay 获取用户额度用于前端显示
func GetUserQuotaForDisplay(userId int) (map[string]interface{}, error) {
	quotaInfo, err := GetUserUnifiedQuota(userId)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"user_id":         quotaInfo.UserId,
		"quota_type":      quotaInfo.QuotaType,
		"available_quota": quotaInfo.AvailableQuota,
		"total_quota":     quotaInfo.TotalQuota,
		"used_quota":      quotaInfo.UsedQuota,
		"has_subscription": quotaInfo.HasSubscription,
	}

	if quotaInfo.HasSubscription {
		result["package_id"] = quotaInfo.PackageId
		result["package_name"] = quotaInfo.PackageName
		result["subscription_id"] = quotaInfo.SubscriptionId

		// 获取详细的订阅额度信息
		activeSubscription, err := model.GetActiveUserSubscription(userId)
		if err == nil && activeSubscription != nil {
			subscriptionPackage, err := model.GetSubscriptionPackageById(activeSubscription.PackageId)
			if err == nil {
				result["permanent_quota"] = subscriptionPackage.PermanentQuota
				result["permanent_quota_used"] = activeSubscription.PermanentQuotaUsed
				result["monthly_quota"] = subscriptionPackage.MonthlyQuota
				result["monthly_quota_used"] = activeSubscription.MonthlyQuotaUsed
				result["daily_quota"] = subscriptionPackage.DailyQuota
				result["daily_quota_used"] = activeSubscription.DailyQuotaUsed
				result["subscription_status"] = activeSubscription.Status
				result["start_time"] = activeSubscription.StartTime
				result["end_time"] = activeSubscription.EndTime
			}
		}
	}

	return result, nil
}

// IsQuotaEnoughForPreConsume 预消费额度检查 (用于替换原有的额度检查逻辑)
func IsQuotaEnoughForPreConsume(userId int, requiredQuota int) (bool, error) {
	sufficient, _, err := CheckUserQuotaAvailability(userId, requiredQuota)
	if err != nil {
		return false, err
	}
	return sufficient, nil
}

// RefreshUserQuotaCache 刷新用户额度缓存
func RefreshUserQuotaCache(userId int) error {
	// 如果系统有缓存机制，这里可以清除用户额度相关的缓存
	// 目前先返回nil，后续可以根据需要扩展
	return nil
}