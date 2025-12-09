# Prometheus 监控需求文档

## 1. 项目背景

New API 作为 AI 模型网关和资产管理系统，需要实时监控各个渠道和模型的调用情况，以便及时发现问题、优化性能和保障服务质量。本文档详细描述了 Prometheus 监控系统的实施需求。

## 2. 监控目标

### 2.1 核心监控指标

**模型调用成功率监控**
- 按渠道维度统计调用成功率
- 按模型维度统计调用成功率（在渠道内）
- 实时展示错误信息和状态码
- 支持多维度数据筛选和钻取

## 3. 详细需求说明

### 3.1 监控指标设计

#### 3.1.1 Prometheus Metrics 定义

```
# 渠道和模型调用总数（按状态分类）
new_api_model_requests_total{
  channel_id="1",
  channel_name="OpenAI-Main",
  channel_type="openai",
  model_name="gpt-4",
  status="success|failed",
  error_code="",
  site_id="default"
} counter

# 渠道和模型调用响应时间
new_api_model_request_duration_seconds{
  channel_id="1",
  channel_name="OpenAI-Main",
  channel_type="openai",
  model_name="gpt-4",
  site_id="default"
} histogram

# 渠道和模型错误详情（仅失败请求）
new_api_model_request_errors_total{
  channel_id="1",
  channel_name="OpenAI-Main",
  channel_type="openai",
  model_name="gpt-4",
  error_code="429|500|503",
  error_message="Rate limit exceeded|Internal server error|Service unavailable",
  site_id="default"
} counter

# 渠道在线状态
new_api_channel_status{
  channel_id="1",
  channel_name="OpenAI-Main",
  channel_type="openai",
  status="online|offline|testing",
  site_id="default"
} gauge

# 当前活跃请求数
new_api_active_requests{
  channel_id="1",
  channel_name="OpenAI-Main",
  channel_type="openai",
  model_name="gpt-4",
  site_id="default"
} gauge
```

#### 3.1.2 Label 说明

| Label | 说明 | 示例值 |
|-------|------|--------|
| channel_id | 渠道ID | "1", "2", "3" |
| channel_name | 渠道名称 | "OpenAI-Main", "Claude-Backup" |
| channel_type | 渠道类型 | "openai", "claude", "gemini" |
| model_name | 模型名称 | "gpt-4", "claude-3-opus", "gemini-pro" |
| status | 请求状态 | "success", "failed" |
| error_code | HTTP错误码 | "200", "429", "500", "503" |
| error_message | 错误信息摘要 | "Rate limit exceeded" |
| site_id | 站点ID（多站点部署） | "default", "site1" |

### 3.2 数据采集点

#### 3.2.1 采集位置

在 `relay/` 目录的请求处理流程中埋点：

1. **请求开始时**：记录活跃请求数 +1
2. **请求结束时**：
   - 记录活跃请求数 -1
   - 记录请求总数（按状态）
   - 记录响应时间
   - 如果失败，记录错误详情

3. **渠道状态变更时**：更新渠道在线状态

#### 3.2.2 实现位置建议

- **主要埋点文件**：
  - `middleware/metrics.go`（新建）：Prometheus metrics 定义和初始化
  - `relay/adaptor/adaptor.go`：在 `DoRequest` 或 `DoResponse` 方法中埋点
  - `controller/relay.go`：在请求处理入口埋点
  - `model/channel.go`：在渠道状态更新时埋点

### 3.3 Grafana 可视化需求

#### 3.3.1 面板布局设计

**Dashboard 结构**（单页面，分为 4 个区域）

