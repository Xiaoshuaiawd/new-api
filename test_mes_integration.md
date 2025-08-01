# MES 集成测试指南

## 测试前准备

### 1. 设置环境变量

```bash
# 基础配置
export SQL_DSN="root:123456@tcp(localhost:3306)/oneapi"

# MES配置 - 使用独立数据库
export MES_SQL_DSN="root:123456@tcp(localhost:3306)/oneapi_mes"

# 可选：启用日期分表
export MES_DAILY_PARTITION=false  # 测试时建议先用false
```

### 2. 启动应用

```bash
# 编译并启动
go build -o new-api
./new-api
```

应该看到以下日志：
```
MES_SQL_DSN not set, chat history will be stored in main database
# 或者
using MES MySQL as database
MES database migration started
MES database initialized successfully
```

## 测试步骤

### 1. 测试正常聊天补全

发送API请求：

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-token-here" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "你好，请介绍一下你自己"
      }
    ]
  }'
```

预期结果：
- API返回正常响应
- 在日志中看到: `MES: 成功保存聊天补全, 对话ID: conv_xxx`

### 2. 测试流式聊天补全

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-token-here" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "请写一首关于春天的诗"
      }
    ],
    "stream": true
  }'
```

预期结果：
- 流式响应正常
- 在日志中看到: `MES流式: 成功保存聊天补全, 对话ID: conv_xxx`

### 3. 测试错误情况

使用无效的model名称：

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-token-here" \
  -d '{
    "model": "invalid-model-name",
    "messages": [
      {
        "role": "user",
        "content": "这应该会失败"
      }
    ]
  }'
```

预期结果：
- API返回错误响应
- 在日志中看到: `MES: 成功保存错误对话, 对话ID: conv_xxx`

## 数据库验证

### 查看存储的数据

1. **正常对话历史**：
```sql
SELECT * FROM conversation_histories ORDER BY created_at DESC LIMIT 10;
```

2. **错误对话历史**：
```sql
SELECT * FROM error_conversation_histories ORDER BY created_at DESC LIMIT 10;
```

3. **如果启用了日期分表**：
```sql
-- 查看今天的表
SELECT * FROM conversation_histories_2025_01_15 ORDER BY created_at DESC LIMIT 10;

-- 查看所有分表
SHOW TABLES LIKE 'conversation_histories_%';
```

### 验证字段

确保以下字段都有正确的值：
- `conversation_id`: 以 "conv_" 开头
- `user_id`: 用户ID
- `token_id`: 令牌ID  
- `channel_id`: 渠道ID
- `model_name`: 模型名称
- `content`: 消息内容
- `role`: 角色（user/assistant）
- `ip`: 客户端IP
- `prompt_tokens`, `completion_tokens`, `total_tokens`: 令牌使用统计

## 性能测试

### 并发测试

使用Apache Bench进行并发测试：

```bash
# 创建测试请求文件
cat > test_request.json << EOF
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user", 
      "content": "性能测试消息"
    }
  ]
}
EOF

# 进行并发测试
ab -n 100 -c 10 -p test_request.json -T application/json \
   -H "Authorization: Bearer sk-your-token-here" \
   http://localhost:3000/v1/chat/completions
```

检查：
- 所有请求都成功保存到MES数据库
- 没有数据丢失或重复
- 日志中没有MES相关错误

## 故障排除

### 常见问题

1. **MES数据库连接失败**
   - 检查MES_SQL_DSN格式是否正确
   - 确认数据库服务器可访问
   - 验证用户权限

2. **表创建失败**
   - 检查用户是否有CREATE权限
   - 对于PostgreSQL，确保数据库已手动创建

3. **保存失败**
   - 查看详细错误日志
   - 检查数据库连接池配置
   - 验证字段长度限制

4. **日期分表问题**
   - 检查分表创建是否成功
   - 验证跨表查询功能
   - 确认时区设置正确

### 调试模式

启用调试模式查看详细信息：

```bash
export DEBUG=true
./new-api
```

这将显示更多MES相关的调试信息。

## 成功标准

测试成功的标准：
- ✅ 正常聊天补全能正确保存到MES数据库
- ✅ 流式聊天补全能正确保存完整响应
- ✅ 错误情况能正确保存到错误表
- ✅ 所有必要字段都有正确值
- ✅ 日期分表功能正常（如果启用）
- ✅ 并发情况下数据完整性保持
- ✅ 性能影响在可接受范围内

如果所有测试都通过，说明MES集成功能已经成功实现！