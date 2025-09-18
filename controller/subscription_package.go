package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SubscriptionPackageRequest 创建/更新套餐请求
type SubscriptionPackageRequest struct {
	Name               string                 `json:"name" binding:"required" validate:"required,max=100"`
	Description        string                 `json:"description" validate:"max=1000"`
	PermanentQuota     int64                  `json:"permanent_quota" validate:"min=0"`
	MonthlyQuota       int64                  `json:"monthly_quota" validate:"min=0"`
	DailyQuota         int64                  `json:"daily_quota" validate:"min=0"`
	Price              float64                `json:"price" validate:"min=0"`
	Currency           string                 `json:"currency" validate:"max=10"`
	Duration           int                    `json:"duration" validate:"min=1"`
	Status             int                    `json:"status" validate:"oneof=0 1"`
	GroupLimit         []string               `json:"group_limit"`
	ModelLimit         []string               `json:"model_limit"`
	MaxUsersPerPackage int                    `json:"max_users_per_package" validate:"min=0"`
	Features           map[string]interface{} `json:"features"`
	SortOrder          int                    `json:"sort_order"`
}

// CreateSubscriptionPackage 创建订阅套餐
func CreateSubscriptionPackage(c *gin.Context) {
	var req SubscriptionPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数: " + err.Error(),
		})
		return
	}

	// 验证请求
	if err := common.Validate.Struct(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法: " + err.Error(),
		})
		return
	}

	// 验证至少有一种额度
	if req.PermanentQuota == 0 && req.MonthlyQuota == 0 && req.DailyQuota == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "至少需要设置一种额度（永久额度、每月额度或每日额度）",
		})
		return
	}

	// 如果设置了每日额度但没有每月额度，提醒用户
	if req.DailyQuota > 0 && req.MonthlyQuota == 0 {
		common.SysLog("警告：套餐设置了每日额度但没有每月额度，每日额度将独立计算")
	}

	// 如果每日额度大于每月额度，给出警告
	if req.DailyQuota > 0 && req.MonthlyQuota > 0 && req.DailyQuota*30 > req.MonthlyQuota {
		common.SysLog("警告：每日额度*30天大于每月额度，这可能导致用户无法充分使用每月额度")
	}

	pkg := &model.SubscriptionPackage{
		Name:               req.Name,
		Description:        req.Description,
		PermanentQuota:     req.PermanentQuota,
		MonthlyQuota:       req.MonthlyQuota,
		DailyQuota:         req.DailyQuota,
		Price:              req.Price,
		Currency:           req.Currency,
		Duration:           req.Duration,
		Status:             req.Status,
		MaxUsersPerPackage: req.MaxUsersPerPackage,
		SortOrder:          req.SortOrder,
		CreatedBy:          c.GetInt("id"),
	}

	// 设置组限制
	if len(req.GroupLimit) > 0 {
		pkg.SetGroupLimitList(req.GroupLimit)
	}

	// 设置模型限制
	if len(req.ModelLimit) > 0 {
		pkg.SetModelLimitList(req.ModelLimit)
	}

	// 设置特性
	if len(req.Features) > 0 {
		pkg.SetFeatures(req.Features)
	}

	if err := pkg.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "创建套餐失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("创建订阅套餐: %s", pkg.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐创建成功",
		"data":    pkg,
	})
}

