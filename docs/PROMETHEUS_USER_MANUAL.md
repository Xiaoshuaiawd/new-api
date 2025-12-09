# Prometheus 监控使用手册

本手册介绍如何使用 New API 的 Prometheus 监控系统进行日常运维和故障排查。

## 目录

1. [快速开始](#快速开始)
2. [Grafana Dashboard 使用](#grafana-dashboard-使用)
3. [常见监控场景](#常见监控场景)
4. [告警处理](#告警处理)
5. [故障排查](#故障排查)
6. [性能分析](#性能分析)

## 快速开始

### 访问监控系统

- **Grafana**: http://localhost:3001
  - 账号: `admin`
  - 密码: `admin123`
- **Prometheus**: http://localhost:9090
- **AlertManager**: http://localhost:9093

### Dashboard 概览

登录 Grafana 后，主 Dashboard 显示以下内容：

```
┌─────────────────────────────────────────┐
│ 📊 概览指标（顶部4个大卡片）              │
├─────────────────────────────────────────┤
│ 🎯 总成功率  📈 总请求数                 │
│ ⚡ 活跃请求  ⏱️ P50延迟                  │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│ 📊 渠道维度分析                          │
├─────────────────────────────────────────┤
│ • 渠道成功率排名                         │
│ • 渠道成功率趋势                         │
│ • 渠道请求量分布                         │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│ 🔬 模型维度分析                          │
├─────────────────────────────────────────┤
│ • 模型成功率排名                         │
│ • 模型成功率趋势                         │
│ • 模型平均响应时间                       │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│ ❌ 错误详情                              │
├─────────────────────────────────────────┤
│ • Top 10 错误类型                        │
│ • 错误码分布                             │
│ • 错误趋势图                             │
└─────────────────────────────────────────┘
```

## Grafana Dashboard 使用

### 1. 时间范围选择

点击右上角的时间选择器：

- **快速选择**: Last 5m, 15m, 1h, 6h, 24h, 7d, 30d
- **自定义范围**: 点击 "Custom range" 设置开始和结束时间
- **刷新间隔**: 点击刷新按钮旁边的下拉菜单选择自动刷新间隔（推荐30s）

### 2. 使用筛选变量

Dashboard 顶部提供三个筛选器：

#### 站点筛选器
```
选择器: [站点]
值: default, site1, site2...
用途: 多站点部署时选择特定站点
```

#### 渠道筛选器
```
选择器: [渠道]
值: All, OpenAI-Main, Claude-1, Gemini-Backup...
用途: 筛选特定渠道的数据
支持: 多选
```

#### 模型筛选器
```
选择器: [模型]
值: All, gpt-4, gpt-3.5-turbo, claude-3-opus...
用途: 筛选特定模型的数据
依赖: 根据选择的渠道动态更新
支持: 多选
```

**使用示例**:

1. 查看所有渠道所有模型 → [All] + [All]
2. 查看 OpenAI 渠道所有模型 → [OpenAI-Main] + [All]
3. 查看特定模型在所有渠道 → [All] + [gpt-4]
4. 对比多个渠道 → [OpenAI-Main, Claude-1] + [All]

### 3. 面板交互

#### 放大查看
- 点击面板标题，选择 "View"
- 或直接点击面板内的图表区域

#### 查看详细数据
- 鼠标悬停在图表上查看具体数值
- 点击图例可以隐藏/显示特定数据系列

#### 导出数据
- 点击面板标题 → "Inspect" → "Data"
- 选择"Download CSV"导出数据

#### 共享面板
- 点击面板标题 → "Share"
- 复制链接或生成快照

### 4. Dashboard 管理

#### 保存 Dashboard
```
1. 修改 Dashboard 后，点击右上角 "Save dashboard"
2. 输入保存说明
3. 点击 "Save"
```

#### 创建副本
```
1. 点击 Dashboard settings（齿轮图标）
2. 选择 "Save As..."
3. 输入新名称
4. 点击 "Save"
```

#### 导出/导入
```
导出:
1. Dashboard settings → JSON Model
2. 点击 "Copy to Clipboard" 或 "Save to file"

导入:
1. 点击 "+" → "Import"
2. 粘贴 JSON 或上传文件
3. 选择数据源
4. 点击 "Import"
```

## 常见监控场景

### 场景 1: 检查系统整体健康状况

**步骤**:
1. 打开主 Dashboard
2. 查看顶部4个概览指标:
   - **总成功率**: 应 > 99%（绿色）
   - **总请求数**: 了解系统负载
   - **活跃请求数**: 应 < 100（绿色）
   - **P50延迟**: 应 < 1000ms（绿色）

**判断标准**:
- ✅ 正常: 成功率 > 99%, 延迟 < 1s, 活跃请求 < 50
- ⚠️ 需关注: 成功率 95-99%, 延迟 1-3s, 活跃请求 50-100
- 🔴 异常: 成功率 < 95%, 延迟 > 3s, 活跃请求 > 100

### 场景 2: 排查特定渠道问题

**问题**: 收到 "渠道 OpenAI-Main 成功率低于95%" 的告警

**排查步骤**:

1. **定位渠道**
   ```
   筛选器设置: 渠道 = [OpenAI-Main]
   时间范围: Last 1h
   ```

2. **查看渠道成功率趋势图**
   - 找到"渠道成功率趋势"面板
   - 观察成功率何时开始下降
   - 记录时间点

3. **查看错误详情**
   - 滚动到"Top 10 错误详情"表格
   - 筛选该渠道的错误
   - 记录主要错误码和错误信息

4. **分析错误类型**
   ```
   错误码 429 → 限流问题 → 减少请求频率或增加渠道
   错误码 500/502/503 → 上游服务问题 → 联系API提供商
   错误码 401/403 → 认证问题 → 检查API密钥
   错误码 timeout → 超时问题 → 检查网络或增加超时时间
   ```

5. **查看模型分布**
   - 查看"模型成功率排名"
   - 确认是所有模型还是特定模型出问题

### 场景 3: 性能分析

**问题**: 用户反馈响应慢

**分析步骤**:

1. **查看整体延迟**
   ```
   面板: P50/P95/P99 延迟
   正常值: P50 < 1s, P95 < 3s, P99 < 5s
   ```

2. **按渠道分析延迟**
   ```
   PromQL查询:
   histogram_quantile(0.95, sum by (channel_name, le) (rate(new_api_model_request_duration_seconds_bucket[5m])))
   ```

3. **按模型分析延迟**
   ```
   筛选器: 选择慢的渠道
   面板: 模型平均响应时间
   找出最慢的模型
   ```

4. **检查并发请求**
   ```
   面板: 活跃请求数
   如果过高（>100）→ 系统负载过大
   ```

5. **优化建议**
   - 延迟主要在特定渠道 → 增加该渠道实例或切换渠道
   - 延迟主要在特定模型 → 调整模型权重或禁用该模型
   - 整体延迟高 → 检查网络、数据库、Redis

### 场景 4: 容量规划

**目的**: 评估是否需要扩容

**分析指标**:

1. **请求量趋势**
   ```
   PromQL:
   sum(rate(new_api_model_requests_total[5m])) * 60

   查看:
   - 近7天趋势
   - 峰值时间段
   - 增长率
   ```

2. **渠道负载分布**
   ```
   面板: 渠道请求量分布（饼图）

   判断:
   - 单个渠道占比 > 50% → 有单点风险
   - 分布均匀 → 负载均衡良好
   ```

3. **成功率与负载关系**
   ```
   对比:
   - 高峰期成功率
   - 低峰期成功率

   如果高峰期成功率明显下降 → 需要扩容
   ```

4. **活跃请求数**
   ```
   PromQL:
   max(new_api_active_requests)

   如果经常 > 80% 容量 → 需要扩容
   ```

### 场景 5: 成本优化

**目的**: 降低API调用成本

**分析步骤**:

1. **查看渠道使用分布**
   ```
   面板: 渠道请求量分布
   识别: 使用量最大的渠道
   ```

2. **对比渠道成功率和成本**
   ```
   如果某渠道:
   - 使用量大
   - 成功率高
   - 成本低
   → 应该增加该渠道权重
   ```

3. **识别低效渠道**
   ```
   如果某渠道:
   - 成功率低（< 95%）
   - 响应慢（P95 > 5s）
   - 错误率高
   → 考虑禁用或降低权重
   ```

4. **模型使用分析**
   ```
   面板: 模型请求量分布

   优化:
   - 高成本模型是否可替换为低成本模型？
   - 是否可以引导用户使用更便宜的模型？
   ```

## 告警处理

### 告警级别

| 级别 | 说明 | 响应时间 | 处理优先级 |
|------|------|----------|-----------|
| 🔴 Critical | 严重影响服务 | 立即 | P0 |
| 🟠 Warning | 需要关注 | 30分钟内 | P1 |
| 🔵 Info | 一般信息 | 2小时内 | P2 |

### 告警处理流程

```
收到告警
    ↓
查看告警详情
    ↓
登录 Grafana Dashboard
    ↓
定位问题（使用本手册的排查场景）
    ↓
执行修复操作
    ↓
验证问题已解决
    ↓
记录处理过程
    ↓
标记告警已解决
```

### 常见告警及处理方法

#### 1. ChannelCriticalSuccessRate

**告警内容**: 渠道 XXX 成功率低于90%（严重）

**处理步骤**:
```bash
1. 查看错误详情（见场景2）

2. 如果是429限流:
   - 降低该渠道权重
   - 增加其他渠道
   - 或联系API提供商提升限额

3. 如果是5xx错误:
   - 检查上游服务状态
   - 临时禁用该渠道
   - 切换到备用渠道

4. 如果是认证错误:
   - 检查API密钥是否过期
   - 更新API密钥
   - 测试渠道连接

5. 验证修复:
   # 等待5-10分钟
   # 查看成功率是否恢复
```

#### 2. ModelHighLatency

**告警内容**: 模型 XXX 在渠道 YYY 响应时间过长

**处理步骤**:
```bash
1. 确认延迟数据:
   PromQL: histogram_quantile(0.95, rate(new_api_model_request_duration_seconds_bucket{channel_name="YYY",model_name="XXX"}[5m]))

2. 对比其他渠道的同模型延迟

3. 如果是特定渠道问题:
   - 降低该渠道权重
   - 增加其他渠道权重

4. 如果是模型本身问题:
   - 检查是否是模型参数导致（如max_tokens过大）
   - 考虑切换到更快的模型

5. 如果是整体网络问题:
   - 检查服务器网络
   - 检查DNS解析
   - 检查防火墙规则
```

#### 3. HighErrorRate

**告警内容**: 错误码 XXX 在渠道 YYY 出现频率过高

**处理步骤**:
```bash
1. 查看错误详情表格

2. 根据错误码分类处理:
   - 4xx错误 → 检查请求参数、认证
   - 5xx错误 → 检查上游服务、网络
   - timeout → 检查超时配置、网络

3. 临时缓解:
   - 降低请求频率
   - 启用重试机制
   - 切换到备用渠道

4. 长期解决:
   - 优化请求参数
   - 增加渠道容量
   - 改进错误处理逻辑
```

## 故障排查

### 快速排查检查清单

当收到告警或用户报告问题时，按以下顺序检查：

#### 1. 系统层面检查（1分钟）

```bash
# 检查服务状态
docker-compose ps

# 检查 New API 日志
docker-compose logs --tail=100 new-api

# 检查资源使用
docker stats --no-stream
```

#### 2. Prometheus/Grafana 检查（2分钟）

```
✓ Grafana 总成功率
✓ 活跃请求数
✓ Top 10 错误
✓ 渠道状态
```

#### 3. 渠道层面检查（3分钟）

```
✓ 各渠道成功率
✓ 渠道是否被禁用
✓ 渠道API密钥是否有效
✓ 渠道余额是否充足
```

#### 4. 模型层面检查（2分钟）

```
✓ 特定模型成功率
✓ 模型响应时间
✓ 模型错误分布
```

### 详细故障排查场景

#### 场景：突然大量请求失败

**现象**:
- Grafana显示成功率突降
- 大量错误告警
- 用户投诉无法使用

**排查步骤**:

1. **确定影响范围**
   ```
   Q: 是所有渠道还是特定渠道？
   → 查看渠道成功率排名

   Q: 是所有模型还是特定模型？
   → 查看模型成功率排名

   Q: 什么时候开始的？
   → 查看成功率趋势图，定位时间点
   ```

2. **查看错误详情**
   ```
   查看 Top 10 错误详情表格
   记录:
   - 主要错误码
   - 错误信息
   - 影响的渠道和模型
   ```

3. **检查外部因素**
   ```bash
   # 检查上游API服务状态
   curl https://status.openai.com
   curl https://status.anthropic.com

   # 检查网络连接
   ping api.openai.com
   traceroute api.openai.com

   # 检查DNS解析
   nslookup api.openai.com
   ```

4. **检查配置变更**
   ```bash
   # 查看最近的配置变更
   git log --since="2 hours ago"

   # 查看环境变量
   docker-compose config

   # 查看渠道配置
   # 登录 New API 管理后台检查
   ```

5. **临时修复**
   ```bash
   # 如果是特定渠道问题，禁用该渠道
   # 在 New API 管理后台操作

   # 如果是所有渠道问题，检查 New API 服务
   docker-compose restart new-api

   # 查看重启后的日志
   docker-compose logs -f new-api
   ```

#### 场景：性能缓慢

**现象**:
- 用户反馈响应慢
- P95延迟 > 5s
- 活跃请求数持续偏高

**排查步骤**:

1. **识别瓶颈**
   ```
   检查 Grafana:
   - P50/P95/P99 延迟趋势
   - 按渠道的响应时间
   - 按模型的响应时间
   - 活跃请求数
   ```

2. **检查系统资源**
   ```bash
   # CPU使用率
   docker stats --no-stream

   # 内存使用率
   free -h

   # 磁盘IO
   iostat -x 1 5

   # 网络带宽
   iftop
   ```

3. **检查数据库性能**
   ```bash
   # 如果使用MySQL
   mysql> SHOW PROCESSLIST;
   mysql> SHOW ENGINE INNODB STATUS;

   # 如果使用PostgreSQL
   psql> SELECT * FROM pg_stat_activity;
   ```

4. **检查Redis性能**
   ```bash
   # 连接Redis
   redis-cli

   # 检查慢查询
   SLOWLOG GET 10

   # 检查内存使用
   INFO memory

   # 检查连接数
   INFO clients
   ```

5. **分析请求分布**
   ```
   Grafana查询:
   - 哪个渠道请求量最大？
   - 哪个模型请求量最大？
   - 请求是否均匀分布？
   ```

6. **优化措施**
   ```
   如果CPU高:
   - 增加服务器CPU核心数
   - 使用更快的CPU

   如果内存高:
   - 增加服务器内存
   - 优化缓存策略

   如果网络慢:
   - 检查网络带宽
   - 使用CDN或就近部署

   如果数据库慢:
   - 添加索引
   - 优化查询
   - 增加连接池

   如果特定渠道慢:
   - 降低该渠道权重
   - 增加其他渠道
   ```

## 性能分析

### 使用 Prometheus 直接查询

访问 http://localhost:9090，使用以下查询：

#### 1. 查看请求速率
```promql
# 每秒请求数（QPS）
sum(rate(new_api_model_requests_total[5m])) * 60

# 按渠道分组的QPS
sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 60

# 按模型分组的QPS
sum by (model_name) (rate(new_api_model_requests_total[5m])) * 60
```

#### 2. 计算成功率
```promql
# 总成功率
sum(rate(new_api_model_requests_total{status="success"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100

# 按渠道的成功率
sum by (channel_name) (rate(new_api_model_requests_total{status="success"}[5m])) / sum by (channel_name) (rate(new_api_model_requests_total[5m])) * 100

# 按模型的成功率
sum by (model_name) (rate(new_api_model_requests_total{status="success"}[5m])) / sum by (model_name) (rate(new_api_model_requests_total[5m])) * 100
```

#### 3. 分析响应时间
```promql
# P50延迟
histogram_quantile(0.50, sum(rate(new_api_model_request_duration_seconds_bucket[5m])) by (le))

# P95延迟
histogram_quantile(0.95, sum(rate(new_api_model_request_duration_seconds_bucket[5m])) by (le))

# P99延迟
histogram_quantile(0.99, sum(rate(new_api_model_request_duration_seconds_bucket[5m])) by (le))

# 按渠道的P95延迟
histogram_quantile(0.95, sum by (channel_name, le) (rate(new_api_model_request_duration_seconds_bucket[5m])))
```

#### 4. 错误分析
```promql
# 总错误率
sum(rate(new_api_model_requests_total{status="failed"}[5m])) / sum(rate(new_api_model_requests_total[5m])) * 100

# 按错误码分组的错误数
sum by (error_code) (rate(new_api_model_request_errors_total[5m]))

# Top 5 错误
topk(5, sum by (error_code, error_message) (rate(new_api_model_request_errors_total[5m])))
```

#### 5. 容量分析
```promql
# 当前活跃请求数
sum(new_api_active_requests)

# 活跃请求数趋势
sum(new_api_active_requests) by (channel_name)

# 渠道状态
new_api_channel_status
```

### 导出数据进行深度分析

#### 1. 从 Prometheus 导出数据

```bash
# 使用 Prometheus API 导出数据
curl 'http://localhost:9090/api/v1/query_range?query=sum(rate(new_api_model_requests_total[5m]))&start=2025-12-03T00:00:00Z&end=2025-12-03T23:59:59Z&step=60s' > data.json

# 使用 promtool 导出
promtool query range \
  --start=2025-12-03T00:00:00Z \
  --end=2025-12-03T23:59:59Z \
  'sum(rate(new_api_model_requests_total[5m]))' \
  http://localhost:9090
```

#### 2. 从 Grafana 导出数据

```
1. 打开面板
2. 点击面板标题 → "Inspect" → "Data"
3. 点击 "Download CSV"
```

#### 3. 使用 Python 分析

```python
import pandas as pd
import matplotlib.pyplot as plt

# 读取导出的CSV
df = pd.read_csv('data.csv')

# 分析成功率趋势
df['success_rate'] = df['success'] / df['total'] * 100

# 绘图
plt.figure(figsize=(12, 6))
plt.plot(df['timestamp'], df['success_rate'])
plt.xlabel('Time')
plt.ylabel('Success Rate (%)')
plt.title('Success Rate Trend')
plt.grid(True)
plt.savefig('success_rate_trend.png')
```

## 最佳实践

### 日常运维检查清单

#### 每天检查（5分钟）
- [ ] 查看 Grafana 总成功率（应 > 99%）
- [ ] 查看活跃告警数量
- [ ] 查看 Top 10 错误列表
- [ ] 检查是否有渠道被禁用

#### 每周检查（30分钟）
- [ ] 分析成功率趋势（过去7天）
- [ ] 分析请求量趋势
- [ ] 检查渠道负载分布
- [ ] 查看性能指标（P95延迟）
- [ ] 检查错误类型分布
- [ ] 审查告警历史

#### 每月检查（2小时）
- [ ] 生成月度报告
- [ ] 分析容量趋势，评估是否需要扩容
- [ ] 优化渠道配置
- [ ] 优化模型权重
- [ ] 审查成本效益
- [ ] 更新监控规则
- [ ] 备份 Prometheus 数据
- [ ] 备份 Grafana 配置

### 监控告警配置建议

```yaml
# 告警配置原则
1. 不要设置太多告警（会导致告警疲劳）
2. 告警阈值要合理（根据历史数据调整）
3. 重要告警立即通知，一般告警可以聚合
4. 告警消息要清晰，包含必要的上下文
5. 定期审查告警规则，删除不必要的规则
```

### Dashboard 使用技巧

1. **使用变量简化Dashboard**
   - 创建渠道、模型变量
   - 使用变量可以减少面板数量

2. **合理设置刷新间隔**
   - 生产环境: 30s-1min
   - 开发环境: 5s-10s
   - 历史数据分析: 关闭自动刷新

3. **使用注释标记重要事件**
   - 部署: 标记部署时间
   - 事故: 标记事故开始和结束时间
   - 配置变更: 标记配置变更时间

4. **创建多个 Dashboard**
   - 概览 Dashboard: 高层视图
   - 详细 Dashboard: 深入分析
   - 告警 Dashboard: 专注于告警

## 获取帮助

### 文档资源
- [部署指南](PROMETHEUS_DEPLOYMENT_GUIDE.md)
- [需求文档](PROMETHEUS_MONITORING_REQUIREMENTS.md)
- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Grafana 官方文档](https://grafana.com/docs/)

### 社区支持
- GitHub Issues: https://github.com/your-org/new-api/issues
- 讨论区: https://github.com/your-org/new-api/discussions

### 常见问题
查看 [FAQ](PROMETHEUS_FAQ.md)

---

**文档版本**: v1.0
**最后更新**: 2025-12-03
**维护者**: New API Team