```
┌─────────────────────────────────────────────────────────────────┐
│ 📊 New API - 模型调用监控面板                                      │
│ Time Range: [Last 24h ▼]  Refresh: [30s ▼]                      │
├─────────────────────────────────────────────────────────────────┤
│ 🔍 筛选器区域                                                      │
│ ┌──────────────────┬──────────────────┬─────────────────────┐   │
│ │ 渠道: [All ▼]    │ 模型: [All ▼]    │ 站点: [default ▼] │   │
│ └──────────────────┴──────────────────┴─────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│ 📈 概览指标区域（4个大数字卡片）                                    │
│ ┌──────────┬──────────┬──────────┬──────────┐                   │
│ │ 总成功率  │ 总请求数  │ 活跃请求  │ 平均延迟 │                   │
│ │ 99.8% ↑  │ 1.2M    │ 245      │ 523ms   │                   │
│ └──────────┴──────────┴──────────┴──────────┘                   │
├─────────────────────────────────────────────────────────────────┤
│ 📊 渠道维度分析区域                                                │
│ ┌────────────────────────────────┬──────────────────────────┐   │
│ │ 🎯 渠道成功率排名（横向柱状图）   │ 📉 渠道成功率趋势（时序） │   │
│ │ OpenAI-Main    ████████ 99.9% │ [多线图，每条线代表一个渠道] │   │
│ │ Claude-1       ███████  98.5% │                          │   │
│ │ Gemini-Backup  ██████   96.2% │                          │   │
│ └────────────────────────────────┴──────────────────────────┘   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ 📊 渠道请求量分布（饼图/环形图）                              │   │
│ │     OpenAI-Main: 45%                                      │   │
│ │     Claude-1: 30%                                         │   │
│ │     Gemini-Backup: 25%                                    │   │
│ └───────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│ 🔬 模型维度分析区域（选择渠道后显示）                               │
│ ┌────────────────────────────────┬──────────────────────────┐   │
│ │ 🎯 模型成功率排名（横向柱状图）   │ 📉 模型成功率趋势（时序） │   │
│ │ gpt-4-turbo    ████████ 99.9% │ [多线图，每条线代表一个模型] │   │
│ │ gpt-4          ███████  99.1% │                          │   │
│ │ gpt-3.5-turbo  ██████   97.8% │                          │   │
│ └────────────────────────────────┴──────────────────────────┘   │
│ ┌──────────────────────────────┬────────────────────────────┐   │
│ │ ⚡ 模型平均响应时间（柱状图）   │ 📊 模型请求量分布（饼图）   │   │
│ │ gpt-4: 523ms                │ gpt-4-turbo: 50%          │   │
│ │ gpt-3.5: 234ms              │ gpt-4: 30%                │   │
│ └──────────────────────────────┴────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│ ❌ 错误详情区域                                                   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ 🔴 Top 10 错误类型（表格，实时更新）                          │   │
│ │ ┌────┬─────────┬──────┬──────────┬─────────┬─────────┐   │   │
│ │ │排名│ 错误码   │ 次数 │ 渠道      │ 模型    │ 错误信息 │   │   │
│ │ ├────┼─────────┼──────┼──────────┼─────────┼─────────┤   │   │
│ │ │ 1  │ 429     │ 1.2K │ OpenAI-1 │ gpt-4   │ Rate... │   │   │
│ │ │ 2  │ 500     │ 856  │ Claude-1 │ claude..│ Inter...│   │   │
│ │ └────┴─────────┴──────┴──────────┴─────────┴─────────┘   │   │
│ └───────────────────────────────────────────────────────────┘   │
│ ┌──────────────────────────────┬────────────────────────────┐   │
│ │ 📊 错误码分布（饼图）          │ 📈 错误趋势（时序堆叠图）    │   │
│ │ 429: 45%                    │ [堆叠区域图，不同错误码用   │   │
│ │ 500: 30%                    │  不同颜色表示]             │   │
│ │ 503: 25%                    │                            │   │
│ └──────────────────────────────┴────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

#### 3.3.2 面板详细配置

**1. 总成功率卡片（Stat Panel）**
```json
{
  "title": "总成功率",
  "type": "stat",
  "targets": [{
    "expr": "sum(rate(new_api_model_requests_total{status=\"success\"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100"
  }],
  "options": {
    "graphMode": "area",
    "colorMode": "background",
    "orientation": "horizontal",
    "textMode": "value_and_name",
    "reduceOptions": {
      "values": false,
      "calcs": ["lastNotNull"]
    }
  },
  "fieldConfig": {
    "defaults": {
      "unit": "percent",
      "decimals": 2,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "value": 0, "color": "red" },
          { "value": 95, "color": "orange" },
          { "value": 98, "color": "yellow" },
          { "value": 99, "color": "green" }
        ]
      }
    }
  }
}
```

**2. 渠道成功率排名（Bar Gauge）**
```json
{
  "title": "渠道成功率排名（最近5分钟）",
  "type": "bargauge",
  "targets": [{
    "expr": "sum by (channel_name) (rate(new_api_model_requests_total{status=\"success\"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 100",
    "legendFormat": "{{channel_name}}"
  }],
  "options": {
    "orientation": "horizontal",
    "displayMode": "gradient",
    "showUnfilled": true
  },
  "fieldConfig": {
    "defaults": {
      "unit": "percent",
      "min": 0,
      "max": 100,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "value": 0, "color": "red" },
          { "value": 95, "color": "orange" },
          { "value": 98, "color": "yellow" },
          { "value": 99.5, "color": "green" }
        ]
      }
    }
  }
}
```

**3. 渠道成功率趋势（Time Series）**
```json
{
  "title": "渠道成功率趋势",
  "type": "timeseries",
  "targets": [{
    "expr": "sum by (channel_name) (rate(new_api_model_requests_total{status=\"success\"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 100",
    "legendFormat": "{{channel_name}}"
  }],
  "options": {
    "tooltip": {
      "mode": "multi",
      "sort": "desc"
    },
    "legend": {
      "displayMode": "table",
      "placement": "right",
      "calcs": ["lastNotNull", "min", "max", "mean"]
    }
  },
  "fieldConfig": {
    "defaults": {
      "unit": "percent",
      "min": 0,
      "max": 100,
      "custom": {
        "drawStyle": "line",
        "lineInterpolation": "smooth",
        "lineWidth": 2,
        "fillOpacity": 10,
        "showPoints": "never"
      }
    }
  }
}
```

**4. 模型成功率排名（支持渠道筛选）**
```json
{
  "title": "模型成功率排名（最近5分钟）",
  "type": "bargauge",
  "targets": [{
    "expr": "sum by (model_name) (rate(new_api_model_requests_total{status=\"success\", channel_name=~\"$channel\"}[5m])) / sum by (model_name) (rate(new_api_model_requests_total{channel_name=~\"$channel\"}[5m])) * 100",
    "legendFormat": "{{model_name}}"
  }],
  "options": {
    "orientation": "horizontal",
    "displayMode": "gradient",
    "showUnfilled": true
  },
  "fieldConfig": {
    "defaults": {
      "unit": "percent",
      "min": 0,
      "max": 100,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "value": 0, "color": "red" },
          { "value": 95, "color": "orange" },
          { "value": 98, "color": "yellow" },
          { "value": 99.5, "color": "green" }
        ]
      }
    }
  }
}
```

**5. Top 10 错误详情表格（Table Panel）**
```json
{
  "title": "Top 10 错误详情（最近1小时）",
  "type": "table",
  "targets": [{
    "expr": "topk(10, sum by (error_code, channel_name, model_name, error_message) (increase(new_api_model_request_errors_total[1h])))",
    "format": "table",
    "instant": true
  }],
  "transformations": [
    {
      "id": "organize",
      "options": {
        "excludeByName": {
          "Time": true
        },
        "indexByName": {
          "error_code": 0,
          "Value": 1,
          "channel_name": 2,
          "model_name": 3,
          "error_message": 4
        },
        "renameByName": {
          "error_code": "错误码",
          "Value": "次数",
          "channel_name": "渠道",
          "model_name": "模型",
          "error_message": "错误信息"
        }
      }
    }
  ],
  "fieldConfig": {
    "defaults": {
      "custom": {
        "align": "left",
        "filterable": true
      }
    },
    "overrides": [
      {
        "matcher": { "id": "byName", "options": "次数" },
        "properties": [
          {
            "id": "custom.displayMode",
            "value": "color-background"
          },
          {
            "id": "thresholds",
            "value": {
              "mode": "absolute",
              "steps": [
                { "value": 0, "color": "green" },
                { "value": 100, "color": "yellow" },
                { "value": 500, "color": "orange" },
                { "value": 1000, "color": "red" }
              ]
            }
          }
        ]
      },
      {
        "matcher": { "id": "byName", "options": "错误码" },
        "properties": [
          {
            "id": "custom.displayMode",
            "value": "color-text"
          },
          {
            "id": "mappings",
            "value": [
              { "type": "value", "options": { "429": { "text": "429 限流", "color": "orange" } } },
              { "type": "value", "options": { "500": { "text": "500 服务器错误", "color": "red" } } },
              { "type": "value", "options": { "503": { "text": "503 服务不可用", "color": "red" } } }
            ]
          }
        ]
      }
    ]
  }
}
```

**6. 错误码分布（Pie Chart）**
```json
{
  "title": "错误码分布（最近1小时）",
  "type": "piechart",
  "targets": [{
    "expr": "sum by (error_code) (increase(new_api_model_request_errors_total[1h]))",
    "legendFormat": "{{error_code}}"
  }],
  "options": {
    "pieType": "donut",
    "displayLabels": ["name", "percent"],
    "legend": {
      "displayMode": "table",
      "placement": "right",
      "values": ["value", "percent"]
    }
  },
  "fieldConfig": {
    "defaults": {
      "unit": "short",
      "mappings": [
        { "type": "value", "options": { "429": { "text": "429 限流" } } },
        { "type": "value", "options": { "500": { "text": "500 服务器错误" } } },
        { "type": "value", "options": { "503": { "text": "503 服务不可用" } } }
      ]
    }
  }
}
```

**7. 错误趋势堆叠图（Time Series）**
```json
{
  "title": "错误趋势（按错误码堆叠）",
  "type": "timeseries",
  "targets": [{
    "expr": "sum by (error_code) (rate(new_api_model_request_errors_total[5m]))",
    "legendFormat": "{{error_code}}"
  }],
  "options": {
    "tooltip": {
      "mode": "multi",
      "sort": "desc"
    },
    "legend": {
      "displayMode": "table",
      "placement": "right",
      "calcs": ["lastNotNull", "sum"]
    }
  },
  "fieldConfig": {
    "defaults": {
      "unit": "reqps",
      "custom": {
        "drawStyle": "line",
        "lineInterpolation": "smooth",
        "lineWidth": 1,
        "fillOpacity": 70,
        "stacking": {
          "mode": "normal",
          "group": "A"
        },
        "showPoints": "never"
      }
    }
  }
}
```

#### 3.3.3 变量（Variables）配置

**1. 渠道选择器**
```json
{
  "name": "channel",
  "type": "query",
  "label": "渠道",
  "datasource": "Prometheus",
  "query": "label_values(new_api_model_requests_total, channel_name)",
  "multi": true,
  "includeAll": true,
  "allValue": ".*",
  "refresh": 1
}
```

**2. 模型选择器**
```json
{
  "name": "model",
  "type": "query",
  "label": "模型",
  "datasource": "Prometheus",
  "query": "label_values(new_api_model_requests_total{channel_name=~\"$channel\"}, model_name)",
  "multi": true,
  "includeAll": true,
  "allValue": ".*",
  "refresh": 1
}
```

**3. 站点选择器**
```json
{
  "name": "site_id",
  "type": "query",
  "label": "站点",
  "datasource": "Prometheus",
  "query": "label_values(new_api_model_requests_total, site_id)",
  "multi": false,
  "includeAll": false,
  "refresh": 1
}
```

#### 3.3.4 主题和样式

**主题配置**
- 使用 Grafana 深色主题（Dark）
- 主色调：蓝色系（#3b82f6）
- 成功色：绿色（#10b981）
- 警告色：橙色（#f59e0b）
- 错误色：红色（#ef4444）

**面板通用配置**
- 透明背景
- 圆角边框
- 阴影效果
- 自适应布局

### 3.4 告警规则配置

#### 3.4.1 渠道成功率告警

```yaml
# alerts/channel_success_rate.yml
groups:
  - name: channel_alerts
    interval: 30s
    rules:
      # 渠道成功率低于95%
      - alert: ChannelLowSuccessRate
        expr: |
          sum by (channel_name, channel_id) (rate(new_api_model_requests_total{status="success"}[5m]))
          /
          sum by (channel_name, channel_id) (rate(new_api_model_requests_total[5m]))
          * 100 < 95
        for: 5m
        labels:
          severity: warning
          component: channel
        annotations:
          summary: "渠道 {{ $labels.channel_name }} 成功率低于95%"
          description: "渠道 {{ $labels.channel_name }} (ID: {{ $labels.channel_id }}) 在过去5分钟的成功率为 {{ $value | humanize }}%，低于95%阈值"

      # 渠道成功率低于90%（严重告警）
      - alert: ChannelCriticalSuccessRate
        expr: |
          sum by (channel_name, channel_id) (rate(new_api_model_requests_total{status="success"}[5m]))
          /
          sum by (channel_name, channel_id) (rate(new_api_model_requests_total[5m]))
          * 100 < 90
        for: 2m
        labels:
          severity: critical
          component: channel
        annotations:
          summary: "渠道 {{ $labels.channel_name }} 成功率低于90%（严重）"
          description: "渠道 {{ $labels.channel_name }} (ID: {{ $labels.channel_id }}) 在过去5分钟的成功率为 {{ $value | humanize }}%，低于90%阈值，请立即检查"
