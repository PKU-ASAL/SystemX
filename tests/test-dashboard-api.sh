#!/bin/bash

# Dashboard API 测试脚本
# 测试新实现的Dashboard API接口

BASE_URL="http://localhost:8080/api/v1/dashboard"

echo "🧪 Testing SysArmor Dashboard API"
echo "=================================="

# 测试告警严重程度分布
echo ""
echo "📊 Testing Alert Severity Distribution..."
echo "GET $BASE_URL/alerts/severity-distribution"
curl -s "$BASE_URL/alerts/severity-distribution?timeRange=24h" | jq '.' || echo "❌ Failed to get severity distribution"

echo ""
echo "📈 Testing Alert Trends..."
echo "GET $BASE_URL/alerts/trends"
curl -s "$BASE_URL/alerts/trends?timeRange=7d&interval=1h&groupBy=severity" | jq '.' || echo "❌ Failed to get alert trends"

echo ""
echo "📋 Testing Event Types Distribution..."
echo "GET $BASE_URL/alerts/event-types"
curl -s "$BASE_URL/alerts/event-types?timeRange=7d&limit=10" | jq '.' || echo "❌ Failed to get event types"

echo ""
echo "🖥️ Testing Collectors Overview..."
echo "GET $BASE_URL/collectors/overview"
curl -s "$BASE_URL/collectors/overview" | jq '.' || echo "❌ Failed to get collectors overview"

echo ""
echo "⚙️ Testing System Performance Overview..."
echo "GET $BASE_URL/system/performance"
curl -s "$BASE_URL/system/performance" | jq '.' || echo "❌ Failed to get system performance"

echo ""
echo "✅ Dashboard API testing completed!"
echo ""
echo "📖 Available Dashboard API endpoints:"
echo "   - GET /api/v1/dashboard/alerts/severity-distribution"
echo "   - GET /api/v1/dashboard/alerts/trends"
echo "   - GET /api/v1/dashboard/alerts/event-types"
echo "   - GET /api/v1/dashboard/collectors/overview"
echo "   - GET /api/v1/dashboard/system/performance"
