# MES API 中文文档

## 核心函数说明

### MESHelper 辅助类

`MESHelper` 是为 MES（消息/对话历史）功能提供的便捷操作类。

#### 主要方法

#### 1. SaveChatCompletion - 保存聊天补全

```go
func (h *MESHelper) SaveChatCompletion(
    c *gin.Context,           // Gin 上下文
    conversationId string,    // 对话 ID
    messages []map[string]interface{}, // 用户消息列表
    response map[string]interface{},   // AI 响应
    modelName string,         // 模型名称
    userId int,              // 用户 ID
    tokenId int,             // 令牌 ID
    channelId int            // 渠道 ID
) error
```

**功能**：将完整的聊天对话（包括用户消息和 AI 响应）保存到 MES 数据库。

**使用示例**：
```go
mesHelper := model.GetMESHelper()
err := mesHelper.SaveChatCompletion(c, "conv_123", messages, response, "gpt-3.5-turbo", 1, 1, 1)
```

#### 2. SaveErrorConversation - 保存错误对话

```go
func (h *MESHelper) SaveErrorConversation(
    c *gin.Context,           // Gin 上下文
    conversationId string,    // 对话 ID
    messages []map[string]interface{}, // 导致错误的消息
    errorCode int,           // 错误代码
    errorMessage string,     // 错误消息
    modelName string,        // 模型名称
    userId int,              // 用户 ID
    tokenId int,             // 令牌 ID
    channelId int            // 渠道 ID
) error
```

**功能**：保存导致错误的对话到专用的错误表中。

#### 3. GetConversationMessages - 获取对话消息

```go
func (h *MESHelper) GetConversationMessages(
    conversationId string,   // 对话 ID
    limit int               // 消息数量限制
) ([]map[string]interface{}, error)
```

**功能**：获取指定对话的消息列表，返回 OpenAI 兼容格式。

#### 4. GetUserConversations - 获取用户对话

```go
func (h *MESHelper) GetUserConversations(
    userId int,    // 用户 ID
    limit int,     // 限制数量
    offset int     // 偏移量
) ([]*ConversationHistory, error)
```

**功能**：获取用户的对话历史列表。

#### 5. DeleteUserConversation - 删除用户对话

```go
func (h *MESHelper) DeleteUserConversation(
    userId int,              // 用户 ID
    conversationId string    // 对话 ID
) error
```

**功能**：删除用户的指定对话（包含权限检查）。

#### 6. GetConversationStats - 获取对话统计

```go
func (h *MESHelper) GetConversationStats(
    userId int    // 用户 ID
) (map[string]interface{}, error)
```

**功能**：获取用户的对话使用统计信息。

**返回数据示例**：
```json
{
    "total_conversations": 15,
    "total_messages": 150,
    "total_tokens": 50000,
    "models_used": {
        "gpt-3.5-turbo": 100,
        "gpt-4": 50
    },
    "daily_message_count": {
        "2025-01-15": 20,
        "2025-01-14": 30
    }
}
```

### 核心数据库函数

#### SaveConversationHistory - 保存对话历史

```go
func SaveConversationHistory(history *ConversationHistory) error
```

**功能**：将单条对话历史记录保存到数据库。

#### SaveErrorConversationHistory - 保存错误对话历史

```go
func SaveErrorConversationHistory(history *ErrorConversationHistory) error
```

**功能**：将错误对话历史记录保存到数据库。

#### GetConversationHistory - 获取对话历史

```go
func GetConversationHistory(
    conversationId string,   // 对话 ID
    limit int,              // 限制数量
    offset int              // 偏移量
) ([]*ConversationHistory, error)
```

**功能**：根据对话 ID 获取对话历史记录。

#### DeleteConversationHistory - 删除对话历史

```go
func DeleteConversationHistory(conversationId string) error
```

**功能**：删除指定对话的所有历史记录。

### 日期分表相关函数

#### getConversationHistoryTableName - 获取对话历史表名

```go
func getConversationHistoryTableName(date ...time.Time) string
```

**功能**：
- 如果禁用日期分表：返回 `conversation_histories`
- 如果启用日期分表：返回 `conversation_histories_2025_01_15` 格式

#### createTableIfNotExists - 创建表（如果不存在）

```go
func createTableIfNotExists(tableName string, model interface{}) error
```

**功能**：为日期分表动态创建表。

#### getExistingPartitionTables - 获取现有分区表

```go
func getExistingPartitionTables(prefix string) ([]string, error)
```

**功能**：获取所有现有的分区表列表，用于跨表查询。

