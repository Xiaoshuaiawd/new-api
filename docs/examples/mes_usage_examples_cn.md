# MES（消息/对话历史）使用示例

本文档提供了如何在 New API 中使用 MES（消息/对话历史）功能的示例。

## 环境变量配置

### 基础配置

```bash
# 主数据库
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES 聊天历史数据库
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages
```

### 启用日期分表的高级配置

```bash
# 主数据库
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# 启用日期分表的 MES 数据库
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages
MES_DAILY_PARTITION=true
```

### PostgreSQL 示例

```bash
# 主数据库（MySQL）
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES 数据库（PostgreSQL）
MES_SQL_DSN=postgres://user:password@localhost:5432/oneapi_messages
```

### Docker Compose 示例

```yaml
version: '3.4'

services:
  new-api:
    image: calciumion/new-api:latest
    container_name: new-api
    restart: always
    ports:
      - "3000:3000"
    environment:
      - SQL_DSN=root:123456@tcp(mysql:3306)/new-api
      - MES_SQL_DSN=root:123456@tcp(mysql:3306)/new-api-mes
      - MES_DAILY_PARTITION=true
      - REDIS_CONN_STRING=redis://redis
      - TZ=Asia/Shanghai
    depends_on:
      - redis
      - mysql

  mysql:
    image: mysql:8.2
    container_name: mysql
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: 123456
      MYSQL_DATABASE: new-api
    volumes:
      - mysql_data:/var/lib/mysql
      # 初始化两个数据库
      - ./init-mes-db.sql:/docker-entrypoint-initdb.d/init-mes-db.sql

volumes:
  mysql_data:
```

### 数据库初始化脚本 (init-mes-db.sql)

```sql
-- 如果不存在则创建 MES 数据库
CREATE DATABASE IF NOT EXISTS `new-api-mes` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

## 代码使用示例

### 示例 1：基础聊天补全与历史保存

```go
package main

import (
    "one-api/model"
    "github.com/gin-gonic/gin"
    "fmt"
)

func handleChatCompletion(c *gin.Context) {
    // 在这里处理你现有的聊天补全逻辑...
    
    // 提取对话详情
    conversationId := "conv_123456789"
    userId := 1
    tokenId := 1
    channelId := 1
    modelName := "gpt-3.5-turbo"
    
    // 来自用户的输入消息
    messages := []map[string]interface{}{
        {
            "role": "user",
            "content": "你好，你怎么样？",
        },
    }
    
    // 模拟来自AI的响应
    response := map[string]interface{}{
        "choices": []interface{}{
            map[string]interface{}{
                "message": map[string]interface{}{
                    "role": "assistant",
                    "content": "你好！我很好，谢谢你的询问。我今天能为你做些什么？",
                },
                "finish_reason": "stop",
            },
        },
        "usage": map[string]interface{}{
            "prompt_tokens": 15,
            "completion_tokens": 25,
            "total_tokens": 40,
        },
    }
    
    // 保存到 MES 数据库
    mesHelper := model.GetMESHelper()
    err := mesHelper.SaveChatCompletion(c, conversationId, messages, response, modelName, userId, tokenId, channelId)
    if err != nil {
        fmt.Printf("保存聊天历史失败: %v\n", err)
    }
}
```

### 示例 2：MES 错误处理

```go
func handleChatCompletionWithError(c *gin.Context) {
    conversationId := "conv_error_123"
    userId := 1
    tokenId := 1
    channelId := 1
    modelName := "gpt-4"
    
    messages := []map[string]interface{}{
        {
            "role": "user",
            "content": "这是一条会引起错误的消息",
        },
    }
    
    // 模拟发生错误
    errorCode := 400
    errorMessage := "无效请求：内容过滤器触发"
    
    // 保存错误到 MES 数据库
    mesHelper := model.GetMESHelper()
    err := mesHelper.SaveErrorConversation(c, conversationId, messages, errorCode, errorMessage, modelName, userId, tokenId, channelId)
    if err != nil {
        fmt.Printf("保存错误对话失败: %v\n", err)
    }
}
```

### 示例 3：检索对话历史

```go
func getConversationHistory(c *gin.Context) {
    conversationId := c.Param("conversation_id")
    limit := 50 // 获取最近50条消息
    
    mesHelper := model.GetMESHelper()
    messages, err := mesHelper.GetConversationMessages(conversationId, limit)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"messages": messages})
}
```

### 示例 4：用户对话管理

```go
func getUserConversations(c *gin.Context) {
    userId := c.GetInt("user_id") // 来自认证中间件
    limit := 20
    offset := 0
    
    mesHelper := model.GetMESHelper()
    conversations, err := mesHelper.GetUserConversations(userId, limit, offset)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"conversations": conversations})
}

