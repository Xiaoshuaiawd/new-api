# 日志查询接口文档

## 接口概述

**接口名称**: 根据Token查询日志  
**接口路径**: `/api/log/token`  
**请求方法**: `GET`  
**鉴权要求**: 无需鉴权  
**适用场景**: 查询指定Token的使用日志，支持大数据量高性能查询  

## 查询模式

本接口支持三种查询模式，根据数据量选择最适合的方式：

### 1. 普通分页模式
适用于小数据量（< 10万条）

### 2. 轻量级查询模式  
适用于中等数据量（10-100万条），只返回核心字段，查询速度更快

### 3. 游标分页模式
适用于大数据量（100万+条），使用时间戳游标实现高性能分页

## 请求参数

### 基础参数

| 参数名 | 类型 | 必填 | 说明 | 示例 |
|--------|------|------|------|------|
| `key` | string | ✅ | Token密钥 | `sk-LYjCRscuufA465EuJCADmweH7OnCnECg9ZzVJ8ZSYjsJ2Gru` |

### 分页参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| `page` | int | ❌ | 1 | 页码（普通分页模式） |
| `page_size` | int | ❌ | 20 | 每页大小，无上限 |

### 筛选参数

| 参数名 | 类型 | 必填 | 说明 | 示例 |
|--------|------|------|------|------|
| `type` | int | ❌ | 日志类型筛选 | `2`（消费日志） |
| `start_timestamp` | int64 | ❌ | 开始时间戳（秒） | `1640995200` |
| `end_timestamp` | int64 | ❌ | 结束时间戳（秒） | `1641081600` |
| `model_name` | string | ❌ | 模型名称（支持模糊匹配） | `gpt` |
| `group` | string | ❌ | 分组名称 | `default` |

### 查询模式参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `lightweight` | boolean | ❌ | 轻量级查询，值为 `true` 时启用 |
| `use_cursor` | boolean | ❌ | 游标分页，值为 `true` 时启用 |
| `cursor` | string | ❌ | 游标值，用于获取下一页数据 |

### 日志类型说明

| 类型值 | 说明 |
|--------|------|
| `0` | 全部类型 |
| `1` | 充值日志 |
| `2` | 消费日志 |
| `3` | 管理日志 |
| `4` | 系统日志 |
| `5` | 错误日志 |

## 响应格式

### 普通模式响应

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12345,
      "user_id": 1,
      "created_at": 1640995200,
      "type": 2,
      "content": "使用模型 gpt-4",
      "username": "user123",
      "token_name": "我的Token",
      "model_name": "gpt-4",
      "quota": 1000,
      "prompt_tokens": 50,
      "completion_tokens": 100,
      "use_time": 2,
      "is_stream": false,
      "channel_id": 1,
      "channel_name": "OpenAI",
      "token_id": 456,
      "group": "default",
      "ip": "192.168.1.1",
      "other": "{\"request_id\":\"req_123\"}"
    }
  ],
  "total": 15420,
  "pages": 771,
  "page": 1,
  "page_size": 20
}
```

#### 分页信息字段说明

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `total` | int | 符合条件的日志总数 |
| `pages` | int | 总页数（根据 page_size 计算） |
| `page` | int | 当前页码 |
| `page_size` | int | 每页显示的记录数 |

### 游标分页模式响应

```json
{
  "success": true,
  "message": "",
  "data": [
    // ... 日志数据数组
  ],
  "next_cursor": "1640995200_12345",
  "has_more": true
}
```

**注意**: 游标分页模式为了性能考虑，不提供 `total` 和 `pages` 字段，因为计算总数会显著影响大数据量查询的性能。

### 轻量级查询响应

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12345,
      "created_at": 1640995200,
      "type": 2,
      "content": "使用模型 gpt-4",
      "model_name": "gpt-4",
      "quota": 1000,
      "prompt_tokens": 50,
      "completion_tokens": 100,
      "use_time": 2,
      "is_stream": false
    }
  ],
  "total": 15420,
  "pages": 771,
  "page": 1,
  "page_size": 20
}
```

## 使用示例

### 1. 基础查询

```bash
# 获取最新20条日志
curl "http://your-domain.com/api/log/token?key=sk-xxx"

# 获取第2页，每页50条
curl "http://your-domain.com/api/log/token?key=sk-xxx&page=2&page_size=50"
```

### 2. 带筛选条件的查询

