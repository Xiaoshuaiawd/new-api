# Docker Compose 整合完成说明

## ✅ 完成内容

已成功将 Prometheus 监控服务整合到主 `docker-compose.yml` 文件中，所有服务现在运行在同一个 Docker 网络 `new-api-network` 中。

## 📝 主要变更

### 1. 合并 docker-compose 文件

**之前**:
- `docker-compose.yml` - New API 应用 + Redis + PostgreSQL
- `docker-compose.prometheus.yml` - Prometheus + Grafana + AlertManager (独立网络)

**现在**:
- `docker-compose.yml` - 所有服务在一个文件中，共享 `new-api-network` 网络

### 2. 添加的服务

在主 `docker-compose.yml` 中新增了以下监控服务：

#### Prometheus (监控数据采集)
```yaml
prometheus:
  image: prom/prometheus:latest
  container_name: new-api-prometheus
  ports: "9090:9090"
  networks: new-api-network
  depends_on: new-api
```

#### Grafana (可视化面板)
```yaml
grafana:
  image: grafana/grafana:latest
  container_name: new-api-grafana
  ports: "3001:3000"
  networks: new-api-network
  depends_on: prometheus
```

#### AlertManager (告警管理)
```yaml
alertmanager:
  image: prom/alertmanager:latest
  container_name: new-api-alertmanager
  ports: "9093:9093"
  networks: new-api-network
```

### 3. 网络配置

所有服务现在都加入了统一的 `new-api-network` 网络：

```yaml
networks:
  new-api-network:
    driver: bridge
```

这确保了：
- ✅ Prometheus 可以通过 `new-api:3000/metrics` 抓取指标
- ✅ Grafana 可以通过 `prometheus:9090` 访问 Prometheus
- ✅ AlertManager 可以通过 `alertmanager:9093` 接收告警
- ✅ 所有服务之间可以通过服务名进行网络通信

### 4. 环境变量配置

在 `new-api` 服务中添加了 Prometheus 启用标志：

```yaml
environment:
  - PROMETHEUS_ENABLED=true  # 启用 Prometheus 监控
```

### 5. 依赖配置

添加了正确的服务依赖关系：

```yaml
prometheus:
  depends_on:
    - new-api

grafana:
  depends_on:
    - prometheus
```

### 6. 新增 volumes

添加了监控服务的数据持久化卷：

```yaml
volumes:
  pg_data:          # PostgreSQL 数据
  prometheus_data:  # Prometheus 时序数据
  grafana_data:     # Grafana 配置和面板
  alertmanager_data: # AlertManager 数据
```

## 🚀 快速开始

### 启动所有服务

```bash
# 1. 进入项目目录
cd /Users/zhangwenshuai/Desktop/副业类/new-api

# 2. 一键启动所有服务（包括监控）
docker-compose up -d

# 3. 查看服务状态
docker-compose ps
```

### 访问服务

启动后可以访问以下服务：

| 服务 | 地址 | 默认账号 | 说明 |
|------|------|----------|------|
| New API | http://localhost:3000 | - | 主应用 |
| Grafana | http://localhost:3001 | admin/admin123 | 监控面板 |
| Prometheus | http://localhost:9090 | - | 监控数据 |
| AlertManager | http://localhost:9093 | - | 告警管理 |

### 验证网络连通性

```bash
# 进入 prometheus 容器
docker exec -it new-api-prometheus sh

# 测试连接 new-api
wget -O- http://new-api:3000/metrics

# 应该能看到 Prometheus metrics 输出
```

## 📊 导入 Grafana Dashboard

### 方式一：自动加载（推荐）

Dashboard 会通过 Grafana Provisioning 自动加载：

1. 确保 `grafana/dashboards/new-api-monitoring.json` 文件存在
2. 启动服务后等待 1-2 分钟
3. 登录 Grafana，在左侧菜单找到 "Dashboards"
4. Dashboard 会自动出现

### 方式二：手动导入