```

#### 3.4.2 模型成功率告警

```yaml
# alerts/model_success_rate.yml
groups:
  - name: model_alerts
    interval: 30s
    rules:
      # 模型成功率低于90%
      - alert: ModelLowSuccessRate
        expr: |
          sum by (model_name, channel_name) (rate(new_api_model_requests_total{status="success"}[5m]))
          /
          sum by (model_name, channel_name) (rate(new_api_model_requests_total[5m]))
          * 100 < 90
        for: 5m
        labels:
          severity: warning
          component: model
        annotations:
          summary: "模型 {{ $labels.model_name }} 在渠道 {{ $labels.channel_name }} 成功率低于90%"
          description: "模型 {{ $labels.model_name }} 在渠道 {{ $labels.channel_name }} 的成功率为 {{ $value | humanize }}%"
```

#### 3.4.3 错误率激增告警

```yaml
# alerts/error_spike.yml
groups:
  - name: error_alerts
    interval: 30s
    rules:
      # 特定错误码激增
      - alert: HighErrorRate
        expr: |
          sum by (error_code, channel_name) (rate(new_api_model_request_errors_total[5m])) > 10
        for: 2m
        labels:
          severity: warning
          component: error
        annotations:
          summary: "错误码 {{ $labels.error_code }} 在渠道 {{ $labels.channel_name }} 出现频率过高"
          description: "错误码 {{ $labels.error_code }} 在渠道 {{ $labels.channel_name }} 的发生率为 {{ $value | humanize }} 次/秒"

      # 429限流告警
      - alert: RateLimitErrors
        expr: |
          sum by (channel_name) (rate(new_api_model_request_errors_total{error_code="429"}[5m])) > 5
        for: 5m
        labels:
          severity: warning
          component: rate_limit
        annotations:
          summary: "渠道 {{ $labels.channel_name }} 出现大量限流错误"
          description: "渠道 {{ $labels.channel_name }} 的429限流错误发生率为 {{ $value | humanize }} 次/秒，可能需要调整请求速率"