func deleteUserConversation(c *gin.Context) {
    userId := c.GetInt("user_id")
    conversationId := c.Param("conversation_id")
    
    mesHelper := model.GetMESHelper()
    err := mesHelper.DeleteUserConversation(userId, conversationId)
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"message": "对话删除成功"})
}
```

### 示例 5：对话统计

```go
func getUserStats(c *gin.Context) {
    userId := c.GetInt("user_id")
    
    mesHelper := model.GetMESHelper()
    stats, err := mesHelper.GetConversationStats(userId)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, stats)
}
```

## API 端点示例

以下是如何将 MES 集成到你的 API 路由中：

```go
// api-router.go
func SetupMESRoutes(router *gin.Engine) {
    api := router.Group("/api/v1")
    api.Use(authMiddleware()) // 你的认证中间件
    
    // 对话管理
    api.GET("/conversations", getUserConversations)
    api.GET("/conversations/:conversation_id", getConversationHistory)
    api.DELETE("/conversations/:conversation_id", deleteUserConversation)
    
    // 统计信息
    api.GET("/stats/conversations", getUserStats)
}
```

## 数据库架构

### 禁用日期分表时

当 `MES_DAILY_PARTITION=false` 或未设置时，会创建以下表：

- `conversation_histories` - 对话历史表
- `error_conversation_histories` - 错误对话历史表

### 启用日期分表时

当 `MES_DAILY_PARTITION=true` 时，表会动态创建：

- `conversation_histories_2025_01_15` - 2025年1月15日的对话历史
- `conversation_histories_2025_01_16` - 2025年1月16日的对话历史
- `error_conversation_histories_2025_01_15` - 2025年1月15日的错误对话历史
- `error_conversation_histories_2025_01_16` - 2025年1月16日的错误对话历史

## 性能考虑

### 日期分表的优势

1. **提高查询性能**：较小的表意味着更快的查询
2. **便于归档**：旧表可以轻松归档或删除
3. **维护便利**：索引重建和维护操作更快

### 日期分表的注意事项

1. **查询复杂度**：跨日期查询需要搜索多个表
2. **存储空间**：每天都会创建新表，需要监控磁盘使用
3. **备份策略**：考虑分别备份旧分区

### 推荐设置

对于高并发应用：
```bash
MES_DAILY_PARTITION=true
SQL_MAX_IDLE_CONNS=50
SQL_MAX_OPEN_CONNS=500
SQL_MAX_LIFETIME=300
```

对于低并发应用：
```bash
MES_DAILY_PARTITION=false
SQL_MAX_IDLE_CONNS=10
SQL_MAX_OPEN_CONNS=100
SQL_MAX_LIFETIME=60
```

## 监控和维护

### 监控 MES 数据库大小

```sql
-- MySQL
SELECT 
    table_schema AS '数据库',
    table_name AS '表名',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS '大小(MB)'
FROM information_schema.TABLES 
WHERE table_schema = 'oneapi_messages'
ORDER BY (data_length + index_length) DESC;
```

### 清理旧分区

```sql
-- 删除30天前的表（请小心操作！）
DROP TABLE IF EXISTS conversation_histories_2024_12_15;
DROP TABLE IF EXISTS error_conversation_histories_2024_12_15;
```

### 归档旧数据

```sql
-- 将旧数据归档到另一个数据库/表
CREATE TABLE archive_conversation_histories_2024_12 
SELECT * FROM conversation_histories_2024_12_01;
-- ... 然后删除原表
```

## 配置示例

### 环境变量完整示例

```bash
# 主数据库配置
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES 数据库配置
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages
MES_DAILY_PARTITION=true

# 数据库连接池配置
SQL_MAX_IDLE_CONNS=50
SQL_MAX_OPEN_CONNS=500
SQL_MAX_LIFETIME=300

# 其他配置
DEBUG=false
REDIS_CONN_STRING=redis://localhost:6379
```

### 不同场景的配置建议

#### 1. 开发环境
```bash
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_dev
MES_SQL_DSN=local  # 使用SQLite
MES_DAILY_PARTITION=false
DEBUG=true
```

#### 2. 测试环境
```bash
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_test
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages_test
MES_DAILY_PARTITION=false
```

#### 3. 生产环境
```bash
SQL_DSN=user:password@tcp(mysql-server:3306)/oneapi_prod
MES_SQL_DSN=user:password@tcp(mysql-server:3306)/oneapi_messages_prod
MES_DAILY_PARTITION=true
SQL_MAX_IDLE_CONNS=100
SQL_MAX_OPEN_CONNS=1000
```

## 故障排除

### 常见问题

1. **MES 数据库连接失败**
   - 检查 `MES_SQL_DSN` 配置是否正确
   - 确认数据库服务器可访问
   - 验证用户权限

2. **表创建失败**
   - 检查数据库用户是否有创建表的权限
   - 确认数据库存在（PostgreSQL需要手动创建）
   - 查看系统日志获取详细错误信息

3. **日期分表查询慢**
   - 考虑减少查询的时间范围
   - 定期清理或归档旧分区
   - 优化查询条件

### 日志分析

系统会记录以下关键信息：
- MES 数据库初始化状态
- 表创建操作
- 查询性能警告
- 错误详情

```bash
# 查看 MES 相关日志
grep "MES" /path/to/logs/system.log
```

## 最佳实践

1. **定期备份**：为 MES 数据库制定独立的备份策略
2. **监控存储**：定期检查数据库大小和分区数量
3. **性能调优**：根据实际使用情况调整连接池参数
4. **数据清理**：制定旧数据的清理和归档策略
5. **安全考虑**：确保聊天历史数据的访问权限控制