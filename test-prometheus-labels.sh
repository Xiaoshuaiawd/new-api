#!/bin/bash

echo "=========================================="
echo "Prometheus 站点标签测试"
echo "=========================================="
echo ""

echo "1️⃣  检查 Prometheus 配置..."
if [ -f "prometheus/prometheus.yml" ]; then
    echo "✅ prometheus.yml 存在"
    echo ""
    echo "配置的 job 和 site labels:"
    grep -A 10 "job_name:" prometheus/prometheus.yml | grep -E "(job_name|site:)"
else
    echo "❌ prometheus.yml 不存在"
fi

echo ""
echo "=========================================="
echo "2️⃣  测试 Prometheus API..."
echo ""

# 等待 Prometheus 启动
sleep 2

# 查询所有 site label 的值
echo "查询所有可用的 site 标签值:"
curl -s "http://localhost:9090/api/v1/label/site/values" | python3 -m json.tool

echo ""
echo "=========================================="
echo "3️⃣  查询带 site label 的指标:"
echo ""

# 查询 new_api_model_requests_total 指标的 site label
curl -s "http://localhost:9090/api/v1/query?query=new_api_model_requests_total" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    if data['status'] == 'success' and data['data']['result']:
        print('✅ 找到指标数据')
        print('')
        print('可用的 site 值:')
        sites = set()
        for item in data['data']['result']:
            if 'site' in item['metric']:
                sites.add(item['metric']['site'])
        for site in sorted(sites):
            print(f'  - {site}')
        if not sites:
            print('  ⚠️  没有找到 site label！')
            print('')
            print('示例指标 labels:')
            if data['data']['result']:
                print(json.dumps(data['data']['result'][0]['metric'], indent=2))
    else:
        print('❌ 没有数据')
        print(json.dumps(data, indent=2))
except Exception as e:
    print(f'❌ 解析错误: {e}')
    sys.exit(1)
"

echo ""
echo "=========================================="
echo "4️⃣  检查 Prometheus Targets 状态:"
echo ""

curl -s "http://localhost:9090/api/v1/targets" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    if data['status'] == 'success':
        active = data['data']['activeTargets']
        print(f'活跃 Targets 数量: {len(active)}')
        print('')
        for target in active:
            job = target['labels']['job']
            instance = target['labels']['instance']
            health = target['health']
            site = target['labels'].get('site', 'N/A')

            status_icon = '✅' if health == 'up' else '❌'
            print(f'{status_icon} {job:20s} | instance: {instance:20s} | site: {site:10s} | health: {health}')

            if health != 'up':
                print(f'    错误: {target.get(\"lastError\", \"Unknown\")}')
    else:
        print('❌ 获取 targets 失败')
except Exception as e:
    print(f'❌ 解析错误: {e}')
    sys.exit(1)
"

echo ""
echo "=========================================="
echo "5️⃣  测试 Grafana 变量查询:"
echo ""

# 模拟 Grafana 的查询
echo "查询: label_values(new_api_model_requests_total, site)"
curl -s "http://localhost:9090/api/v1/label/site/values" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    if data['status'] == 'success':
        values = data['data']
        if values:
            print('✅ Grafana 应该能看到以下站点选项:')
            for value in values:
                print(f'  - {value}')
        else:
            print('❌ 没有找到任何 site 值！')
            print('')
            print('可能的原因:')
            print('  1. Prometheus 还没有抓取到数据（等待 15-30 秒）')
            print('  2. New API 没有生成任何指标（发送测试请求）')
            print('  3. Prometheus 配置中没有设置 site label')
    else:
        print('❌ 查询失败')
        print(json.dumps(data, indent=2))
except Exception as e:
    print(f'❌ 解析错误: {e}')
    sys.exit(1)
"

echo ""
echo "=========================================="
echo "✅ 测试完成"
echo "=========================================="
echo ""
echo "如果看到问题，请运行以下命令:"
echo "  docker-compose restart prometheus"
echo "  等待 30 秒后重新运行此脚本"
echo ""