```

#### 3.4.4 渠道离线告警

```yaml
# alerts/channel_status.yml
groups:
  - name: channel_status_alerts
    interval: 30s
    rules:
      # 渠道离线
      - alert: ChannelOffline
        expr: new_api_channel_status{status="offline"} == 1
        for: 2m
        labels:
          severity: critical
          component: channel_status
        annotations:
          summary: "渠道 {{ $labels.channel_name }} 已离线"
          description: "渠道 {{ $labels.channel_name }} (ID: {{ $labels.channel_id }}) 状态为离线，请检查渠道配置和网络连接"

      # 渠道无请求（可能异常）
      - alert: ChannelNoRequests
        expr: |
          sum by (channel_name, channel_id) (rate(new_api_model_requests_total[5m])) == 0
        for: 10m
        labels:
          severity: warning
          component: channel_status
        annotations:
          summary: "渠道 {{ $labels.channel_name }} 在过去10分钟无请求"
          description: "渠道 {{ $labels.channel_name }} (ID: {{ $labels.channel_id }}) 在过去10分钟没有收到任何请求，可能存在路由或负载均衡问题"
```

### 3.5 部署架构

#### 3.5.1 组件架构

```
┌─────────────────┐
│   New API       │
│   Application   │
│                 │
│  ┌───────────┐  │
│  │ Metrics   │  │──────┐
│  │ Endpoint  │  │      │
│  │ :9090     │  │      │
│  └───────────┘  │      │
└─────────────────┘      │
                         │ Scrape (15s interval)
                         ▼
              ┌─────────────────┐
              │  Prometheus     │
              │  Server         │
              │                 │
              │  - Data Storage │
              │  - Alert Rules  │
              │  - Query Engine │
              └─────────────────┘
                         │
                         │ Query
                         ▼
              ┌─────────────────┐
              │   Grafana       │
              │   Dashboard     │
              │                 │
              │  - Dashboards   │
              │  - Alerts       │
              │  - Users        │
              └─────────────────┘
                         │
                         │ Notify
                         ▼
              ┌─────────────────┐
              │  AlertManager   │
              │                 │
              │  - Email        │
              │  - Webhook      │
              │  - Slack/飞书    │
              └─────────────────┘
