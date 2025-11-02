# Prometheus 监控集成说明

本文档描述了自定义指标、Prometheus 配置示例、Grafana 看板示例以及 `/metrics` 输出片段，方便运维快速集成。

## 自定义指标

应用通过 `github.com/prometheus/client_golang` 在运行时暴露以下核心指标：

| 指标 | 类型 | 关键标签 | 说明 |
| --- | --- | --- | --- |
| `app_api_request_total` | Counter | `path`, `method`, `status` | API 请求次数统计 |
| `app_api_request_duration_seconds` | Histogram | `path`, `method` | API 请求延迟分布 |
| `channel_request_total` | Counter | `channel`, `status` | 渠道请求次数（成功/失败） |
| `channel_latency_seconds` | Histogram | `channel` | 渠道请求耗时分布 |
| `channel_error_total` | Counter | `channel`, `status_code`, `error_type` | 渠道错误统计 |
| `channel_error_event_total` | Counter | `channel`, `status_code`, `error_type`, `event_time`, `event_id` | 渠道单次错误事件（含发生时间，默认使用本地时区，可通过 `CHANNEL_ERROR_EVENT_TZ` 指定） |
| `channel_tokens_total` | Counter | `channel`, `token_type` | 渠道 Token 消耗（Prompt/Completion/Total） |
| `channel_rpm` | Gauge | `channel` | 1 分钟窗口请求数（Requests per minute） |
| `channel_tpm` | Gauge | `channel` | 1 分钟窗口 Token 数（Tokens per minute） |

其中 `channel_rpm` 与 `channel_tpm` 通过滑动窗口实时更新，无需额外 PromQL 计算。

## /metrics 输出示例

```text
# HELP app_api_request_total Total number of API requests grouped by path, method and status.
# TYPE app_api_request_total counter
app_api_request_total{method="POST",path="/v1/chat/completions",status="200"} 1523
app_api_request_total{method="POST",path="/v1/chat/completions",status="500"} 8
# HELP app_api_request_duration_seconds Latency distribution for API requests.
# TYPE app_api_request_duration_seconds histogram
app_api_request_duration_seconds_bucket{method="POST",path="/v1/chat/completions",le="0.1"} 845
app_api_request_duration_seconds_sum{method="POST",path="/v1/chat/completions"} 112.7
app_api_request_duration_seconds_count{method="POST",path="/v1/chat/completions"} 1531
# HELP channel_request_total Total number of downstream channel requests grouped by status.
# TYPE channel_request_total counter
channel_request_total{channel="openai",status="success"} 1211
channel_request_total{channel="openai",status="error"} 17
# HELP channel_error_total Total number of downstream channel errors grouped by status code and error type.
# TYPE channel_error_total counter
channel_error_total{channel="azure",error_type="upstream_error",status_code="504"} 12
# HELP channel_error_event_total Individual downstream channel error events with occurrence timestamp.
# TYPE channel_error_event_total counter
channel_error_event_total{channel="openai",channel_id="2",detail="无效的令牌",error_type="openai_error",event_id="req-123",event_time="2025-10-29T15:21:03.456789Z",model="gpt-4o-mini",status_code="401"} 1
# HELP channel_tokens_total Total number of tokens consumed per channel grouped by token type.
# TYPE channel_tokens_total counter
channel_tokens_total{channel="openai",token_type="total"} 84533
# HELP channel_rpm Rolling one-minute requests per minute per channel.
# TYPE channel_rpm gauge
channel_rpm{channel="openai"} 74
# HELP channel_tpm Rolling one-minute tokens per minute per channel.
# TYPE channel_tpm gauge
channel_tpm{channel="openai"} 42311
```

## Prometheus 抓取配置示例

```yaml
scrape_configs:
  - job_name: new-api
    metrics_path: /metrics
    scrape_interval: 15s
    static_configs:
      - targets:
          - new-api.example.com:8080
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
        replacement: new-api-prod
```

请根据实际部署环境调整 `targets` 与 `instance` 标签。

## Grafana 面板

目录 `docs/monitoring/grafana_dashboard.json` 提供示例仪表盘，包含以下关键可视化：

 - 渠道成功率（Table）
 - 渠道错误明细表（Table）
 - 渠道 RPM / TPM（Time series）

在 Grafana 中导入该 JSON 文件即可生成看板，确认数据源指向上述 Prometheus 实例。

## Docker Compose 快速部署

仓库内提供了简单的本地监控编排文件：

- `docs/monitoring/docker-compose.yml`：启动 Prometheus 与 Grafana。
- `docs/monitoring/prometheus.yml`：Prometheus 抓取配置（默认目标为 `new-api:8080`）。

使用步骤：

1. 确保 `docker` 与 `docker-compose` 已安装。
2. 编辑 `prometheus.yml`，将 `new-api:8080` 替换为实际服务地址。
3. 在 `docs/monitoring` 目录执行 `docker-compose up -d`。
4. 访问 `http://127.0.0.1:9090` 验证 Prometheus；访问 `http://127.0.0.1:3000` 登录 Grafana（默认账号密码均为 `admin`）。
5. 在 Grafana 添加 Prometheus 数据源，导入 `grafana_dashboard.json` 完成面板配置。