1. 登录 Grafana (http://localhost:3001)
2. 点击左侧菜单 "+" → "Import"
3. 点击 "Upload JSON file"
4. 选择 `grafana/dashboards/new-api-monitoring.json`
5. 选择 Prometheus 数据源
6. 点击 "Import"

## 🔧 配置说明

### 停止监控服务（可选）

如果只想运行 New API 而不需要监控服务：

```bash
# 方式 1: 停止特定服务
docker-compose stop prometheus grafana alertmanager

# 方式 2: 移除环境变量
# 在 docker-compose.yml 中注释掉这一行:
# - PROMETHEUS_ENABLED=true
```

### 修改端口映射

如果默认端口有冲突，可以修改 `docker-compose.yml` 中的端口映射：

```yaml
# 例如将 Grafana 从 3001 改为 3002
grafana:
  ports:
    - "3002:3000"  # 左边是宿主机端口，右边是容器端口
```

### 使用 MySQL 而不是 PostgreSQL

在 `docker-compose.yml` 中：

```yaml
# 1. 注释掉 postgres 服务和 SQL_DSN
#   - SQL_DSN=postgresql://root:123456@postgres:5432/new-api

# 2. 取消注释 mysql 服务和 SQL_DSN
services:
  new-api:
    environment:
      - SQL_DSN=root:123456@tcp(mysql:3306)/new-api
    depends_on:
      - mysql

  mysql:
    # ... (取消注释整个 mysql 服务块)
```

## 🔒 安全建议

### ⚠️ 生产环境必须修改的密码

在 `docker-compose.yml` 中修改以下默认密码：

```yaml
# 1. PostgreSQL 密码
postgres:
  environment:
    POSTGRES_PASSWORD: 123456  # ⚠️ 改为强密码

new-api:
  environment:
    # ⚠️ 同时修改连接字符串中的密码
    - SQL_DSN=postgresql://root:YOUR_NEW_PASSWORD@postgres:5432/new-api

# 2. Grafana 密码
grafana:
  environment:
    - GF_SECURITY_ADMIN_PASSWORD=admin123  # ⚠️ 改为强密码
```

### 其他安全配置

```yaml
# 3. 多机部署时设置 SESSION_SECRET
new-api:
  environment:
    - SESSION_SECRET=your-random-secret-string-here
```

## 🐛 故障排查

### 问题 1: Prometheus 无法抓取 metrics

**症状**: Prometheus Targets 页面显示 new-api 状态为 DOWN

**解决方法**:

```bash
# 1. 检查 new-api 服务是否启用了 Prometheus
docker-compose logs new-api | grep PROMETHEUS

# 2. 测试 metrics endpoint
curl http://localhost:3000/metrics

# 3. 检查网络连通性
docker exec -it new-api-prometheus wget -O- http://new-api:3000/metrics
```

### 问题 2: Grafana 无法连接 Prometheus

**症状**: Grafana 数据源测试失败

**解决方法**:

```bash
# 1. 检查 prometheus 服务是否运行
docker-compose ps prometheus

# 2. 从 Grafana 容器测试连接
docker exec -it new-api-grafana wget -O- http://prometheus:9090/api/v1/status/config

# 3. 检查 Grafana 日志
docker-compose logs grafana
```

### 问题 3: Dashboard 没有数据

**可能原因**:

1. **时间范围过大或过小**: 调整 Grafana 右上角的时间范围
2. **没有请求流量**: 发送一些测试请求到 New API
3. **变量没有选择**: 检查 Dashboard 顶部的筛选器是否有值

**验证步骤**:

```bash
# 1. 在 Prometheus UI 中测试查询
# 访问 http://localhost:9090
# 执行查询: new_api_model_requests_total

# 2. 生成测试请求
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"test"}]}'
```

### 问题 4: 容器启动失败

**解决方法**:

```bash
# 1. 查看详细日志
docker-compose logs [服务名]

# 2. 检查配置文件
docker-compose config

# 3. 重新构建和启动
docker-compose down
docker-compose up -d --build
```

## 📁 相关文件

### 核心配置文件

```
new-api/
├── docker-compose.yml                          # 主配置文件（已整合）
├── prometheus/
│   ├── prometheus.yml                          # Prometheus 配置
│   └── alerts/                                 # 告警规则
│       ├── channel_success_rate.yml
│       ├── model_success_rate.yml
│       ├── error_spike.yml
│       └── channel_status.yml
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/prometheus.yml          # 数据源自动配置
│   │   └── dashboards/dashboards.yml           # Dashboard 自动加载
│   └── dashboards/
│       ├── new-api-monitoring.json             # 完整 Dashboard JSON
│       └── README.md                           # Dashboard 使用说明
└── alertmanager/
    └── alertmanager.yml                        # AlertManager 配置
```

### 文档文件

```
docs/
├── PROMETHEUS_MONITORING_REQUIREMENTS.md       # 需求文档
├── PROMETHEUS_DEPLOYMENT_GUIDE.md              # 部署指南
├── PROMETHEUS_USER_MANUAL.md                   # 使用手册
├── PROMETHEUS_QUICKSTART.md                    # 快速开始
├── PROMETHEUS_IMPLEMENTATION_SUMMARY.md        # 实现总结
└── PROMETHEUS_DOCKER_COMPOSE_MERGED.md         # 本文档
```

## 🎯 下一步

1. ✅ 启动所有服务: `docker-compose up -d`
2. ✅ 验证服务状态: `docker-compose ps`
3. ✅ 访问 Grafana: http://localhost:3001
4. ✅ 导入或查看 Dashboard
5. ✅ 生成测试请求查看监控数据
6. ✅ 配置告警通知（可选）

## 💡 重要提示

### 与旧配置文件的关系

- ✅ `docker-compose.prometheus.yml` 文件可以删除或保留作为参考
- ✅ 所有功能已整合到主 `docker-compose.yml` 文件
- ✅ 使用 `docker-compose up -d` 即可启动所有服务

### 数据持久化

所有监控数据都持久化到 Docker volumes 中：

```bash
# 查看 volumes
docker volume ls | grep new-api

# 备份 Prometheus 数据
docker run --rm -v new-api_prometheus_data:/data -v $(pwd)/backup:/backup alpine tar czf /backup/prometheus-backup.tar.gz /data

# 恢复 Prometheus 数据
docker run --rm -v new-api_prometheus_data:/data -v $(pwd)/backup:/backup alpine tar xzf /backup/prometheus-backup.tar.gz -C /
```

## 📞 获取帮助

如果遇到问题：

1. 查看 [部署指南](PROMETHEUS_DEPLOYMENT_GUIDE.md)
2. 查看 [使用手册](PROMETHEUS_USER_MANUAL.md)
3. 查看 [快速开始指南](PROMETHEUS_QUICKSTART.md)
4. 提交 GitHub Issue

---

**文档创建**: 2025-12-03
**版本**: v1.0
**状态**: ✅ 整合完成，已验证