// GetSubscriptionPackages 获取订阅套餐列表
func GetSubscriptionPackages(c *gin.Context) {
	statusStr := c.Query("status")
	status := -1 // 默认获取所有状态
	if statusStr != "" {
		if s, err := strconv.Atoi(statusStr); err == nil {
			status = s
		}
	}

	packages, err := model.GetAllSubscriptionPackages(status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取套餐列表失败: " + err.Error(),
		})
		return
	}

	// 转换为响应格式
	var response []map[string]interface{}
	for _, pkg := range packages {
		item := map[string]interface{}{
			"id":                     pkg.Id,
			"name":                   pkg.Name,
			"description":            pkg.Description,
			"permanent_quota":        pkg.PermanentQuota,
			"monthly_quota":          pkg.MonthlyQuota,
			"daily_quota":            pkg.DailyQuota,
			"price":                  pkg.Price,
			"currency":               pkg.Currency,
			"duration":               pkg.Duration,
			"status":                 pkg.Status,
			"group_limit":            pkg.GetGroupLimitList(),
			"model_limit":            pkg.GetModelLimitList(),
			"max_users_per_package":  pkg.MaxUsersPerPackage,
			"features":               pkg.GetFeatures(),
			"sort_order":             pkg.SortOrder,
			"created_time":           pkg.CreatedTime,
			"updated_time":           pkg.UpdatedTime,
			"created_by":             pkg.CreatedBy,
		}
		response = append(response, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetActiveSubscriptionPackages 获取启用的订阅套餐（供用户选择）
func GetActiveSubscriptionPackages(c *gin.Context) {
	packages, err := model.GetActiveSubscriptionPackages()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取套餐列表失败: " + err.Error(),
		})
		return
	}

	// 获取当前用户信息以过滤可用套餐
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户信息失败: " + err.Error(),
		})
		return
	}

	// 过滤用户可订阅的套餐
	var availablePackages []map[string]interface{}
	for _, pkg := range packages {
		// 检查用户组限制
		groupLimits := pkg.GetGroupLimitList()
		if len(groupLimits) > 0 {
			allowed := false
			for _, group := range groupLimits {
				if user.Group == group {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		// 检查套餐用户数限制
		if pkg.MaxUsersPerPackage > 0 {
			var count int64
			model.DB.Model(&model.UserSubscription{}).Where("package_id = ? AND status = 1", pkg.Id).Count(&count)
			if int(count) >= pkg.MaxUsersPerPackage {
				continue // 套餐已满，跳过
			}
		}

		item := map[string]interface{}{
			"id":                pkg.Id,
			"name":              pkg.Name,
			"description":       pkg.Description,
			"permanent_quota":   pkg.PermanentQuota,
			"monthly_quota":     pkg.MonthlyQuota,
			"daily_quota":       pkg.DailyQuota,
			"price":             pkg.Price,
			"currency":          pkg.Currency,
			"duration":          pkg.Duration,
			"features":          pkg.GetFeatures(),
			"sort_order":        pkg.SortOrder,
		}
		availablePackages = append(availablePackages, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    availablePackages,
	})
}

// GetSubscriptionPackage 获取单个订阅套餐详情
func GetSubscriptionPackage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐ID",
		})
		return
	}

	pkg, err := model.GetSubscriptionPackageById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取套餐详情失败: " + err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"id":                     pkg.Id,
		"name":                   pkg.Name,
		"description":            pkg.Description,
		"permanent_quota":        pkg.PermanentQuota,
		"monthly_quota":          pkg.MonthlyQuota,
		"daily_quota":            pkg.DailyQuota,
		"price":                  pkg.Price,
		"currency":               pkg.Currency,
		"duration":               pkg.Duration,
		"status":                 pkg.Status,
		"group_limit":            pkg.GetGroupLimitList(),
		"model_limit":            pkg.GetModelLimitList(),
		"max_users_per_package":  pkg.MaxUsersPerPackage,
		"features":               pkg.GetFeatures(),
		"sort_order":             pkg.SortOrder,
		"created_time":           pkg.CreatedTime,
		"updated_time":           pkg.UpdatedTime,
		"created_by":             pkg.CreatedBy,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// UpdateSubscriptionPackage 更新订阅套餐
func UpdateSubscriptionPackage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐ID",
		})
		return
	}

	// 获取现有套餐
	pkg, err := model.GetSubscriptionPackageById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐不存在: " + err.Error(),
		})
		return
	}

	var req SubscriptionPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数: " + err.Error(),
		})
		return
	}

	// 验证请求
	if err := common.Validate.Struct(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法: " + err.Error(),
		})
		return
	}

	// 验证至少有一种额度
	if req.PermanentQuota == 0 && req.MonthlyQuota == 0 && req.DailyQuota == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "至少需要设置一种额度（永久额度、每月额度或每日额度）",
		})
		return
	}

	// 更新套餐信息
	pkg.Name = req.Name
	pkg.Description = req.Description
	pkg.PermanentQuota = req.PermanentQuota
	pkg.MonthlyQuota = req.MonthlyQuota
	pkg.DailyQuota = req.DailyQuota
	pkg.Price = req.Price
	pkg.Currency = req.Currency
	pkg.Duration = req.Duration
	pkg.Status = req.Status
	pkg.MaxUsersPerPackage = req.MaxUsersPerPackage
	pkg.SortOrder = req.SortOrder

	// 设置组限制
	pkg.SetGroupLimitList(req.GroupLimit)

	// 设置模型限制
	pkg.SetModelLimitList(req.ModelLimit)

	// 设置特性
	pkg.SetFeatures(req.Features)

	if err := pkg.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新套餐失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("更新订阅套餐: %s", pkg.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐更新成功",
		"data":    pkg,
	})
}

