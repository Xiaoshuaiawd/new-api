# 多站点部署指南

## 概述

此配置允许你在同一个 Prometheus + Grafana 环境中监控多个 New API 实例。

## 两种部署方案

### 方案 1: 单实例 + 自定义站点ID (已配置)

**适用场景**: 只有一个 New API 实例，但想给它一个有意义的标识

**配置文件**: `docker-compose.yml`

**已设置**: `SITE_ID=main`

**启动方式**:
```bash
docker-compose up -d
```

**访问地址**:
- New API: http://localhost:3000
- Grafana: http://localhost:3001
- Prometheus: http://localhost:9090

在 Grafana Dashboard 中，"站点"下拉框会显示 "main"。

---

### 方案 2: 多实例部署 (高级)

**适用场景**: 需要部署多个 New API 实例，统一监控

**配置文件**: `docker-compose.multi-site.yml`

**包含 3 个站点**:
- **main** (主站) - 端口 3000
- **backup** (备用站) - 端口 3001
- **test** (测试站) - 端口 3002

**启动方式**:
```bash
# 停止单实例部署 (如果正在运行)
docker-compose down

# 启动多站点部署
docker-compose -f docker-compose.multi-site.yml up -d
```

**访问地址**:
- New API 主站: http://localhost:3000
- New API 备用站: http://localhost:3001
- New API 测试站: http://localhost:3002
- Grafana: http://localhost:3100 (注意端口改为3100)
- Prometheus: http://localhost:9090

在 Grafana Dashboard 中，"站点"下拉框会显示 "main", "backup", "test"，可以选择查看不同站点的监控数据。

---

## 重要配置说明

### SESSION_SECRET 和 CRYPTO_SECRET

**⚠️ 多站点部署必须设置这两个环境变量，且所有站点必须相同！**

在 `docker-compose.multi-site.yml` 中修改:

```yaml
- SESSION_SECRET=your-random-secret-string-here  # 改为一个随机字符串
- CRYPTO_SECRET=your-crypto-secret-string-here   # 改为另一个随机字符串
```

生成随机字符串的方法:
```bash
# 方法 1: 使用 openssl
openssl rand -hex 32

# 方法 2: 使用 uuidgen
uuidgen | tr -d '-'

# 方法 3: 使用 /dev/urandom
cat /dev/urandom | head -c 32 | base64
```

### SITE_ID 的作用

- 用于在 Prometheus 指标中区分不同站点
- 在 Grafana 中可以通过下拉框选择查看特定站点
- 建议使用有意义的名称: `main`, `backup`, `test`, `us-west`, `eu-east` 等

---

## 修改站点数量

### 增加站点

在 `docker-compose.multi-site.yml` 中复制一个 `new-api-*` 服务块，修改:

1. 服务名: `new-api-site4`
2. 容器名: `container_name: new-api-site4`
3. 端口: `- "3003:3000"`
4. 数据卷: `./data/site4:/data` 和 `./logs/site4:/app/logs`
5. SITE_ID: `- SITE_ID=site4`

然后在 `prometheus/prometheus-multi-site.yml` 中添加抓取配置:

```yaml
- job_name: 'new-api-site4'
  static_configs:
    - targets: ['new-api-site4:3000']
      labels:
        app: 'new-api'
        site: 'site4'
        env: 'production'
  scrape_interval: 15s
  scrape_timeout: 10s
  metrics_path: '/metrics'
```

### 减少站点

注释掉或删除不需要的服务，同时在 Prometheus 配置中也删除对应的 job。

---

## 验证部署

### 1. 检查服务状态

```bash
docker-compose -f docker-compose.multi-site.yml ps
```

所有服务应该显示 "Up" 状态。

### 2. 检查 Prometheus Targets

访问 http://localhost:9090/targets

应该看到所有站点的 targets 状态为 "UP":
- new-api-main
- new-api-backup
- new-api-test

### 3. 检查 Grafana Dashboard

访问 http://localhost:3100 (多站点) 或 http://localhost:3001 (单站点)

登录: admin / admin123

在 Dashboard 中，"站点" 下拉框应该显示所有站点选项。

---

## 故障排查

### 问题: Grafana 站点下拉框只显示 "default"

**原因**: New API 没有设置 SITE_ID 环境变量，或 Prometheus 没有抓取到数据

**解决**:
1. 检查 `docker-compose.yml` 或 `docker-compose.multi-site.yml` 中是否设置了 `SITE_ID`
2. 重启 New API 服务: `docker-compose restart new-api`
3. 等待 1-2 分钟让 Prometheus 抓取数据
4. 刷新 Grafana 页面

### 问题: 站点下拉框是空的

**原因**: Prometheus 没有抓取到任何指标数据

**解决**:
1. 检查 Prometheus Targets: http://localhost:9090/targets
2. 确保所有 targets 状态为 "UP"
3. 检查 New API 日志: `docker-compose logs new-api`
4. 确认 `PROMETHEUS_ENABLED=true` 已设置

### 问题: 多站点部署后，会话/登录状态不同步

**原因**: 没有设置或各站点的 SESSION_SECRET 不一致

**解决**:
在所有站点的环境变量中添加相同的 SESSION_SECRET 和 CRYPTO_SECRET:
```yaml
- SESSION_SECRET=same-random-string-for-all-sites
- CRYPTO_SECRET=same-crypto-string-for-all-sites
```

---

## 推荐配置

### 单服务器部署
使用 `docker-compose.yml` (已配置 SITE_ID=main)

### 多服务器部署
- 每台服务器运行 `docker-compose.yml`，设置不同的 SITE_ID
- 所有服务器的指标发送到中央 Prometheus 服务器
- 所有服务器共享相同的 SESSION_SECRET 和 CRYPTO_SECRET

### 高可用部署
使用 `docker-compose.multi-site.yml` + Nginx/Traefik 负载均衡

---

## 下一步

1. 修改 SESSION_SECRET 和 CRYPTO_SECRET (生产环境必须)
2. 修改数据库密码 (POSTGRES_PASSWORD 和 SQL_DSN)
3. 修改 Grafana 密码 (GF_SECURITY_ADMIN_PASSWORD)
4. 根据需要调整站点数量
5. 配置告警通知 (AlertManager)

更多信息请参考: `docs/PROMETHEUS_DOCKER_COMPOSE_MERGED.md`
