package middleware

import (
	"fmt"
	"net/http"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SubscriptionQuotaCheck 订阅额度检查中间件
func SubscriptionQuotaCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过不需要检查额度的路径
		if shouldSkipQuotaCheck(c.Request.URL.Path) {
			c.Next()
			return
		}

		userId := c.GetInt("id")
		if userId == 0 {
			// 未登录用户，跳过检查
			c.Next()
			return
		}

		// 从请求中获取需要的额度量（这里需要根据具体的API来确定）
		requiredQuota := getRequiredQuotaFromRequest(c)
		if requiredQuota <= 0 {
			// 如果无法确定所需额度，跳过检查
			c.Next()
			return
		}

		// 检查用户订阅额度
		available, reason, err := checkUserQuota(userId, requiredQuota)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "检查订阅额度失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		if !available {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "订阅额度不足: " + reason,
				"error":   "SUBSCRIPTION_QUOTA_INSUFFICIENT",
			})
			c.Abort()
			return
		}

		// 额度检查通过，继续处理请求
		c.Set("required_quota", requiredQuota)
		c.Next()
	}
}

// ConsumeSubscriptionQuota 消费订阅额度中间件（在请求成功处理后调用）
func ConsumeSubscriptionQuota() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // 先处理请求

		// 只有在请求成功时才消费额度
		if c.Writer.Status() == http.StatusOK {
			userId := c.GetInt("id")
			if userId > 0 {
				requiredQuota, exists := c.Get("required_quota")
				if exists {
					if quota, ok := requiredQuota.(int64); ok && quota > 0 {
						err := consumeUserQuota(userId, quota)
						if err != nil {
							// 记录错误但不阻止响应
							// 可以考虑添加到日志或监控系统
						}
					}
				}
			}
		}
	}
}

// shouldSkipQuotaCheck 判断是否应该跳过额度检查
func shouldSkipQuotaCheck(path string) bool {
	// 定义不需要检查额度的路径列表
	skipPaths := []string{
		"/api/user/login",
		"/api/user/register",
		"/api/user/logout",
		"/api/user/subscription", // 订阅相关接口
		"/api/subscription",      // 订阅套餐接口
		"/api/admin",             // 管理员接口
		"/api/system",            // 系统接口
		"/api/health",            // 健康检查
	}

	for _, skipPath := range skipPaths {
		if path == skipPath || (len(path) > len(skipPath) && path[:len(skipPath)] == skipPath) {
			return true
		}
	}

	return false
}

// getRequiredQuotaFromRequest 从请求中获取所需的额度量
func getRequiredQuotaFromRequest(c *gin.Context) int64 {
	// 这里需要根据具体的API来确定所需额度
	// 可以从请求体、查询参数或请求头中获取

	// 示例：从查询参数中获取
	if quotaStr := c.Query("quota"); quotaStr != "" {
		if quota, err := strconv.ParseInt(quotaStr, 10, 64); err == nil {
			return quota
		}
	}

	// 示例：从请求头中获取
	if quotaStr := c.GetHeader("X-Required-Quota"); quotaStr != "" {
		if quota, err := strconv.ParseInt(quotaStr, 10, 64); err == nil {
			return quota
		}
	}

	// 默认额度（可以根据API类型设置不同的默认值）
	switch c.Request.URL.Path {
	case "/api/chat/completions":
		return 1000 // 聊天接口默认1000额度
	case "/api/images/generations":
		return 5000 // 图片生成接口默认5000额度
	case "/api/audio/transcriptions":
		return 2000 // 音频转录接口默认2000额度
	default:
		return 100 // 其他接口默认100额度
	}
}

// SubscriptionQuotaMiddleware 组合中间件，包含检查和消费
func SubscriptionQuotaMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 先检查额度
		SubscriptionQuotaCheck()(c)
		if c.IsAborted() {
			return
		}

		// 处理请求
		c.Next()

		// 请求完成后消费额度
		ConsumeSubscriptionQuota()(c)
	})
}

// AdminQuotaManagement 管理员额度管理中间件
func AdminQuotaManagement() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查管理员权限
		role := c.GetInt("role")
		if role < 10 { // 假设10是管理员权限
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "权限不足",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetUserQuotaStatus 获取用户额度状态
func GetUserQuotaStatus(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户未登录",
		})
		return
	}

	// 获取用户活跃订阅
	subscriptions, err := model.GetActiveUserSubscriptions(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取订阅信息失败: " + err.Error(),
		})
		return
	}

	if len(subscriptions) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "用户没有活跃的订阅",
			"data": map[string]interface{}{
				"has_subscription": false,
				"subscriptions":    []interface{}{},
			},
		})
		return
	}

	// 构建响应数据
	var quotaStatus []map[string]interface{}
	for _, subscription := range subscriptions {
		if subscription.Package == nil {
			continue
		}

		status := map[string]interface{}{
			"subscription_id": subscription.Id,
			"package_name":   subscription.Package.Name,
			"status":         subscription.Status,
			"end_time":       subscription.EndTime,
		}

		quotas := map[string]interface{}{}
		if subscription.Package.PermanentQuota > 0 {
			quotas["permanent"] = map[string]interface{}{
				"total":     subscription.Package.PermanentQuota,
				"used":      subscription.PermanentQuotaUsed,
				"remaining": subscription.Package.PermanentQuota - subscription.PermanentQuotaUsed,
			}
		}
		if subscription.Package.MonthlyQuota > 0 {
			quotas["monthly"] = map[string]interface{}{
				"total":     subscription.Package.MonthlyQuota,
				"used":      subscription.MonthlyQuotaUsed,
				"remaining": subscription.Package.MonthlyQuota - subscription.MonthlyQuotaUsed,
			}
		}
		if subscription.Package.DailyQuota > 0 {
			quotas["daily"] = map[string]interface{}{
				"total":     subscription.Package.DailyQuota,
				"used":      subscription.DailyQuotaUsed,
				"remaining": subscription.Package.DailyQuota - subscription.DailyQuotaUsed,
			}
		}

		status["quotas"] = quotas
		quotaStatus = append(quotaStatus, status)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]interface{}{
			"has_subscription": true,
			"subscriptions":    quotaStatus,
		},
	})
}

// checkUserQuota 检查用户订阅额度（内部函数）
func checkUserQuota(userId int, requiredQuota int64) (bool, string, error) {
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

// consumeUserQuota 消费用户订阅额度（内部函数）
func consumeUserQuota(userId int, quota int64) error {
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