// DeleteSubscriptionPackage 删除订阅套餐
func DeleteSubscriptionPackage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐ID",
		})
		return
	}

	// 获取套餐信息
	pkg, err := model.GetSubscriptionPackageById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐不存在: " + err.Error(),
		})
		return
	}

	// 检查是否有用户正在使用此套餐
	var activeCount int64
	model.DB.Model(&model.UserSubscription{}).Where("package_id = ? AND status = 1", id).Count(&activeCount)
	if activeCount > 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("无法删除套餐，还有 %d 个用户正在使用此套餐", activeCount),
		})
		return
	}

	if err := pkg.Delete(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "删除套餐失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("删除订阅套餐: %s", pkg.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐删除成功",
	})
}

// UserSubscribeRequest 用户订阅请求
type UserSubscribeRequest struct {
	PackageId int `json:"package_id" binding:"required"`
	Duration  int `json:"duration"` // 可选，如果不指定则使用套餐默认持续时间
}

// SubscribeToPackage 用户订阅套餐
func SubscribeToPackage(c *gin.Context) {
	var req UserSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数: " + err.Error(),
		})
		return
	}

	userId := c.GetInt("id")

	subscription, err := model.SubscribeToPackage(userId, req.PackageId, req.Duration)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订阅失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订阅成功",
		"data":    subscription,
	})
}

// GetUserSubscriptions 获取用户订阅列表
func GetUserSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")

	// 如果是管理员且指定了用户ID，则查询指定用户的订阅
	if c.GetInt("role") >= common.RoleAdminUser {
		userIdStr := c.Query("user_id")
		if userIdStr != "" {
			if uid, err := strconv.Atoi(userIdStr); err == nil {
				userId = uid
			}
		}
	}

	statusStr := c.Query("status")
	status := -1 // 默认获取所有状态
	if statusStr != "" {
		if s, err := strconv.Atoi(statusStr); err == nil {
			status = s
		}
	}

	subscriptions, err := model.GetUserSubscriptions(userId, status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取订阅列表失败: " + err.Error(),
		})
		return
	}

	// 转换响应格式
	var response []map[string]interface{}
	for _, subscription := range subscriptions {
		item := map[string]interface{}{
			"id":                    subscription.Id,
			"user_id":               subscription.UserId,
			"package_id":            subscription.PackageId,
			"status":                subscription.Status,
			"start_time":            subscription.StartTime,
			"end_time":              subscription.EndTime,
			"permanent_quota_used":  subscription.PermanentQuotaUsed,
			"monthly_quota_used":    subscription.MonthlyQuotaUsed,
			"daily_quota_used":      subscription.DailyQuotaUsed,
			"last_monthly_reset":    subscription.LastMonthlyReset,
			"last_daily_reset":      subscription.LastDailyReset,
			"total_usage":           subscription.TotalUsage,
			"created_time":          subscription.CreatedTime,
			"updated_time":          subscription.UpdatedTime,
		}

		// 添加套餐信息
		if subscription.Package != nil {
			item["package"] = map[string]interface{}{
				"id":              subscription.Package.Id,
				"name":            subscription.Package.Name,
				"description":     subscription.Package.Description,
				"permanent_quota": subscription.Package.PermanentQuota,
				"monthly_quota":   subscription.Package.MonthlyQuota,
				"daily_quota":     subscription.Package.DailyQuota,
				"price":           subscription.Package.Price,
				"currency":        subscription.Package.Currency,
				"duration":        subscription.Package.Duration,
			}
		}

		response = append(response, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetUserActiveSubscriptions 获取用户有效订阅
func GetUserActiveSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")

	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取有效订阅失败: " + err.Error(),
		})
		return
	}

	var response []map[string]interface{}
	for _, subscription := range subscriptions {
		// 计算剩余额度
		remainingQuotas := map[string]interface{}{}

		if subscription.Package != nil {
			if subscription.Package.PermanentQuota > 0 {
				remainingQuotas["permanent"] = subscription.Package.PermanentQuota - subscription.PermanentQuotaUsed
			}
			if subscription.Package.MonthlyQuota > 0 {
				remainingQuotas["monthly"] = subscription.Package.MonthlyQuota - subscription.MonthlyQuotaUsed
			}
			if subscription.Package.DailyQuota > 0 {
				remainingQuotas["daily"] = subscription.Package.DailyQuota - subscription.DailyQuotaUsed
			}
		}

		item := map[string]interface{}{
			"id":                    subscription.Id,
			"package_id":            subscription.PackageId,
			"status":                subscription.Status,
			"start_time":            subscription.StartTime,
			"end_time":              subscription.EndTime,
			"permanent_quota_used":  subscription.PermanentQuotaUsed,
			"monthly_quota_used":    subscription.MonthlyQuotaUsed,
			"daily_quota_used":      subscription.DailyQuotaUsed,
			"total_usage":           subscription.TotalUsage,
			"remaining_quotas":      remainingQuotas,
			"created_time":          subscription.CreatedTime,
		}

		// 添加套餐信息
		if subscription.Package != nil {
			item["package"] = map[string]interface{}{
				"id":              subscription.Package.Id,
				"name":            subscription.Package.Name,
				"description":     subscription.Package.Description,
				"permanent_quota": subscription.Package.PermanentQuota,
				"monthly_quota":   subscription.Package.MonthlyQuota,
				"daily_quota":     subscription.Package.DailyQuota,
				"features":        subscription.Package.GetFeatures(),
			}
		}

		response = append(response, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// CancelUserSubscription 取消用户订阅
func CancelUserSubscription(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅ID",
		})
		return
	}

	userId := c.GetInt("id")

	// 获取订阅信息
	subscription, err := model.GetUserSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订阅不存在: " + err.Error(),
		})
		return
	}

	// 检查权限：只有订阅用户本人或管理员可以取消订阅
	if subscription.UserId != userId && c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权操作此订阅",
		})
		return
	}

	// 更新订阅状态为已取消
	subscription.Status = 0
	if err := subscription.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "取消订阅失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(subscription.UserId, model.LogTypeSystem, fmt.Sprintf("取消订阅套餐: %s", subscription.Package.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订阅已取消",
	})
}