```

#### 3.5.2 Docker Compose 部署配置

```yaml
# docker-compose.prometheus.yml
version: '3.8'

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: new-api-prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
      - ./prometheus/alerts:/etc/prometheus/alerts
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=30d'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
      - '--web.enable-lifecycle'
    networks:
      - new-api-monitor

  grafana:
    image: grafana/grafana:latest
    container_name: new-api-grafana
    restart: unless-stopped
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin123
      - GF_USERS_ALLOW_SIGN_UP=false
      - GF_SERVER_ROOT_URL=http://localhost:3001
      - GF_INSTALL_PLUGINS=grafana-piechart-panel
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning
      - ./grafana/dashboards:/var/lib/grafana/dashboards
    depends_on:
      - prometheus
    networks:
      - new-api-monitor

  alertmanager:
    image: prom/alertmanager:latest
    container_name: new-api-alertmanager
    restart: unless-stopped
    ports:
      - "9093:9093"
    volumes:
      - ./alertmanager/alertmanager.yml:/etc/alertmanager/alertmanager.yml
      - alertmanager_data:/alertmanager
    command:
      - '--config.file=/etc/alertmanager/alertmanager.yml'
      - '--storage.path=/alertmanager'
    networks:
      - new-api-monitor

  # New API 应用（需要暴露 metrics endpoint）
  new-api:
    build: .
    container_name: new-api-app
    restart: unless-stopped
    ports:
      - "3000:3000"
      - "9091:9091"  # Prometheus metrics endpoint
    environment:
      - PROMETHEUS_ENABLED=true
      - PROMETHEUS_PORT=9091
    volumes:
      - ./data:/data
    networks:
      - new-api-monitor

volumes:
  prometheus_data:
  grafana_data:
  alertmanager_data:

networks:
  new-api-monitor:
    driver: bridge
```

#### 3.5.3 Prometheus 配置

```yaml
# prometheus/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'new-api-prod'
    environment: 'production'

# AlertManager 配置
alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - alertmanager:9093

