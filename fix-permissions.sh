#!/bin/bash

# 一键修复 Prometheus 监控服务权限问题脚本
#
# 使用方法:
#   chmod +x fix-permissions.sh
#   ./fix-permissions.sh

set -e

echo "=========================================="
echo "开始修复 Prometheus 监控服务权限问题"
echo "=========================================="
echo ""

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "当前目录: $SCRIPT_DIR"
echo ""

# 1. 修复 Prometheus 配置文件权限
echo "1️⃣  修复 Prometheus 配置文件权限..."
if [ -d "prometheus" ]; then
    # Prometheus 容器使用 nobody 用户 (uid: 65534)
    sudo chown -R 65534:65534 prometheus/
    sudo chmod -R 755 prometheus/
    sudo chmod 644 prometheus/prometheus.yml
    sudo chmod 644 prometheus/alerts/*.yml 2>/dev/null || true
    echo "   ✅ Prometheus 配置文件权限已修复"
else
    echo "   ⚠️  prometheus/ 目录不存在，跳过"
fi
echo ""

# 2. 修复 Grafana 配置文件权限
echo "2️⃣  修复 Grafana 配置文件权限..."
if [ -d "grafana" ]; then
    # Grafana 容器使用 grafana 用户 (uid: 472)
    sudo chown -R 472:472 grafana/
    sudo chmod -R 755 grafana/
    sudo chmod 644 grafana/provisioning/datasources/*.yml 2>/dev/null || true
    sudo chmod 644 grafana/provisioning/dashboards/*.yml 2>/dev/null || true
    sudo chmod 644 grafana/dashboards/*.json 2>/dev/null || true
    echo "   ✅ Grafana 配置文件权限已修复"
else
    echo "   ⚠️  grafana/ 目录不存在，跳过"
fi
echo ""

# 3. 修复 AlertManager 配置文件权限
echo "3️⃣  修复 AlertManager 配置文件权限..."
if [ -d "alertmanager" ]; then
    # AlertManager 容器使用 nobody 用户 (uid: 65534)
    sudo chown -R 65534:65534 alertmanager/
    sudo chmod -R 755 alertmanager/
    sudo chmod 644 alertmanager/alertmanager.yml
    echo "   ✅ AlertManager 配置文件权限已修复"
else
    echo "   ⚠️  alertmanager/ 目录不存在，跳过"
fi
echo ""

# 4. 确保数据目录存在（如果使用本地挂载）
echo "4️⃣  检查数据目录..."
if [ -d "data" ]; then
    sudo chmod -R 755 data/
    echo "   ✅ data/ 目录权限已修复"
fi
if [ -d "logs" ]; then
    sudo chmod -R 755 logs/
    echo "   ✅ logs/ 目录权限已修复"
fi
echo ""

# 5. 显示当前权限状态
echo "5️⃣  当前权限状态:"
echo ""
echo "📁 Prometheus:"
ls -la prometheus/ 2>/dev/null | head -5 || echo "   目录不存在"
echo ""
echo "📁 Grafana:"
ls -la grafana/ 2>/dev/null | head -5 || echo "   目录不存在"
echo ""
echo "📁 AlertManager:"
ls -la alertmanager/ 2>/dev/null | head -5 || echo "   目录不存在"
echo ""

# 6. 重启服务
echo "6️⃣  是否重启 Docker 服务? (y/n)"
read -r RESTART_SERVICES

if [ "$RESTART_SERVICES" = "y" ] || [ "$RESTART_SERVICES" = "Y" ]; then
    echo ""
    echo "正在重启监控服务..."
    echo ""

    # 停止服务
    echo "停止服务..."
    docker-compose stop prometheus grafana alertmanager 2>/dev/null || true

    # 删除旧容器（避免权限缓存问题）
    echo "删除旧容器..."
    docker-compose rm -f prometheus grafana alertmanager 2>/dev/null || true

    # 启动服务
    echo "启动服务..."
    docker-compose up -d prometheus grafana alertmanager

    echo ""
    echo "⏳ 等待服务启动..."
    sleep 5

    echo ""
    echo "📊 服务状态:"
    docker-compose ps prometheus grafana alertmanager

    echo ""
    echo "📝 查看日志 (Ctrl+C 退出):"
    echo "   docker-compose logs -f prometheus"
    echo "   docker-compose logs -f grafana"
    echo "   docker-compose logs -f alertmanager"
fi

echo ""
echo "=========================================="
echo "✅ 权限修复完成！"
echo "=========================================="
echo ""
echo "🔍 验证方法:"
echo ""
echo "1. 检查 Prometheus:"
echo "   curl http://localhost:9090/-/healthy"
echo "   浏览器访问: http://localhost:9090"
echo ""
echo "2. 检查 Grafana:"
echo "   curl http://localhost:3001/api/health"
echo "   浏览器访问: http://localhost:3001"
echo "   默认账号: admin / admin123"
echo ""
echo "3. 检查 AlertManager:"
echo "   curl http://localhost:9093/-/healthy"
echo "   浏览器访问: http://localhost:9093"
echo ""
echo "4. 检查服务日志:"
echo "   docker-compose logs prometheus"
echo "   docker-compose logs grafana"
echo "   docker-compose logs alertmanager"
echo ""
echo "如果仍有问题，请查看文档:"
echo "   docs/PROMETHEUS_DOCKER_COMPOSE_MERGED.md"
echo ""
