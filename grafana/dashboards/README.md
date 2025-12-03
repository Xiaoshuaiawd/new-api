# Grafana Dashboard

✅ **完整的 Dashboard JSON 文件已创建！**

文件位置：`grafana/dashboards/new-api-monitoring.json`

## 快速导入 Dashboard

### 方式一：直接导入 JSON 文件（推荐）

1. 登录 Grafana（http://localhost:3001，默认账号密码：admin/admin123）
2. 点击左侧菜单 "+" → "Import"
3. 点击 "Upload JSON file"
4. 选择 `grafana/dashboards/new-api-monitoring.json` 文件
5. 在 "Prometheus" 下拉菜单中选择你的 Prometheus 数据源
6. 点击 "Import"

### 方式二：复制粘贴 JSON

1. 打开 `grafana/dashboards/new-api-monitoring.json` 文件
2. 复制全部内容
3. 登录 Grafana，点击 "+" → "Import"
4. 将 JSON 内容粘贴到文本框
5. 选择 Prometheus 数据源
6. 点击 "Import"

### 方式三：使用 Provisioning 自动加载

Dashboard 会通过 `grafana/provisioning/dashboards/dashboards.yml` 自动加载。

确保在 `docker-compose.prometheus.yml` 中配置了正确的挂载：

```yaml
grafana:
  volumes:
    - ./grafana/dashboards:/var/lib/grafana/dashboards
    - ./grafana/provisioning:/etc/grafana/provisioning
```

重启 Grafana 后，Dashboard 会自动出现在 "New API" 文件夹中。

## Dashboard 概览

Dashboard 包含以下部分：

### 1. 概览指标（4个大卡片）
- 总成功率
- 总请求数
- 活跃请求数
- P50延迟

### 2. 渠道维度分析
- 渠道成功率排名（横向柱状图）
- 渠道成功率趋势（时序图）
- 渠道请求量分布（饼图）
- 渠道响应时间对比（柱状图）

### 3. 模型维度分析
- 模型成功率排名（横向柱状图）
- 模型成功率趋势（时序图）
- 模型平均响应时间（柱状图）
- 模型请求量分布（饼图）

### 4. 错误详情
- Top 10 错误详情表格
- 错误码分布（饼图）
- 错误趋势堆叠图

## 快速导入

### 方式一：使用 Grafana UI 创建

1. 登录 Grafana（http://localhost:3001，默认账号密码：admin/admin123）
2. 点击左侧菜单 "+" → "Create Dashboard"
3. 点击 "Add visualization"
4. 选择 "Prometheus" 数据源
5. 根据下面的查询语句配置面板

### 方式二：使用完整JSON文件

完整的 Dashboard JSON 文件可以使用以下命令生成：

```bash
cd /Users/zhangwenshuai/Desktop/副业类/new-api
python3 scripts/generate_dashboard.py
```

或者访问 Grafana Dashboards 官网，搜索 "New API" 获取社区版本。

## 核心 PromQL 查询

### 总成功率
```promql
sum(rate(new_api_model_requests_total{status="success"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100
```

### 渠道成功率排名
```promql
sum by (channel_name) (rate(new_api_model_requests_total{status="success"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 100
```

### 模型成功率（按渠道筛选）
```promql
sum by (model_name) (rate(new_api_model_requests_total{status="success", channel_name=~"$channel"}[5m])) / sum by (model_name) (rate(new_api_model_requests_total{channel_name=~"$channel"}[5m])) * 100
```

### Top 10 错误
```promql
topk(10, sum by (error_code, channel_name, model_name, error_message) (increase(new_api_model_request_errors_total[1h])))
```

### 响应时间 P95
```promql
histogram_quantile(0.95, sum by (channel_name, le) (rate(new_api_model_request_duration_seconds_bucket[5m])))
```

## Dashboard 变量

配置以下变量以支持动态筛选：

1. **site_id**
   - Type: Query
   - Query: `label_values(new_api_model_requests_total, site_id)`
   - Multi: false
   - Include All: false

2. **channel**
   - Type: Query
   - Query: `label_values(new_api_model_requests_total{site_id=~"$site_id"}, channel_name)`
   - Multi: true
   - Include All: true

3. **model**
   - Type: Query
   - Query: `label_values(new_api_model_requests_total{site_id=~"$site_id", channel_name=~"$channel"}, model_name)`
   - Multi: true
   - Include All: true

## 面板配置示例

### 渠道成功率柱状图

**Type**: Bar gauge
**Query**:
```promql
sum by (channel_name) (rate(new_api_model_requests_total{status="success", site_id=~"$site_id"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total{site_id=~"$site_id"}[5m])) * 100
```
**Options**:
- Orientation: Horizontal
- Display mode: Gradient
- Show unfilled: true
**Thresholds**:
- 0-95: Red
- 95-98: Orange
- 98-99.5: Yellow
- 99.5-100: Green

### Top 10 错误表格

**Type**: Table
**Query**:
```promql
topk(10, sum by (error_code, channel_name, model_name, error_message) (increase(new_api_model_request_errors_total[1h])))
```
**Format**: Table
**Instant**: true
**Transformations**:
- Organize fields: 移除 Time 字段
- Rename fields:
  - error_code → 错误码
  - Value → 次数
  - channel_name → 渠道
  - model_name → 模型
  - error_message → 错误信息

## 主题和样式

- 主题：Dark
- 主色调：蓝色系 (#3b82f6)
- 成功色：绿色 (#10b981)
- 警告色：橙色 (#f59e0b)
- 错误色：红色 (#ef4444)
- 刷新间隔：30秒
- 时间范围：最近24小时

## 需要帮助？

如果需要完整的Dashboard JSON文件或有任何问题，请查看：
- 项目文档：`docs/PROMETHEUS_DEPLOYMENT_GUIDE.md`
- GitHub Issues: https://github.com/your-org/new-api/issues
