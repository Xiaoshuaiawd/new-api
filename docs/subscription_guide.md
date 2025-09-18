# 订阅套餐系统使用指南

## 概述

本系统实现了一个完整的订阅套餐功能，支持：
- 自定义套餐配置（永久额度、每月额度、每日额度）
- 自动的每日和每月额度重置（每天晚上12点重置）
- 用户订阅管理
- 额度检查和消费中间件
- 管理员管理功能

## 数据库表结构

### 1. subscription_packages (套餐表)
- id: 套餐ID
- name: 套餐名称
- description: 套餐描述
- permanent_quota: 永久额度
- monthly_quota: 每月额度
- daily_quota: 每日额度
- price: 价格
- currency: 货币类型
- duration: 持续天数
- status: 状态（0-禁用，1-启用）

### 2. user_subscriptions (用户订阅表)
- id: 订阅ID
- user_id: 用户ID
- package_id: 套餐ID
- status: 状态（0-已取消，1-激活，2-暂停，3-已过期）
- start_time: 开始时间
- end_time: 结束时间
- permanent_quota_used: 已使用永久额度
- monthly_quota_used: 已使用月额度
- daily_quota_used: 已使用日额度

### 3. quota_reset_logs (额度重置日志表)
- id: 日志ID
- user_id: 用户ID
- subscription_id: 订阅ID
- reset_type: 重置类型（daily/monthly）
- reset_time: 重置时间
- previous_usage: 重置前使用量
- new_quota: 新额度

## API 接口

### 管理员接口

#### 1. 创建套餐
```http
POST /api/subscription/packages
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "基础套餐",
  "description": "适合个人用户",
  "permanent_quota": 1000000,
  "monthly_quota": 500000,
  "daily_quota": 20000,
  "price": 29.99,
  "currency": "CNY",
  "duration": 30,
  "status": 1,
  "group_limit": ["default"],
  "model_limit": ["gpt-3.5-turbo", "gpt-4"],
  "max_users_per_package": 0,
  "features": {
    "support_level": "basic",
    "api_calls_per_minute": 60
  }
}
```

#### 2. 获取套餐列表
```http
GET /api/subscription/packages?status=1
Authorization: Bearer <admin_token>
```

#### 3. 更新套餐
```http
PUT /api/subscription/packages/{id}
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "高级套餐",
  "monthly_quota": 1000000
}
```

#### 4. 删除套餐
```http
DELETE /api/subscription/packages/{id}
Authorization: Bearer <admin_token>
```

#### 5. 管理员为用户订阅
```http
POST /api/subscription/admin/subscribe
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "user_id": 123,
  "package_id": 1,
  "duration": 30
}
```

#### 6. 手动重置每日额度
```http
POST /api/subscription/admin/reset-daily-quota
Authorization: Bearer <admin_token>
```

#### 7. 手动重置月额度
```http
POST /api/subscription/admin/reset-monthly-quota
Authorization: Bearer <admin_token>
```

### 用户接口

#### 1. 获取可用套餐
```http
GET /api/subscription/packages/active
Authorization: Bearer <user_token>
```

#### 2. 用户订阅套餐
```http
POST /api/subscription/subscribe
Authorization: Bearer <user_token>
Content-Type: application/json

{
  "package_id": 1,
  "duration": 30
}
```

#### 3. 获取用户订阅列表
```http
GET /api/subscription/user/subscriptions
Authorization: Bearer <user_token>
```

#### 4. 获取用户有效订阅
```http
GET /api/subscription/user/active
Authorization: Bearer <user_token>
```

#### 5. 取消订阅
```http
POST /api/subscription/user/cancel/{subscription_id}
Authorization: Bearer <user_token>
```

#### 6. 获取额度使用情况
```http
GET /api/subscription/user/quota-usage
Authorization: Bearer <user_token>
```

## 中间件使用

### 1. 添加额度检查中间件

在需要检查额度的路由上添加中间件：

```go
// 在router中添加
api.Use(middleware.SubscriptionQuotaMiddleware())

// 或者单独使用
api.POST("/chat/completions", middleware.SubscriptionQuotaCheck(), chatHandler)
```

### 2. 在请求中指定所需额度

方式一：通过查询参数
```http
POST /api/chat/completions?quota=1000
```

方式二：通过请求头
```http
POST /api/chat/completions
X-Required-Quota: 1000
```

方式三：系统根据API自动判断（已预设常见接口的默认额度）

## 定时任务

