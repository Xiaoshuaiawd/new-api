package service

import (
	"fmt"
	"one-api/common"
	"one-api/model"
	"strings"
	"time"
)

var quotaSchedulerStop chan bool
var quotaSchedulerRunning bool

// InitQuotaScheduler 初始化额度调度器
func InitQuotaScheduler() {
	if quotaSchedulerRunning {
		return
	}

	quotaSchedulerStop = make(chan bool)
	quotaSchedulerRunning = true

	// 启动定时任务goroutine
	go func() {
		ticker := time.NewTicker(time.Hour) // 每小时检查一次
		defer ticker.Stop()

		for {
			select {
			case <-quotaSchedulerStop:
				common.SysLog("额度调度器已停止")
				return
			case <-ticker.C:
				checkAndRunTasks()
			}
		}
	}()

	common.SysLog("额度调度器已启动")
}

// StopQuotaScheduler 停止额度调度器
func StopQuotaScheduler() {
	if !quotaSchedulerRunning {
		return
	}

	quotaSchedulerRunning = false
	if quotaSchedulerStop != nil {
		close(quotaSchedulerStop)
	}
}

// checkAndRunTasks 检查并运行定时任务
func checkAndRunTasks() {
	now := time.Now()

	// 每天凌晨12点重置每日额度
	if now.Hour() == 0 && now.Minute() < 5 { // 在0:00-0:05之间执行
		common.SysLog("开始执行每日额度重置任务")
		err := model.ResetDailyQuota()
		if err != nil {
			common.SysError("每日额度重置失败: " + err.Error())
		} else {
			common.SysLog("每日额度重置完成")
		}
	}

	// 每小时检查并处理过期订阅
	common.SysLog("开始检查过期订阅")
	err := model.ExpireSubscriptions()
	if err != nil {
		common.SysError("处理过期订阅失败: " + err.Error())
	}

	// 每天凌晨1点检查并重置月额度
	if now.Hour() == 1 && now.Minute() < 5 { // 在1:00-1:05之间执行
		common.SysLog("开始检查月额度重置")
		err := ResetMonthlyQuotaIfNeeded()
		if err != nil {
			common.SysError("月额度重置检查失败: " + err.Error())
		} else {
			common.SysLog("月额度重置检查完成")
		}
	}
}

// ResetMonthlyQuotaIfNeeded 检查并重置月额度
func ResetMonthlyQuotaIfNeeded() error {
	now := common.GetTimestamp()
	nowTime := time.Unix(now, 0)

	// 查找需要重置月额度的订阅
	var subscriptions []*model.UserSubscription
	err := model.DB.Model(&model.UserSubscription{}).
		Preload("Package").
		Where("status = 1").
		Find(&subscriptions).Error

	if err != nil {
		return err
	}

	resetCount := 0
	for _, subscription := range subscriptions {
		if subscription.Package == nil || subscription.Package.MonthlyQuota <= 0 {
			continue
		}

		lastResetTime := time.Unix(subscription.LastMonthlyReset, 0)

		// 如果当前月份不同于上次重置月份，则重置
		if nowTime.Year() != lastResetTime.Year() || nowTime.Month() != lastResetTime.Month() {
			// 记录重置日志
			resetLog := model.QuotaResetLog{
				UserId:         subscription.UserId,
				SubscriptionId: subscription.Id,
				ResetType:      "monthly",
				ResetTime:      now,
				PreviousUsage:  subscription.MonthlyQuotaUsed,
				NewQuota:       subscription.Package.MonthlyQuota,
				CreatedTime:    now,
			}
			model.DB.Create(&resetLog)

			// 重置月额度
			subscription.MonthlyQuotaUsed = 0
			subscription.LastMonthlyReset = now
			subscription.Update()
			resetCount++
		}
	}

	if resetCount > 0 {
		common.SysLog(fmt.Sprintf("已重置 %d 个订阅的月额度", resetCount))
	}

	return nil
}

// ManualResetDailyQuota 手动重置每日额度（管理员功能）
func ManualResetDailyQuota() error {
	common.SysLog("管理员手动触发每日额度重置")
	return model.ResetDailyQuota()
}

// ManualResetMonthlyQuota 手动重置月额度（管理员功能）
func ManualResetMonthlyQuota() error {
	common.SysLog("管理员手动触发月额度重置")
	return ResetMonthlyQuotaIfNeeded()
}

// DistributeDailyQuota 分发每日额度（已包含在ResetDailyQuota中）
func DistributeDailyQuota() error {
	return model.ResetDailyQuota()
}

// GetQuotaResetLogs 获取额度重置日志
func GetQuotaResetLogs(userId int, resetType string, limit int, offset int) ([]*model.QuotaResetLog, int64, error) {
	var logs []*model.QuotaResetLog
	var total int64

	query := model.DB.Model(&model.QuotaResetLog{})

	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}

	if resetType != "" {
		query = query.Where("reset_type = ?", resetType)
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取数据
	err = query.Order("created_time desc").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error

	return logs, total, err
}