# 告警规则文件
rule_files:
  - "/etc/prometheus/alerts/*.yml"

# 抓取配置
scrape_configs:
  # New API 应用指标
  - job_name: 'new-api'
    static_configs:
      - targets: ['new-api:9091']
        labels:
          app: 'new-api'
          env: 'production'
    scrape_interval: 15s
    scrape_timeout: 10s

  # Prometheus 自身指标
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  # Grafana 指标
  - job_name: 'grafana'
    static_configs:
      - targets: ['grafana:3000']
```

#### 3.5.4 AlertManager 配置

```yaml
# alertmanager/alertmanager.yml
global:
  resolve_timeout: 5m
  smtp_smarthost: 'smtp.example.com:587'
  smtp_from: 'alerts@example.com'
  smtp_auth_username: 'alerts@example.com'
  smtp_auth_password: 'your-password'

# 告警路由
route:
  group_by: ['alertname', 'cluster', 'severity']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'
  routes:
    # 严重告警立即发送
    - match:
        severity: critical
      receiver: 'critical-alerts'
      group_wait: 0s
      repeat_interval: 5m

    # 警告告警
    - match:
        severity: warning
      receiver: 'warning-alerts'
      repeat_interval: 30m

# 接收器配置
receivers:
  - name: 'default'
    webhook_configs:
      - url: 'http://your-webhook-endpoint/alerts'

  - name: 'critical-alerts'
    email_configs:
      - to: 'ops@example.com'
        headers:
          Subject: '[CRITICAL] New API Alert: {{ .GroupLabels.alertname }}'
    webhook_configs:
      - url: 'http://your-webhook-endpoint/critical'
        send_resolved: true
    # 企业微信/飞书通知（示例）
    wechat_configs:
      - corp_id: 'your-corp-id'
        to_user: '@all'
        agent_id: 'your-agent-id'
        api_secret: 'your-api-secret'

  - name: 'warning-alerts'
    email_configs:
      - to: 'team@example.com'
        headers:
          Subject: '[WARNING] New API Alert: {{ .GroupLabels.alertname }}'

# 告警抑制规则
inhibit_rules:
  # 如果严重告警触发，抑制同一渠道的警告告警
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['channel_name', 'channel_id']
```

#### 3.5.5 Grafana 自动配置

**数据源配置**
```yaml
# grafana/provisioning/datasources/prometheus.yml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: true
    jsonData:
      timeInterval: "15s"
      queryTimeout: "60s"
```

**Dashboard 自动加载配置**
```yaml
# grafana/provisioning/dashboards/dashboards.yml
apiVersion: 1

providers:
  - name: 'New API Dashboards'
    orgId: 1
    folder: 'New API'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 30
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
      foldersFromFilesStructure: true
