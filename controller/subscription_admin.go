package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ManualResetDailyQuota 手动重置每日额度（管理员功能）
func ManualResetDailyQuota(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	err := service.ManualResetDailyQuota()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "重置每日额度失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "手动重置每日额度")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "每日额度重置成功",
	})
}

// ManualResetMonthlyQuota 手动重置月额度（管理员功能）
func ManualResetMonthlyQuota(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	err := service.ManualResetMonthlyQuota()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "重置月额度失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "手动重置月额度")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "月额度重置成功",
	})
}

// GetQuotaResetLogs 获取额度重置日志（管理员功能）
func GetQuotaResetLogs(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	// 获取查询参数
	userIdStr := c.Query("user_id")
	resetType := c.Query("reset_type")
	pageStr := c.Query("page")
	sizeStr := c.Query("size")

	userId := 0
	if userIdStr != "" {
		if uid, err := strconv.Atoi(userIdStr); err == nil {
			userId = uid
		}
	}

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	size := 20
	if sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}

	offset := (page - 1) * size

	logs, total, err := service.GetQuotaResetLogs(userId, resetType, size, offset)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取重置日志失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]interface{}{
			"logs":  logs,
			"total": total,
			"page":  page,
			"size":  size,
		},
	})
}

// GetUserQuotaUsage 获取用户额度使用情况
func GetUserQuotaUsage(c *gin.Context) {
	userId := c.GetInt("id")

	// 如果是管理员且指定了用户ID，则查询指定用户
	if c.GetInt("role") >= common.RoleAdminUser {
		userIdStr := c.Query("user_id")
		if userIdStr != "" {
			if uid, err := strconv.Atoi(userIdStr); err == nil {
				userId = uid
			}
		}
	}

	usage, err := service.GetUserQuotaUsage(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取额度使用情况失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usage,
	})
}

// GetSystemQuotaStats 获取系统额度统计（管理员功能）
func GetSystemQuotaStats(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	stats, err := service.GetSystemQuotaStats()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取系统统计失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// InitSubscriptionScheduler 初始化订阅调度器（系统启动时调用）
func InitSubscriptionScheduler(c *gin.Context) {
	// 检查超级管理员权限
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足，只有超级管理员可以操作",
		})
		return
	}

	service.InitQuotaScheduler()

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "初始化订阅调度器")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订阅调度器已启动",
	})
}

// StopSubscriptionScheduler 停止订阅调度器（系统维护时使用）
func StopSubscriptionScheduler(c *gin.Context) {
	// 检查超级管理员权限
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足，只有超级管理员可以操作",
		})
		return
	}

	service.StopQuotaScheduler()

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "停止订阅调度器")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订阅调度器已停止",
	})
}

// AdminSubscribeUserToPackage 管理员为用户订阅套餐
func AdminSubscribeUserToPackage(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	type AdminSubscribeRequest struct {
		UserId    int `json:"user_id" binding:"required"`
		PackageId int `json:"package_id" binding:"required"`
		Duration  int `json:"duration"` // 可选，如果不指定则使用套餐默认持续时间
	}

	var req AdminSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数: " + err.Error(),
		})
		return
	}

	// 检查用户是否存在
	user, err := model.GetUserById(req.UserId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在: " + err.Error(),
		})
		return
	}

	subscription, err := model.SubscribeToPackage(req.UserId, req.PackageId, req.Duration)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订阅失败: " + err.Error(),
		})
		return
	}

	// 记录日志
	model.RecordLog(c.GetInt("id"), model.LogTypeManage,
		fmt.Sprintf("管理员为用户 %s (ID: %d) 订阅套餐 %s", user.Username, req.UserId, subscription.Package.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户订阅成功",
		"data":    subscription,
	})
}

// AdminCancelUserSubscription 管理员取消用户订阅
func AdminCancelUserSubscription(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅ID",
		})
		return
	}

	// 获取订阅信息
	subscription, err := model.GetUserSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订阅不存在: " + err.Error(),
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
	model.RecordLog(c.GetInt("id"), model.LogTypeManage,
		fmt.Sprintf("管理员取消用户订阅: 用户ID %d, 套餐 %s", subscription.UserId, subscription.Package.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订阅已取消",
	})
}

// GetAllUserSubscriptions 获取所有用户订阅列表（管理员功能）
func GetAllUserSubscriptions(c *gin.Context) {
	// 检查管理员权限
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "权限不足",
		})
		return
	}

	// 获取查询参数
	statusStr := c.Query("status")
	packageIdStr := c.Query("package_id")
	pageStr := c.Query("page")
	sizeStr := c.Query("size")

	status := -1
	if statusStr != "" {
		if s, err := strconv.Atoi(statusStr); err == nil {
			status = s
		}
	}

	packageId := 0
	if packageIdStr != "" {
		if pid, err := strconv.Atoi(packageIdStr); err == nil {
			packageId = pid
		}
	}

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	size := 20
	if sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}

	offset := (page - 1) * size

	// 构建查询
	query := model.DB.Model(&model.UserSubscription{}).
		Preload("Package").
		Preload("User")

	if status >= 0 {
		query = query.Where("status = ?", status)
	}

	if packageId > 0 {
		query = query.Where("package_id = ?", packageId)
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 获取数据
	var subscriptions []*model.UserSubscription
	err := query.Order("created_time desc").
		Limit(size).
		Offset(offset).
		Find(&subscriptions).Error

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
			"total_usage":           subscription.TotalUsage,
			"created_time":          subscription.CreatedTime,
			"updated_time":          subscription.UpdatedTime,
		}

		// 添加用户信息
		if subscription.User != nil {
			item["user"] = map[string]interface{}{
				"id":       subscription.User.Id,
				"username": subscription.User.Username,
				"email":    subscription.User.Email,
				"group":    subscription.User.Group,
			}
		}

		// 添加套餐信息
		if subscription.Package != nil {
			item["package"] = map[string]interface{}{
				"id":              subscription.Package.Id,
				"name":            subscription.Package.Name,
				"permanent_quota": subscription.Package.PermanentQuota,
				"monthly_quota":   subscription.Package.MonthlyQuota,
				"daily_quota":     subscription.Package.DailyQuota,
				"price":           subscription.Package.Price,
				"currency":        subscription.Package.Currency,
			}
		}

		response = append(response, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]interface{}{
			"subscriptions": response,
			"total":         total,
			"page":          page,
			"size":          size,
		},
	})
}