// GetSubscriptionStats 获取订阅统计信息（管理员功能）
func GetSubscriptionStats(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	stats := map[string]interface{}{}

	// 统计套餐数量
	var packageCount int64
	model.DB.Model(&model.SubscriptionPackage{}).Count(&packageCount)
	stats["total_packages"] = packageCount

	var activePackageCount int64
	model.DB.Model(&model.SubscriptionPackage{}).Where("status = 1").Count(&activePackageCount)
	stats["active_packages"] = activePackageCount

	// 统计订阅数量
	var subscriptionCount int64
	model.DB.Model(&model.UserSubscription{}).Count(&subscriptionCount)
	stats["total_subscriptions"] = subscriptionCount

	var activeSubscriptionCount int64
	model.DB.Model(&model.UserSubscription{}).Where("status = 1").Count(&activeSubscriptionCount)
	stats["active_subscriptions"] = activeSubscriptionCount

	var expiredSubscriptionCount int64
	model.DB.Model(&model.UserSubscription{}).Where("status = 3").Count(&expiredSubscriptionCount)
	stats["expired_subscriptions"] = expiredSubscriptionCount

	// 统计总收入（假设所有订阅都已付费）
	var totalRevenue float64
	model.DB.Model(&model.UserSubscription{}).
		Joins("JOIN subscription_packages ON user_subscriptions.package_id = subscription_packages.id").
		Select("SUM(subscription_packages.price)").
		Scan(&totalRevenue)
	stats["total_revenue"] = totalRevenue

	// 统计用户订阅数量
	var subscribedUserCount int64
	model.DB.Model(&model.UserSubscription{}).
		Select("DISTINCT user_id").
		Where("status = 1").
		Count(&subscribedUserCount)
	stats["subscribed_users"] = subscribedUserCount

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// CheckUserQuota 检查用户订阅额度（内部接口，供其他服务调用）
func CheckUserQuota(userId int, requiredQuota int64) (bool, string, error) {
	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		return false, "", err
	}

	if len(subscriptions) == 0 {
		return false, "用户没有有效的订阅套餐", nil
	}

	// 按优先级检查额度：先检查有永久额度的，再检查月额度，最后检查日额度
	for _, subscription := range subscriptions {
		available, reason, err := subscription.CheckQuotaAvailable(requiredQuota)
		if err != nil {
			continue // 检查下一个订阅
		}
		if available {
			return true, "", nil
		}
		// 记录第一个检查失败的原因
		if reason != "" {
			return false, reason, nil
		}
	}

	return false, "所有订阅套餐的额度都不足", nil
}

// ConsumeUserQuota 消费用户订阅额度
func ConsumeUserQuota(userId int, quota int64) error {
	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		return err
	}

	if len(subscriptions) == 0 {
		return fmt.Errorf("用户没有有效的订阅套餐")
	}

	// 按优先级消费额度：优先消费永久额度，然后是月额度，最后是日额度
	for _, subscription := range subscriptions {
		available, _, err := subscription.CheckQuotaAvailable(quota)
		if err != nil {
			continue
		}
		if available {
			return subscription.ConsumeQuota(quota)
		}
	}

	return fmt.Errorf("所有订阅套餐的额度都不足")
}