```

## 4. 实施步骤

### 4.1 第一阶段：后端 Metrics 实现（预计3天）

**Day 1: 基础框架搭建**
1. 创建 `middleware/metrics.go`，定义所有 Prometheus metrics
2. 初始化 Prometheus registry 和 HTTP handler
3. 在 `main.go` 中注册 metrics endpoint（`:9091/metrics`）
4. 编写单元测试验证 metrics 定义

**Day 2: 请求埋点实现**
1. 在 `relay/adaptor/` 中添加请求开始/结束埋点
2. 在 `controller/relay.go` 中添加入口埋点
3. 捕获错误信息和状态码
4. 实现响应时间 histogram 记录

**Day 3: 渠道状态监控**
1. 在 `model/channel.go` 中添加状态变更埋点
2. 实现渠道健康检查 metrics
3. 添加活跃请求数 gauge 指标
4. 集成测试和性能测试

### 4.2 第二阶段：Prometheus 部署（预计1天）

**Day 4: 基础设施部署**
1. 编写 Docker Compose 配置文件
2. 配置 Prometheus 抓取规则
3. 配置 AlertManager 和告警规则
4. 部署测试环境验证数据采集

### 4.3 第三阶段：Grafana Dashboard 开发（预计3-4天）

**Day 5: 基础面板开发**
1. 创建 Dashboard 基本结构
2. 实现变量（渠道、模型、站点选择器）
3. 开发概览指标卡片（总成功率、总请求数等）
4. 测试数据查询性能

**Day 6: 渠道维度面板**
1. 开发渠道成功率排名面板
2. 开发渠道成功率趋势面板
3. 开发渠道请求量分布面板
4. 优化样式和交互

**Day 7: 模型维度面板**
1. 开发模型成功率排名面板（支持渠道筛选）
2. 开发模型成功率趋势面板
3. 开发模型响应时间面板
4. 开发模型请求量分布面板

**Day 8: 错误详情面板**
1. 开发 Top 10 错误详情表格
2. 开发错误码分布饼图
3. 开发错误趋势堆叠图
4. 实现错误信息格式化和展示

**Day 9: 样式优化和导出**
1. 统一主题和配色方案
2. 优化面板布局和响应式设计
3. 添加面板说明和文档
4. 导出 Dashboard JSON（支持数据源变量）
5. 编写 Dashboard 导入文档

### 4.4 第四阶段：测试和优化（预计2天）

**Day 10: 集成测试**
1. 压力测试验证 metrics 性能影响
2. 验证告警规则触发和通知
3. 测试 Dashboard 在不同数据量下的表现
4. 修复发现的 bug

**Day 11: 文档和交付**
1. 编写完整部署文档
2. 编写使用手册和最佳实践
3. 培训团队成员
4. 正式上线

## 5. Dashboard JSON 导出说明

### 5.1 数据源变量配置

为了让 Dashboard 可以导入到不同环境并选择数据源，需要在 Dashboard JSON 中使用数据源变量：

```json
{
  "__inputs": [
    {
      "name": "DS_PROMETHEUS",
      "label": "Prometheus",
      "description": "Prometheus 数据源",
      "type": "datasource",
      "pluginId": "prometheus",
      "pluginName": "Prometheus"
    }
  ],
  "__requires": [
    {
      "type": "grafana",
      "id": "grafana",
      "name": "Grafana",
      "version": "10.0.0"
    },
    {
      "type": "datasource",
      "id": "prometheus",
      "name": "Prometheus",
      "version": "1.0.0"
    },
    {
      "type": "panel",
      "id": "timeseries",
      "name": "Time series",
      "version": ""
    }
  ],
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "liveNow": false,
  "panels": [
    // 面板配置...
  ],
  "refresh": "30s",
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["new-api", "monitoring", "model-gateway"],
  "templating": {
    "list": [
      // 变量配置...
    ]
  },
  "time": {
    "from": "now-24h",
    "to": "now"
  },
  "timepicker": {},
  "timezone": "browser",
  "title": "New API - 模型调用监控",
  "uid": "new-api-model-monitoring",
  "version": 1,
  "weekStart": ""
}
```

### 5.2 导入步骤

1. 在 Grafana 中选择 "+" → "Import"
2. 粘贴 Dashboard JSON 或上传 JSON 文件
3. 在导入界面选择 Prometheus 数据源
4. 点击 "Import" 完成导入

### 5.3 导出步骤

1. 打开 Dashboard
2. 点击右上角 "Share" → "Export"
3. 勾选 "Export for sharing externally"（导出时移除数据源绑定）
4. 点击 "Save to file" 下载 JSON

## 6. 性能影响评估

### 6.1 Metrics 采集性能影响

**预期性能开销：**
- CPU 增加：< 2%
- 内存增加：~10-20MB（取决于 label 基数）
- 请求延迟增加：< 1ms

**优化措施：**
- 使用高效的 label 设计（避免高基数 label）
- 限制 error_message 长度（最多 200 字符）
- 使用 batch update 减少锁竞争
- 合理设置 histogram buckets

### 6.2 Prometheus 存储估算

**存储公式：**
```
存储大小 = metrics数量 × labels基数 × 采样频率 × 保留时间 × 每个样本大小
```

**示例计算：**
- Metrics 数量：5 个
- Labels 基数：假设 10 个渠道 × 20 个模型 = 200
- 采样频率：15 秒（4 次/分钟）
- 保留时间：30 天
- 每个样本大小：~16 bytes

```
存储大小 ≈ 5 × 200 × 4 × 60 × 24 × 30 × 16 bytes
         ≈ 5 × 200 × 4 × 43200 × 16 bytes
         ≈ 2.76 GB
```

**实际推荐配置：**
- 磁盘空间：预留 10GB（含索引和 WAL）
- 内存：4GB（用于查询缓存）
- 保留时间：30 天

## 7. 安全和权限配置

### 7.1 Prometheus 安全配置

```yaml
# prometheus/web-config.yml
basic_auth_users:
  admin: $2y$10$... # bcrypt hash of password

tls_server_config:
  cert_file: /etc/prometheus/tls/cert.pem
  key_file: /etc/prometheus/tls/key.pem