系统自动运行以下定时任务：

1. **每天00:00** - 重置所有用户的每日额度
2. **每小时** - 检查并处理过期订阅
3. **每天01:00** - 检查并重置月额度（月份变化时）

### 启动定时任务
```go
// 在系统启动时调用
service.InitQuotaScheduler()
```

### 停止定时任务
```go
// 在系统关闭时调用
service.StopQuotaScheduler()
```

## 集成步骤

### 1. 数据库迁移
系统启动时会自动创建相关表结构（已集成到model/main.go中）

### 2. 初始化调度器
在main.go中添加：
```go
func main() {
    // ... 其他初始化代码

    // 启动订阅调度器
    service.InitQuotaScheduler()

    // ... 启动服务器

    // 优雅关闭时停止调度器
    defer service.StopQuotaScheduler()
}
```

### 3. 添加路由
在router中添加订阅相关路由：
```go
// 管理员路由
adminGroup := router.Group("/api/subscription")
adminGroup.Use(middleware.AdminRequired())
{
    adminGroup.POST("/packages", controller.CreateSubscriptionPackage)
    adminGroup.GET("/packages", controller.GetSubscriptionPackages)
    adminGroup.GET("/packages/:id", controller.GetSubscriptionPackage)
    adminGroup.PUT("/packages/:id", controller.UpdateSubscriptionPackage)
    adminGroup.DELETE("/packages/:id", controller.DeleteSubscriptionPackage)

    adminGroup.POST("/admin/subscribe", controller.AdminSubscribeUserToPackage)
    adminGroup.POST("/admin/cancel/:id", controller.AdminCancelUserSubscription)
    adminGroup.POST("/admin/reset-daily-quota", controller.ManualResetDailyQuota)
    adminGroup.POST("/admin/reset-monthly-quota", controller.ManualResetMonthlyQuota)
    adminGroup.GET("/admin/stats", controller.GetSubscriptionStats)
    adminGroup.GET("/admin/subscriptions", controller.GetAllUserSubscriptions)
}

// 用户路由
userGroup := router.Group("/api/subscription")
userGroup.Use(middleware.UserRequired())
{
    userGroup.GET("/packages/active", controller.GetActiveSubscriptionPackages)
    userGroup.POST("/subscribe", controller.SubscribeToPackage)
    userGroup.GET("/user/subscriptions", controller.GetUserSubscriptions)
    userGroup.GET("/user/active", controller.GetUserActiveSubscriptions)
    userGroup.POST("/user/cancel/:id", controller.CancelUserSubscription)
    userGroup.GET("/user/quota-usage", controller.GetUserQuotaUsage)
}
```

### 4. 在需要额度检查的API上添加中间件
```go
// 聊天接口
router.POST("/api/chat/completions", middleware.SubscriptionQuotaMiddleware(), chatCompletionsHandler)

// 图片生成接口
router.POST("/api/images/generations", middleware.SubscriptionQuotaMiddleware(), imageGenerationHandler)
```

## 使用示例

### 1. 创建套餐
```bash
curl -X POST "http://localhost:3000/api/subscription/packages" \
  -H "Authorization: Bearer admin_token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "专业套餐",
    "description": "适合企业用户",
    "permanent_quota": 5000000,
    "monthly_quota": 2000000,
    "daily_quota": 100000,
    "price": 99.99,
    "currency": "CNY",
    "duration": 30,
    "status": 1
  }'
```

### 2. 用户订阅
```bash
curl -X POST "http://localhost:3000/api/subscription/subscribe" \
  -H "Authorization: Bearer user_token" \
  -H "Content-Type: application/json" \
  -d '{
    "package_id": 1
  }'
```

### 3. 检查额度使用情况
```bash
curl -X GET "http://localhost:3000/api/subscription/user/quota-usage" \
  -H "Authorization: Bearer user_token"
```

## 注意事项

1. **额度优先级**：系统按照永久额度 > 月额度 > 日额度的优先级消费额度
2. **时区处理**：重置任务基于服务器时区，确保服务器时区设置正确
3. **并发安全**：额度检查和消费使用了数据库事务，确保并发安全
4. **错误处理**：额度不足时会返回明确的错误信息
5. **日志记录**：所有重要操作都会记录到系统日志中

## 扩展功能

系统已预留扩展接口，可以轻松添加：
- 套餐特性配置（API调用频率限制、模型访问权限等）
- 用户组限制
- 支付集成
- 通知系统（额度警告）
- 使用统计和报表