```bash
# 查询最近7天的消费日志
curl "http://your-domain.com/api/log/token?key=sk-xxx&type=2&start_timestamp=1640995200&end_timestamp=1641081600"

# 查询包含"gpt"的模型日志
curl "http://your-domain.com/api/log/token?key=sk-xxx&model_name=gpt&page_size=100"
```

### 3. 轻量级查询（推荐用于中等数据量）

```bash
# 轻量级查询，只返回核心字段
curl "http://your-domain.com/api/log/token?key=sk-xxx&lightweight=true&page_size=1000"

# 轻量级查询 + 筛选条件
curl "http://your-domain.com/api/log/token?key=sk-xxx&lightweight=true&type=2&page_size=2000"
```

### 4. 游标分页（推荐用于大数据量）

```bash
# 第一次查询，获取最新数据
curl "http://your-domain.com/api/log/token?key=sk-xxx&use_cursor=true&page_size=1000"

# 使用返回的cursor获取下一页
curl "http://your-domain.com/api/log/token?key=sk-xxx&use_cursor=true&page_size=1000&cursor=1640995200_12345"

# 轻量级游标分页（最高性能）
curl "http://your-domain.com/api/log/token?key=sk-xxx&use_cursor=true&lightweight=true&page_size=5000"
```

## 性能优化建议

### 数据量级别对应的推荐查询方式

| 数据量 | 推荐查询方式 | 配置建议 |
|--------|-------------|----------|
| < 1万条 | 普通分页 | `page_size=100` |
| 1-10万条 | 普通分页或轻量级 | `lightweight=true&page_size=500` |
| 10-100万条 | 轻量级查询 | `lightweight=true&page_size=1000` |
| > 100万条 | 游标分页 | `use_cursor=true&lightweight=true&page_size=2000` |

### 查询优化技巧

1. **时间范围筛选**: 尽量指定 `start_timestamp` 和 `end_timestamp` 缩小查询范围
2. **类型筛选**: 使用 `type` 参数只查询需要的日志类型
3. **合理的页面大小**: 
   - 页面展示：`page_size=20-50`
   - 数据导出：`page_size=1000-5000`
4. **游标分页遍历**: 对于大数据量导出，使用游标分页循环获取

## 游标分页详细说明

### 游标格式
游标由时间戳和ID组成：`{timestamp}_{id}`
- 示例：`1640995200_12345`

### 遍历所有数据的示例代码

```javascript
async function getAllLogs(apiKey) {
  const allLogs = [];
  let cursor = null;
  let hasMore = true;
  
  while (hasMore) {
    const url = cursor 
      ? `http://your-domain.com/api/log/token?key=${apiKey}&use_cursor=true&lightweight=true&page_size=2000&cursor=${cursor}`
      : `http://your-domain.com/api/log/token?key=${apiKey}&use_cursor=true&lightweight=true&page_size=2000`;
    
    const response = await fetch(url);
    const result = await response.json();
    
    if (result.success) {
      allLogs.push(...result.data);
      cursor = result.next_cursor;
      hasMore = result.has_more;
      
      console.log(`已获取 ${allLogs.length} 条日志`);
    } else {
      console.error('查询失败:', result.message);
      break;
    }
  }
  
  return allLogs;
}
```

## 错误码说明

| HTTP状态码 | success | message | 说明 |
|-----------|---------|---------|------|
| 200 | false | "key parameter is required" | 缺少key参数 |
| 200 | false | "start_timestamp cannot be greater than end_timestamp" | 时间戳参数错误 |
| 200 | false | "record not found" | Token不存在 |
| 200 | false | "database error: ..." | 数据库查询错误 |

## 注意事项

1. **无鉴权**: 此接口不需要额外的身份验证，但需要有效的Token key
2. **数据安全**: 返回的日志会自动过滤敏感信息（如管理员信息）
3. **ID脱敏**: 日志ID会进行脱敏处理（`id % 1024`）
4. **时间戳单位**: 所有时间戳参数和返回值均为秒级Unix时间戳
5. **模型名称匹配**: `model_name` 参数支持模糊匹配（LIKE查询）
6. **页面大小限制**: 虽然移除了100条的硬限制，但建议单次查询不超过5000条以保证响应速度

## 更新日志

- **v1.0**: 基础分页查询功能
- **v1.1**: 添加轻量级查询模式，优化大数据量查询性能
- **v1.2**: 添加游标分页模式，支持100万+数据的高性能查询
- **v1.3**: 移除页面大小限制，支持自定义大页面查询
- **v1.4**: 添加分页信息返回，包括总数统计（total）、总页数（pages）、当前页码（page）和每页大小（page_size）