```

### 7.2 Grafana 用户权限

**角色设计：**
- **Admin**：完整权限，可编辑 Dashboard
- **Editor**：可编辑 Dashboard，不能修改数据源
- **Viewer**：只读权限，只能查看 Dashboard

**团队配置：**
- **运维团队**：Admin 权限
- **开发团队**：Editor 权限
- **业务团队**：Viewer 权限

## 8. 交付物清单

### 8.1 代码文件

- [ ] `middleware/metrics.go` - Prometheus metrics 定义和初始化
- [ ] `middleware/metrics_test.go` - metrics 单元测试
- [ ] 修改 `relay/adaptor/*.go` - 添加请求埋点
- [ ] 修改 `controller/relay.go` - 添加入口埋点
- [ ] 修改 `model/channel.go` - 添加状态埋点
- [ ] 修改 `main.go` - 注册 metrics endpoint

### 8.2 配置文件

- [ ] `docker-compose.prometheus.yml` - Docker Compose 部署配置
- [ ] `prometheus/prometheus.yml` - Prometheus 主配置
- [ ] `prometheus/alerts/channel_success_rate.yml` - 渠道告警规则
- [ ] `prometheus/alerts/model_success_rate.yml` - 模型告警规则
- [ ] `prometheus/alerts/error_spike.yml` - 错误告警规则
- [ ] `prometheus/alerts/channel_status.yml` - 渠道状态告警规则
- [ ] `alertmanager/alertmanager.yml` - AlertManager 配置
- [ ] `grafana/provisioning/datasources/prometheus.yml` - Grafana 数据源配置
- [ ] `grafana/provisioning/dashboards/dashboards.yml` - Dashboard 加载配置

### 8.3 Grafana Dashboard

- [ ] `grafana/dashboards/new-api-model-monitoring.json` - Dashboard JSON 文件
- [ ] Dashboard 包含以下面板：
  - [ ] 总成功率卡片
  - [ ] 总请求数卡片
  - [ ] 活跃请求数卡片
  - [ ] 平均延迟卡片
  - [ ] 渠道成功率排名
  - [ ] 渠道成功率趋势
  - [ ] 渠道请求量分布
  - [ ] 模型成功率排名
  - [ ] 模型成功率趋势
  - [ ] 模型平均响应时间
  - [ ] 模型请求量分布
  - [ ] Top 10 错误详情表格
  - [ ] 错误码分布饼图
  - [ ] 错误趋势堆叠图

### 8.4 文档

- [ ] `docs/PROMETHEUS_MONITORING_REQUIREMENTS.md` - 本需求文档
- [ ] `docs/PROMETHEUS_DEPLOYMENT_GUIDE.md` - 部署指南
- [ ] `docs/PROMETHEUS_USER_MANUAL.md` - 使用手册
- [ ] `docs/GRAFANA_DASHBOARD_GUIDE.md` - Dashboard 使用说明
- [ ] `README.md` 更新 - 添加监控章节

### 8.5 测试

- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试用例
- [ ] 压力测试报告
- [ ] 告警测试验证报告

## 9. 验收标准

### 9.1 功能验收

- [ ] Prometheus 能成功抓取 New API 的 metrics
- [ ] Grafana Dashboard 能正确展示所有指标
- [ ] 变量筛选器（渠道、模型、站点）工作正常
- [ ] 选择渠道后能正确显示该渠道的模型数据
- [ ] 错误信息和状态码能正确展示
- [ ] 告警规则能正确触发和发送通知
- [ ] Dashboard JSON 能成功导入到新环境

### 9.2 性能验收

- [ ] Metrics 采集对应用性能影响 < 2% CPU
- [ ] Prometheus 查询响应时间 < 1s（P95）
- [ ] Grafana Dashboard 加载时间 < 3s
- [ ] 内存占用增加 < 50MB

### 9.3 可用性验收

- [ ] 所有组件支持 Docker Compose 一键部署
- [ ] Dashboard 主题炫酷且专业
- [ ] 面板布局清晰易读
- [ ] 告警消息清晰准确
- [ ] 文档完整且易于理解

### 9.4 可维护性验收

- [ ] 代码符合项目规范
- [ ] 配置文件结构清晰
- [ ] 告警规则可配置
- [ ] Dashboard 可轻松修改
- [ ] 有完整的故障排查指南

## 10. 后续扩展计划

### 10.1 第二期功能（可选）

1. **用户维度监控**
   - 用户请求量统计
   - 用户消费金额统计
   - 用户 Token 使用情况

2. **计费监控**
   - 实时收入统计
   - 成本分析
   - 利润率监控

3. **性能深度监控**
   - 数据库查询性能
   - Redis 缓存命中率
   - 队列积压监控

4. **业务指标监控**
   - 新用户注册趋势
   - DAU/MAU 统计
   - 付费转化率

### 10.2 集成计划

1. **日志关联**
   - Prometheus + Loki 集成
   - 错误日志快速查询

2. **分布式追踪**
   - 集成 Jaeger/Tempo
   - 请求链路追踪

3. **自动化运维**
   - 基于告警的自动扩容
   - 异常渠道自动切换

## 11. 联系和支持

如有任何问题或需要技术支持，请联系：

- 技术负责人：[姓名]
- Email：[email@example.com]
- 项目 Issue：https://github.com/your-org/new-api/issues

---

**文档版本**：v1.0
**创建日期**：2025-12-03
**最后更新**：2025-12-03
**状态**：待评审