# Prometheus 监控部署指南

本文档详细说明如何部署和配置 New API 的 Prometheus 监控系统。

## 目录

1. [快速开始](#快速开始)
2. [环境要求](#环境要求)
3. [详细部署步骤](#详细部署步骤)
4. [配置说明](#配置说明)
5. [验证和测试](#验证和测试)
6. [常见问题](#常见问题)
7. [性能调优](#性能调优)

## 快速开始

### 使用 Docker Compose 部署（推荐）

```bash
# 1. 进入项目目录
cd /path/to/new-api

# 2. 确保 New API 已启用 Prometheus
export PROMETHEUS_ENABLED=true

# 3. 启动监控服务
docker-compose -f docker-compose.prometheus.yml up -d

# 4. 验证服务状态
docker-compose -f docker-compose.prometheus.yml ps

# 5. 访问服务
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3001 (admin/admin123)
# AlertManager: http://localhost:9093
```

### 完整部署（New API + 监控）

```bash
# 1. 启动 New API 应用（需要先修改 docker-compose.prometheus.yml 中的 new-api 服务配置）
docker-compose -f docker-compose.prometheus.yml up -d

# 2. 检查 New API metrics endpoint
curl http://localhost:3000/metrics

# 3. 查看 Grafana Dashboard
# 访问 http://localhost:3001，使用 admin/admin123 登录
```

## 环境要求

### 硬件要求

| 组件 | 最低配置 | 推荐配置 |
|------|---------|---------|
| CPU | 2核 | 4核+ |
| 内存 | 4GB | 8GB+ |
| 磁盘 | 20GB | 50GB+ (SSD) |

### 软件要求

- Docker: 20.10+
- Docker Compose: 2.0+
- Go: 1.21+ (如果从源码编译)
- 操作系统: Linux/macOS/Windows

### 网络要求

- New API metrics端口: 3000 (或自定义)
- Prometheus: 9090
- Grafana: 3001
- AlertManager: 9093

## 详细部署步骤

### 步骤 1: 准备配置文件

项目已包含所有必要的配置文件：

```
new-api/
├── docker-compose.prometheus.yml    # Docker Compose 配置
├── prometheus/
│   ├── prometheus.yml               # Prometheus 主配置
│   └── alerts/                      # 告警规则
│       ├── channel_success_rate.yml
│       ├── model_success_rate.yml
│       ├── error_spike.yml
│       └── channel_status.yml
├── alertmanager/
│   └── alertmanager.yml             # AlertManager 配置
└── grafana/
    ├── provisioning/
    │   ├── datasources/
    │   │   └── prometheus.yml       # Grafana 数据源
    │   └── dashboards/
    │       └── dashboards.yml       # Dashboard 自动加载
    └── dashboards/
        └── README.md                # Dashboard 说明
```

### 步骤 2: 配置 New API

#### 方式 1: 环境变量配置

在 `.env` 文件中添加：

```bash
# 启用 Prometheus metrics
PROMETHEUS_ENABLED=true

# New API 端口（默认3000）
PORT=3000

# 站点ID（多站点部署时用于隔离）
SITE_ID=default
```

#### 方式 2: Docker Compose 配置

修改 `docker-compose.prometheus.yml` 中的 new-api 服务：

```yaml
services:
  new-api:
    build: .
    container_name: new-api-app
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - PROMETHEUS_ENABLED=true
      - PORT=3000
      - SITE_ID=default
      # 添加其他必要的环境变量
      - SQL_DSN=your_database_dsn
      - SESSION_SECRET=your_session_secret
    volumes:
      - ./data:/data
    networks:
      - new-api-monitor
```

### 步骤 3: 配置 Prometheus

编辑 `prometheus/prometheus.yml`：

```yaml
scrape_configs:
  - job_name: 'new-api'
    static_configs:
      # 如果 new-api 在 Docker Compose 网络中
      - targets: ['new-api:3000']
        labels:
          app: 'new-api'
          env: 'production'

      # 或者，如果 new-api 在宿主机上运行
      # - targets: ['host.docker.internal:3000']
      #   labels:
      #     app: 'new-api'
      #     env: 'production'
```

### 步骤 4: 配置 AlertManager

编辑 `alertmanager/alertmanager.yml` 配置通知渠道：

#### 邮件通知

```yaml
global:
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@yourdomain.com'
  smtp_auth_username: 'alerts@yourdomain.com'
  smtp_auth_password: 'your-app-password'
  smtp_require_tls: true

receivers:
  - name: 'critical-alerts'
    email_configs:
      - to: 'ops-team@yourdomain.com'
        headers:
          Subject: '[CRITICAL] New API Alert: {{ .GroupLabels.alertname }}'
```

#### Webhook 通知

```yaml
receivers:
  - name: 'critical-alerts'
    webhook_configs:
      - url: 'https://your-webhook-endpoint.com/alerts'
        send_resolved: true
```

#### 企业微信/钉钉/飞书通知

参考 `alertmanager/alertmanager.yml` 中的注释示例。

### 步骤 5: 启动服务

```bash
# 启动所有服务
docker-compose -f docker-compose.prometheus.yml up -d

# 查看服务状态
docker-compose -f docker-compose.prometheus.yml ps

# 查看日志
docker-compose -f docker-compose.prometheus.yml logs -f

# 查看特定服务日志
docker-compose -f docker-compose.prometheus.yml logs -f prometheus
docker-compose -f docker-compose.prometheus.yml logs -f grafana
```

### 步骤 6: 配置 Grafana

1. **访问 Grafana**
   - URL: http://localhost:3001
   - 默认账号: admin
   - 默认密码: admin123
   - 首次登录后建议修改密码

2. **验证数据源**
   - 导航到 Configuration → Data Sources
   - 确认 Prometheus 数据源已自动配置
   - 点击 "Test" 按钮验证连接

3. **导入 Dashboard**

   参考 `grafana/dashboards/README.md` 中的说明创建 Dashboard。

   **快速创建核心面板**:

   a. 创建新 Dashboard:
   - 点击 "+" → "Create Dashboard"
   - 点击 "Add visualization"
   - 选择 "Prometheus" 数据源

   b. 添加"总成功率"面板:
   - Panel type: Stat
   - Query:
     ```promql
     sum(rate(new_api_model_requests_total{status="success"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100
     ```
   - Unit: Percent (0-100)
   - Thresholds: 0→Red, 95→Orange, 98→Yellow, 99→Green

   c. 添加"渠道成功率排名"面板:
   - Panel type: Bar gauge
   - Query:
     ```promql
     sum by (channel_name) (rate(new_api_model_requests_total{status="success"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 100
     ```
   - Orientation: Horizontal
   - Display mode: Gradient

4. **配置变量**
   - Dashboard settings → Variables → Add variable
   - 添加三个变量: site_id, channel, model
   - 参考 `grafana/dashboards/README.md` 中的配置

## 配置说明

### Prometheus Metrics 说明

New API 暴露的 metrics：

1. **new_api_model_requests_total**
   - Type: Counter
   - Labels: channel_id, channel_name, channel_type, model_name, status, error_code, site_id
   - 说明: 模型请求总数

2. **new_api_model_request_duration_seconds**
   - Type: Histogram
   - Labels: channel_id, channel_name, channel_type, model_name, site_id
   - 说明: 模型请求响应时间

3. **new_api_model_request_errors_total**
   - Type: Counter
   - Labels: channel_id, channel_name, channel_type, model_name, error_code, error_message, site_id
   - 说明: 模型请求错误总数

4. **new_api_channel_status**
   - Type: Gauge
   - Labels: channel_id, channel_name, channel_type, status, site_id
   - 说明: 渠道状态 (1=enabled, 2=manually_disabled, 3=auto_disabled, 4=deleted)

5. **new_api_active_requests**
   - Type: Gauge
   - Labels: channel_id, channel_name, channel_type, model_name, site_id
   - 说明: 当前活跃请求数

### 告警规则说明

#### 渠道成功率告警

- **ChannelLowSuccessRate**: 渠道成功率 < 95%，持续5分钟
- **ChannelCriticalSuccessRate**: 渠道成功率 < 90%，持续2分钟
- **ChannelLowTraffic**: 渠道请求量异常下降

#### 模型成功率告警

- **ModelLowSuccessRate**: 模型成功率 < 90%，持续5分钟
- **ModelCriticalSuccessRate**: 模型成功率 < 80%，持续2分钟
- **ModelHighLatency**: 模型 P95 响应时间 > 30秒

#### 错误告警

- **HighErrorRate**: 特定错误码频率 > 10次/秒
- **RateLimitErrors**: 429错误频率 > 5次/秒
- **ServerErrors**: 5xx错误频率 > 3次/秒
- **HighOverallErrorRate**: 系统整体错误率 > 10%

#### 渠道状态告警

- **ChannelDisabled**: 渠道被禁用
- **ChannelNoRequests**: 在线渠道10分钟无请求
- **HighActiveRequests**: 活跃请求数 > 100

## 验证和测试

### 1. 验证 Metrics 采集

```bash
# 检查 New API metrics endpoint
curl http://localhost:3000/metrics

# 应该看到类似输出:
# new_api_model_requests_total{channel_id="1",channel_name="OpenAI-Main",...} 1234
# new_api_model_request_duration_seconds_bucket{...} 567
```

### 2. 验证 Prometheus

访问 http://localhost:9090

```promql
# 测试查询 1: 查看所有 metrics
new_api_model_requests_total

# 测试查询 2: 计算成功率
sum(rate(new_api_model_requests_total{status="success"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100

# 测试查询 3: 查看渠道状态
new_api_channel_status
```

### 3. 验证告警规则

在 Prometheus UI 中:
- 导航到 Status → Rules
- 确认所有告警规则已加载
- 导航到 Alerts 查看当前告警状态

### 4. 测试告警发送

```bash
# 触发测试告警（使用 AlertManager API）
curl -X POST http://localhost:9093/api/v1/alerts \
  -H 'Content-Type: application/json' \
  -d '[{
    "labels": {
      "alertname": "TestAlert",
      "severity": "warning"
    },
    "annotations": {
      "summary": "This is a test alert"
    }
  }]'

# 查看 AlertManager UI
# 访问 http://localhost:9093
```

### 5. 验证 Grafana Dashboard

1. 访问 http://localhost:3001
2. 登录后创建测试请求
3. 等待 15-30 秒（数据采集间隔）
4. 刷新 Dashboard，应该看到数据更新

## 常见问题

### Q1: Prometheus 无法抓取 New API 的 metrics

**问题**: Prometheus UI 中显示 target 状态为 "down"

**解决方案**:

```bash
# 1. 检查 New API 是否启用了 Prometheus
echo $PROMETHEUS_ENABLED  # 应该是 true

# 2. 检查 metrics endpoint 是否可访问
curl http://localhost:3000/metrics

# 3. 如果在 Docker 网络中，检查网络连接
docker-compose -f docker-compose.prometheus.yml exec prometheus ping new-api

# 4. 检查 prometheus.yml 中的 targets 配置
cat prometheus/prometheus.yml | grep targets
```

### Q2: Grafana 无法连接 Prometheus

**问题**: Grafana 中数据源测试失败

**解决方案**:

```bash
# 1. 确认 Prometheus 服务正在运行
docker-compose -f docker-compose.prometheus.yml ps prometheus

# 2. 在 Grafana 容器中测试连接
docker-compose -f docker-compose.prometheus.yml exec grafana curl http://prometheus:9090/api/v1/status/config

# 3. 检查数据源配置
cat grafana/provisioning/datasources/prometheus.yml
```

### Q3: Dashboard 没有数据

**问题**: Grafana Dashboard 显示 "No data"

**解决方案**:

```bash
# 1. 确认时间范围正确（Dashboard 右上角）

# 2. 在 Prometheus UI 中测试查询
# 访问 http://localhost:9090，执行查询:
new_api_model_requests_total

# 3. 检查变量是否正确配置
# Dashboard settings → Variables → 确认变量有值

# 4. 生成一些测试请求
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}'
```

### Q4: 告警没有触发

**问题**: 明明达到告警阈值但没有收到通知

**解决方案**:

```bash
# 1. 检查告警规则是否正确加载
# 访问 http://localhost:9090/rules

# 2. 查看告警状态
# 访问 http://localhost:9090/alerts

# 3. 检查 AlertManager 配置
docker-compose -f docker-compose.prometheus.yml logs alertmanager

# 4. 测试通知渠道
# 访问 http://localhost:9093，手动创建测试告警
```

### Q5: Prometheus 存储空间不足

**问题**: Docker volume 空间占用过大

**解决方案**:

```bash
# 1. 查看当前存储使用
docker volume inspect new-api_prometheus_data

# 2. 调整数据保留时间（修改 docker-compose.prometheus.yml）
# 将 --storage.tsdb.retention.time=30d 改为 15d 或 7d

# 3. 重启 Prometheus
docker-compose -f docker-compose.prometheus.yml restart prometheus

# 4. 清理旧数据（慎用！）
docker-compose -f docker-compose.prometheus.yml down
docker volume rm new-api_prometheus_data
docker-compose -f docker-compose.prometheus.yml up -d
```

## 性能调优

### Prometheus 优化

```yaml
# prometheus/prometheus.yml
global:
  # 调整抓取间隔（默认15s）
  scrape_interval: 30s     # 减少抓取频率可降低负载
  evaluation_interval: 30s

  # 启用压缩
  compression: gzip

scrape_configs:
  - job_name: 'new-api'
    # 针对单个job调整
    scrape_interval: 15s
    scrape_timeout: 10s

    # 只抓取特定metrics（可选）
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'new_api_.*'
        action: keep
```

### 存储优化

```bash
# 在 docker-compose.prometheus.yml 中添加
services:
  prometheus:
    command:
      - '--storage.tsdb.retention.time=15d'        # 数据保留15天
      - '--storage.tsdb.retention.size=10GB'       # 最大存储10GB
      - '--storage.tsdb.min-block-duration=2h'     # 最小块持续时间
      - '--storage.tsdb.max-block-duration=24h'    # 最大块持续时间
```

### Grafana 优化

1. **使用查询缓存**
   - Dashboard settings → JSON Model
   - 添加 `"cacheTimeout": 60` 到 panel 配置

2. **优化查询时间范围**
   - 避免查询过长时间范围（> 7天）
   - 使用较短的 refresh 间隔（30s-1min）

3. **减少并发查询**
   - 每个 Dashboard 不超过 20 个 panel
   - 使用变量减少重复查询

### New API 性能影响

Prometheus metrics 采集对 New API 的性能影响：

- **CPU 增加**: < 2%
- **内存增加**: 10-20MB
- **请求延迟增加**: < 1ms

**建议**:
- 在生产环境中始终启用
- 监控系统本身也需要监控
- 定期检查 metrics 基数（cardinality）

## 生产环境最佳实践

### 1. 高可用部署

```yaml
# 使用多个 Prometheus 实例（联邦模式）
# 或使用 Prometheus Operator + Thanos
```

### 2. 数据备份

```bash
# 定期备份 Prometheus 数据
docker run --rm -v new-api_prometheus_data:/data -v $(pwd)/backup:/backup alpine tar czf /backup/prometheus-$(date +%Y%m%d).tar.gz /data

# 备份 Grafana 配置
docker exec new-api-grafana grafana-cli admin export-dashboard > grafana-backup.json
```

### 3. 安全加固

```yaml
# prometheus/web-config.yml
basic_auth_users:
  admin: $2y$10$... # bcrypt hash

tls_server_config:
  cert_file: /etc/prometheus/tls/cert.pem
  key_file: /etc/prometheus/tls/key.pem

# 在 docker-compose.prometheus.yml 中挂载
volumes:
  - ./prometheus/web-config.yml:/etc/prometheus/web-config.yml
command:
  - '--web.config.file=/etc/prometheus/web-config.yml'
```

### 4. 日志管理

```bash
# 配置日志轮转
docker-compose -f docker-compose.prometheus.yml logs --tail=1000 prometheus > prometheus.log

# 或使用 Docker logging driver
services:
  prometheus:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## 下一步

- 阅读 [Prometheus 用户手册](docs/PROMETHEUS_USER_MANUAL.md)
- 配置自定义告警规则
- 创建自定义 Grafana Dashboard
- 集成第三方监控平台（如 Datadog, New Relic）

## 获取帮助

如有问题或需要帮助:
- 查看[需求文档](docs/PROMETHEUS_MONITORING_REQUIREMENTS.md)
- 提交 [GitHub Issue](https://github.com/your-org/new-api/issues)
- 加入社区讨论

---

**文档版本**: v1.0
**最后更新**: 2025-12-03
**维护者**: New API Team