// GetUserQuotaUsage 获取用户额度使用情况
func GetUserQuotaUsage(userId int) (map[string]interface{}, error) {
	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"subscriptions": []map[string]interface{}{},
		"total_usage":   int64(0),
	}

	var totalUsage int64
	var subscriptionData []map[string]interface{}

	for _, subscription := range subscriptions {
		if subscription.Package == nil {
			continue
		}

		usage := map[string]interface{}{
			"subscription_id": subscription.Id,
			"package_name":   subscription.Package.Name,
			"total_usage":    subscription.TotalUsage,
		}

		// 计算各类额度使用情况
		quotaUsage := map[string]interface{}{}

		if subscription.Package.PermanentQuota > 0 {
			quotaUsage["permanent"] = map[string]interface{}{
				"total":     subscription.Package.PermanentQuota,
				"used":      subscription.PermanentQuotaUsed,
				"remaining": subscription.Package.PermanentQuota - subscription.PermanentQuotaUsed,
				"percentage": float64(subscription.PermanentQuotaUsed) / float64(subscription.Package.PermanentQuota) * 100,
			}
		}

		if subscription.Package.MonthlyQuota > 0 {
			quotaUsage["monthly"] = map[string]interface{}{
				"total":     subscription.Package.MonthlyQuota,
				"used":      subscription.MonthlyQuotaUsed,
				"remaining": subscription.Package.MonthlyQuota - subscription.MonthlyQuotaUsed,
				"percentage": float64(subscription.MonthlyQuotaUsed) / float64(subscription.Package.MonthlyQuota) * 100,
			}
		}

		if subscription.Package.DailyQuota > 0 {
			quotaUsage["daily"] = map[string]interface{}{
				"total":     subscription.Package.DailyQuota,
				"used":      subscription.DailyQuotaUsed,
				"remaining": subscription.Package.DailyQuota - subscription.DailyQuotaUsed,
				"percentage": float64(subscription.DailyQuotaUsed) / float64(subscription.Package.DailyQuota) * 100,
			}
		}

		usage["quota_usage"] = quotaUsage
		subscriptionData = append(subscriptionData, usage)
		totalUsage += subscription.TotalUsage
	}

	result["subscriptions"] = subscriptionData
	result["total_usage"] = totalUsage

	return result, nil
}

// CheckAndNotifyQuotaUsage 检查并通知额度使用情况
func CheckAndNotifyQuotaUsage(userId int) error {
	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		return err
	}

	// 获取用户设置以确定通知阈值和方式
	userSetting, err := model.GetUserSetting(userId, false)
	if err != nil {
		return err
	}

	// 如果用户没有设置通知阈值，使用默认值80%
	threshold := userSetting.QuotaWarningThreshold
	if threshold <= 0 {
		threshold = 80.0
	}

	for _, subscription := range subscriptions {
		if subscription.Package == nil {
			continue
		}

		// 检查各类额度是否超过阈值
		warnings := []string{}

		if subscription.Package.PermanentQuota > 0 {
			percentage := float64(subscription.PermanentQuotaUsed) / float64(subscription.Package.PermanentQuota) * 100
			if percentage >= threshold {
				warnings = append(warnings, fmt.Sprintf("永久额度已使用 %.1f%%", percentage))
			}
		}

		if subscription.Package.MonthlyQuota > 0 {
			percentage := float64(subscription.MonthlyQuotaUsed) / float64(subscription.Package.MonthlyQuota) * 100
			if percentage >= threshold {
				warnings = append(warnings, fmt.Sprintf("月额度已使用 %.1f%%", percentage))
			}
		}

		if subscription.Package.DailyQuota > 0 {
			percentage := float64(subscription.DailyQuotaUsed) / float64(subscription.Package.DailyQuota) * 100
			if percentage >= threshold {
				warnings = append(warnings, fmt.Sprintf("日额度已使用 %.1f%%", percentage))
			}
		}

		// 如果有警告，发送通知
		if len(warnings) > 0 {
			message := fmt.Sprintf("套餐 %s 额度警告：%s", subscription.Package.Name, strings.Join(warnings, "，"))
			// 这里可以集成具体的通知服务
			common.SysLog(fmt.Sprintf("用户 %d 额度警告: %s", userId, message))
		}
	}

	return nil
}

// GetSystemQuotaStats 获取系统额度统计（管理员功能）
func GetSystemQuotaStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	// 统计总额度使用量
	var totalUsage int64
	model.DB.Model(&model.UserSubscription{}).
		Where("status = 1").
		Select("SUM(total_usage)").
		Scan(&totalUsage)
	stats["total_usage"] = totalUsage

	// 统计各类型额度使用情况
	var permanentUsage, monthlyUsage, dailyUsage int64
	model.DB.Model(&model.UserSubscription{}).
		Where("status = 1").
		Select("SUM(permanent_quota_used)").
		Scan(&permanentUsage)
	model.DB.Model(&model.UserSubscription{}).
		Where("status = 1").
		Select("SUM(monthly_quota_used)").
		Scan(&monthlyUsage)
	model.DB.Model(&model.UserSubscription{}).
		Where("status = 1").
		Select("SUM(daily_quota_used)").
		Scan(&dailyUsage)

	stats["permanent_usage"] = permanentUsage
	stats["monthly_usage"] = monthlyUsage
	stats["daily_usage"] = dailyUsage

	// 统计各套餐的使用情况
	type PackageStats struct {
		PackageId   int    `json:"package_id"`
		PackageName string `json:"package_name"`
		UserCount   int64  `json:"user_count"`
		TotalUsage  int64  `json:"total_usage"`
	}

	var packageStats []PackageStats
	model.DB.Model(&model.UserSubscription{}).
		Select("package_id, COUNT(*) as user_count, SUM(total_usage) as total_usage").
		Joins("JOIN subscription_packages ON user_subscriptions.package_id = subscription_packages.id").
		Where("user_subscriptions.status = 1").
		Group("package_id").
		Scan(&packageStats)

	// 填充套餐名称
	for i := range packageStats {
		pkg, err := model.GetSubscriptionPackageById(packageStats[i].PackageId)
		if err == nil {
			packageStats[i].PackageName = pkg.Name
		}
	}

	stats["package_stats"] = packageStats

	return stats, nil
}