## 数据模型

### ConversationHistory - 对话历史模型

```go
type ConversationHistory struct {
    Id               int    // 主键 ID
    UserId           int    // 用户 ID
    CreatedAt        int64  // 创建时间
    UpdatedAt        int64  // 更新时间
    ConversationId   string // 对话 ID
    MessageId        string // 消息 ID
    Role             string // 角色（user/assistant/system）
    Content          string // 消息内容
    ModelName        string // AI 模型名称
    TokenId          int    // 令牌 ID
    ChannelId        int    // 渠道 ID
    PromptTokens     int    // 提示词令牌数
    CompletionTokens int    // 补全令牌数
    TotalTokens      int    // 总令牌数
    IsStream         bool   // 是否流式输出
    FinishReason     string // 完成原因
    Usage            string // 使用情况 JSON
    Other            string // 其他元数据 JSON
    Ip               string // 客户端 IP
}
```

### ErrorConversationHistory - 错误对话历史模型

```go
type ErrorConversationHistory struct {
    Id               int    // 主键 ID
    UserId           int    // 用户 ID
    CreatedAt        int64  // 创建时间
    ConversationId   string // 对话 ID
    MessageId        string // 消息 ID
    Role             string // 角色
    Content          string // 消息内容
    ModelName        string // AI 模型名称
    TokenId          int    // 令牌 ID
    ChannelId        int    // 渠道 ID
    ErrorCode        int    // 错误代码
    ErrorMessage     string // 错误消息
    PromptTokens     int    // 提示词令牌数
    CompletionTokens int    // 补全令牌数
    TotalTokens      int    // 总令牌数
    Other            string // 其他元数据 JSON
    Ip               string // 客户端 IP
}
```

## 环境变量配置

### 基础配置

```bash
# MES 数据库连接字符串
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages

# 启用日期分表（可选）
MES_DAILY_PARTITION=true
```

### 数据库类型支持

1. **MySQL**：
   ```bash
   MES_SQL_DSN=username:password@tcp(host:port)/database_name
   ```

2. **PostgreSQL**：
   ```bash
   MES_SQL_DSN=postgres://username:password@host:port/database_name
   ```

3. **SQLite**：
   ```bash
   MES_SQL_DSN=local
   ```

## 使用流程

### 1. 系统启动时

1. 检查 `MES_SQL_DSN` 环境变量
2. 如果设置了，初始化独立的 MES 数据库连接
3. 如果未设置，使用主数据库存储聊天历史
4. 执行数据库迁移（创建必要的表）

### 2. 聊天补全时

1. 处理用户请求
2. 调用 AI 服务获取响应
3. 使用 `SaveChatCompletion` 保存完整对话
4. 返回响应给用户

### 3. 错误处理时

1. 检测到错误
2. 使用 `SaveErrorConversation` 保存错误信息
3. 返回错误响应

### 4. 查询历史时

1. 验证用户权限
2. 使用 `GetConversationMessages` 获取对话
3. 返回格式化的消息列表

## 性能优化

### 日期分表优势

- **查询性能**：小表查询更快
- **维护便利**：按日期归档数据
- **存储管理**：灵活的存储策略

### 连接池配置

```bash
SQL_MAX_IDLE_CONNS=50      # 最大空闲连接数
SQL_MAX_OPEN_CONNS=500     # 最大开放连接数
SQL_MAX_LIFETIME=300       # 连接最大生存时间（秒）
```

## 监控和维护

### 表大小监控

```sql
-- 查看 MES 数据库中所有表的大小
SELECT 
    table_name AS '表名',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS '大小(MB)'
FROM information_schema.TABLES 
WHERE table_schema = 'oneapi_messages'
ORDER BY (data_length + index_length) DESC;
```

### 分区清理

```sql
-- 删除30天前的分区表
DROP TABLE IF EXISTS conversation_histories_2024_12_15;
DROP TABLE IF EXISTS error_conversation_histories_2024_12_15;
```

## 错误处理

### 常见错误和解决方案

1. **数据库连接失败**
   - 检查 `MES_SQL_DSN` 配置
   - 验证数据库服务状态
   - 确认网络连接

2. **权限不足**
   - 确保数据库用户有创建表权限
   - 验证读写权限

3. **表创建失败**
   - 检查数据库是否存在
   - 验证字符集设置

## 安全考虑

1. **数据隔离**：聊天历史与业务数据分离
2. **访问控制**：用户只能访问自己的对话
3. **IP 记录**：记录客户端 IP 用于审计
4. **权限验证**：所有操作都有权限检查