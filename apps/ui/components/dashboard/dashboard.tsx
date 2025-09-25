"use client";

import { useState, useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from "@/components/ui/chart";
import {
  RefreshCw,
  Server,
  Activity,
  Database,
  TrendingUp,
  AlertTriangle,
  CheckCircle,
  BarChart3,
  Shield,
  Zap,
  Eye,
  Cpu,
} from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  XAxis,
  YAxis,
  PieChart,
  Pie,
  Cell,
  Area,
  AreaChart,
} from "recharts";

// Dashboard API 接口定义
interface AlertSeverityDistribution {
  critical: number;
  high: number;
  medium: number;
  low: number;
  total: number;
  timeRange: string;
  updatedAt: string;
}

interface AlertTrend {
  timestamp: string;
  critical: number;
  high: number;
  medium: number;
  low: number;
  total: number;
}

interface AlertTrends {
  timeline: AlertTrend[];
  statistics: {
    total_alerts: number;
    avg_per_interval: number;
    interval: string;
    time_range: string;
  };
}

interface EventTypeDistribution {
  total: number;
  top_types: Array<{
    type: string;
    count: number;
    percentage: number;
  }>;
  [key: string]: any;
}

interface CollectorOverview {
  summary: {
    total: number;
    active: number;
    inactive: number;
    offline: number;
    error: number;
  };
  byDeploymentType: {
    "agentless": number;
    "sysarmor-stack": number;
    "wazuh-hybrid": number;
  };
  byEnvironment: Record<string, number>;
  performance: {
    healthyPercentage: number;
    avgResponseTime: number;
    recentlyActive: number;
  };
  recentChanges: {
    newCollectors24h: number;
    offlineCollectors24h: number;
  };
}

interface SystemPerformance {
  kafka: {
    cluster_status: string;
    brokers: number;
    topics: number;
    partitions: number;
    messages_per_sec: number;
    disk_usage: number;
  };
  opensearch: {
    cluster_status: string;
    indices_count: number;
    docs_count: number;
    storage_size: string;
    query_performance: {
      avg_response_time: number;
      queries_per_sec: number;
      slow_queries: number;
    };
  };
  flink: {
    cluster_status: string;
    taskmanagers: number;
    jobs_running: number;
    slots_total: number;
    slots_used: number;
    memory_usage: number;
    processing_rate: number;
  };
  database: {
    status: string;
    connections: number;
    response_time: number;
    query_performance: string;
  };
}

export function ChartsDashboard() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  
  // Dashboard 数据状态
  const [alertSeverity, setAlertSeverity] = useState<AlertSeverityDistribution | null>(null);
  const [alertTrends, setAlertTrends] = useState<AlertTrends | null>(null);
  const [eventTypes, setEventTypes] = useState<EventTypeDistribution | null>(null);
  const [collectorsOverview, setCollectorsOverview] = useState<CollectorOverview | null>(null);
  const [systemPerformance, setSystemPerformance] = useState<SystemPerformance | null>(null);

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      setError(null);

      // 并行获取所有 Dashboard API 数据
      const [
        severityResponse,
        trendsResponse,
        eventTypesResponse,
        collectorsResponse,
        performanceResponse,
      ] = await Promise.allSettled([
        fetch('/api/v1/dashboard/alerts/severity-distribution').then(r => r.json()),
        fetch('/api/v1/dashboard/alerts/trends').then(r => r.json()),
        fetch('/api/v1/dashboard/alerts/event-types').then(r => r.json()),
        fetch('/api/v1/dashboard/collectors/overview').then(r => r.json()),
        fetch('/api/v1/dashboard/system/performance').then(r => r.json()),
      ]);

      // 处理响应数据
      if (severityResponse.status === 'fulfilled' && severityResponse.value.success) {
        setAlertSeverity(severityResponse.value.data);
      }

      if (trendsResponse.status === 'fulfilled' && trendsResponse.value.success) {
        setAlertTrends(trendsResponse.value.data);
      }

      if (eventTypesResponse.status === 'fulfilled' && eventTypesResponse.value.success) {
        setEventTypes(eventTypesResponse.value.data);
      }

      if (collectorsResponse.status === 'fulfilled' && collectorsResponse.value.success) {
        setCollectorsOverview(collectorsResponse.value.data);
      }

      if (performanceResponse.status === 'fulfilled' && performanceResponse.value.success) {
        setSystemPerformance(performanceResponse.value.data);
      }

      setLastUpdated(new Date());
    } catch (error) {
      console.error("Failed to fetch dashboard data:", error);
      setError("获取 Dashboard 数据失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDashboardData();
    // 每30秒自动刷新
    const interval = setInterval(fetchDashboardData, 30000);
    return () => clearInterval(interval);
  }, []);

  // 图表配置
  const alertSeverityConfig = {
    critical: {
      label: "严重",
      color: "hsl(var(--chart-1))",
    },
    high: {
      label: "高危",
      color: "hsl(var(--chart-2))",
    },
    medium: {
      label: "中等",
      color: "hsl(var(--chart-3))",
    },
    low: {
      label: "低危",
      color: "hsl(var(--chart-4))",
    },
  } satisfies ChartConfig;

  const alertTrendsConfig = {
    total: {
      label: "总告警",
      color: "hsl(var(--chart-1))",
    },
  } satisfies ChartConfig;

  const eventTypesConfig = {
    count: {
      label: "事件数量",
      color: "hsl(var(--chart-1))",
    },
  } satisfies ChartConfig;

  const performanceConfig = {
    memory_usage: {
      label: "内存使用率",
      color: "hsl(var(--chart-1))",
    },
    disk_usage: {
      label: "磁盘使用率",
      color: "hsl(var(--chart-2))",
    },
  } satisfies ChartConfig;

  const getStatusColor = (status: string) => {
    const colors = {
      healthy: "text-green-600 bg-green-100",
      green: "text-green-600 bg-green-100",
      online: "text-green-600 bg-green-100",
      connected: "text-green-600 bg-green-100",
      good: "text-green-600 bg-green-100",
      yellow: "text-yellow-600 bg-yellow-100",
      red: "text-red-600 bg-red-100",
      offline: "text-red-600 bg-red-100",
      unhealthy: "text-red-600 bg-red-100",
    };
    return colors[status as keyof typeof colors] || "text-gray-600 bg-gray-100";
  };

  // 准备图表数据
  const alertSeverityData = alertSeverity ? [
    { name: "严重", value: alertSeverity.critical, fill: "var(--color-critical)" },
    { name: "高危", value: alertSeverity.high, fill: "var(--color-high)" },
    { name: "中等", value: alertSeverity.medium, fill: "var(--color-medium)" },
    { name: "低危", value: alertSeverity.low, fill: "var(--color-low)" },
  ].filter(item => item.value > 0) : [];

  const alertTrendsData = alertTrends?.timeline.map(trend => ({
    time: new Date(trend.timestamp).toLocaleTimeString('zh-CN', { 
      hour: '2-digit', 
      minute: '2-digit' 
    }),
    total: trend.total,
  })) || [];

  const eventTypesData = eventTypes?.top_types.slice(0, 5).map(type => ({
    type: type.type,
    count: type.count,
    fill: `var(--color-count)`,
  })) || [];

  const performanceData = systemPerformance ? [
    {
      metric: "Flink内存",
      value: systemPerformance.flink.memory_usage,
      fill: "var(--color-memory_usage)",
    },
    {
      metric: "Kafka磁盘",
      value: systemPerformance.kafka.disk_usage,
      fill: "var(--color-disk_usage)",
    },
  ] : [];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin mr-2" />
        <span>加载 Dashboard 数据...</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-auto p-4 lg:p-6">
        {/* 页面标题和刷新按钮 */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold">SysArmor 安全监控中心</h1>
            <p className="text-gray-600">
              实时监控系统安全状态和威胁情报
              {lastUpdated && (
                <span className="ml-2 text-sm">
                  最后更新: {lastUpdated.toLocaleTimeString()}
                </span>
              )}
            </p>
          </div>
          <Button onClick={fetchDashboardData} variant="outline" size="sm">
            <RefreshCw className="h-4 w-4 mr-2" />
            刷新
          </Button>
        </div>

        {error && (
          <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg dark:bg-red-950 dark:border-red-800">
            <div className="flex items-center">
              <AlertTriangle className="h-5 w-5 text-red-600 mr-2" />
              <span className="text-red-800 dark:text-red-200">{error}</span>
            </div>
          </div>
        )}

        {/* 告警概览卡片 */}
        <div className="grid grid-cols-1 gap-4 mb-8 md:grid-cols-2 lg:grid-cols-4">
          {/* 总告警数 */}
          <Card>
            <CardHeader>
              <CardDescription>总告警数</CardDescription>
              <CardTitle className="text-3xl font-bold text-red-600">
                {alertSeverity?.total || 0}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-red-100">
                  <Shield className="h-5 w-5 text-red-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-gray-600">
              过去 {alertSeverity?.timeRange || '24h'} 内的告警
            </CardFooter>
          </Card>

          {/* 严重告警 */}
          <Card>
            <CardHeader>
              <CardDescription>严重告警</CardDescription>
              <CardTitle className="text-3xl font-bold text-red-600">
                {(alertSeverity?.critical || 0) + (alertSeverity?.high || 0)}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-red-100">
                  <AlertTriangle className="h-5 w-5 text-red-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-gray-600">
              需要立即处理的告警
            </CardFooter>
          </Card>

          {/* 活跃终端 */}
          <Card>
            <CardHeader>
              <CardDescription>活跃终端</CardDescription>
              <CardTitle className="text-3xl font-bold text-green-600">
                {collectorsOverview?.summary.active || 0}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  <Activity className="h-5 w-5 text-green-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-gray-600">
              总计 {collectorsOverview?.summary.total || 0} 个终端
            </CardFooter>
          </Card>

          {/* 系统健康度 */}
          <Card>
            <CardHeader>
              <CardDescription>系统健康度</CardDescription>
              <CardTitle className="text-3xl font-bold text-green-600">
                {collectorsOverview?.performance.healthyPercentage || 0}%
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  <CheckCircle className="h-5 w-5 text-green-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-gray-600">
              所有服务综合健康状态
            </CardFooter>
          </Card>
        </div>

        {/* 图表区域 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
          {/* 告警严重程度分布 - 饼图 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <BarChart3 className="h-5 w-5" />
                告警严重程度分布
              </CardTitle>
              <CardDescription>按严重程度分类的告警统计</CardDescription>
            </CardHeader>
            <CardContent>
              {alertSeverityData.length > 0 ? (
                <ChartContainer
                  config={alertSeverityConfig}
                  className="mx-auto aspect-square max-h-[300px]"
                >
                  <PieChart>
                    <ChartTooltip
                      cursor={false}
                      content={<ChartTooltipContent hideLabel />}
                    />
                    <Pie
                      data={alertSeverityData}
                      dataKey="value"
                      nameKey="name"
                      innerRadius={60}
                      strokeWidth={5}
                    >
                      {alertSeverityData.map((entry, index) => (
                        <Cell key={`cell-${index}`} fill={entry.fill} />
                      ))}
                    </Pie>
                    <ChartLegend
                      content={<ChartLegendContent nameKey="name" />}
                      className="-translate-y-2 flex-wrap gap-2 [&>*]:basis-1/4 [&>*]:justify-center"
                    />
                  </PieChart>
                </ChartContainer>
              ) : (
                <div className="flex items-center justify-center h-[300px] text-gray-500">
                  暂无告警数据
                </div>
              )}
            </CardContent>
          </Card>

          {/* 告警趋势 - 面积图 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                告警趋势
              </CardTitle>
              <CardDescription>
                过去 {alertTrends?.statistics.time_range || '7天'} 的告警变化趋势
              </CardDescription>
            </CardHeader>
            <CardContent>
              {alertTrendsData.length > 0 ? (
                <ChartContainer config={alertTrendsConfig} className="h-[300px]">
                  <AreaChart
                    accessibilityLayer
                    data={alertTrendsData}
                    margin={{
                      left: 12,
                      right: 12,
                    }}
                  >
                    <CartesianGrid vertical={false} />
                    <XAxis
                      dataKey="time"
                      tickLine={false}
                      axisLine={false}
                      tickMargin={8}
                    />
                    <ChartTooltip
                      cursor={false}
                      content={<ChartTooltipContent />}
                    />
                    <defs>
                      <linearGradient id="fillTotal" x1="0" y1="0" x2="0" y2="1">
                        <stop
                          offset="5%"
                          stopColor="var(--color-total)"
                          stopOpacity={0.8}
                        />
                        <stop
                          offset="95%"
                          stopColor="var(--color-total)"
                          stopOpacity={0.1}
                        />
                      </linearGradient>
                    </defs>
                    <Area
                      dataKey="total"
                      type="natural"
                      fill="url(#fillTotal)"
                      fillOpacity={0.4}
                      stroke="var(--color-total)"
                      stackId="a"
                    />
                  </AreaChart>
                </ChartContainer>
              ) : (
                <div className="flex items-center justify-center h-[300px] text-gray-500">
                  暂无趋势数据
                </div>
              )}
            </CardContent>
          </Card>

          {/* 事件类型分布 - 条形图 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Eye className="h-5 w-5" />
                事件类型分布
              </CardTitle>
              <CardDescription>最常见的安全事件类型</CardDescription>
            </CardHeader>
            <CardContent>
              {eventTypesData.length > 0 ? (
                <ChartContainer config={eventTypesConfig} className="h-[300px]">
                  <BarChart
                    accessibilityLayer
                    data={eventTypesData}
                    layout="horizontal"
                    margin={{
                      left: 80,
                    }}
                  >
                    <CartesianGrid horizontal={false} />
                    <YAxis
                      dataKey="type"
                      type="category"
                      tickLine={false}
                      tickMargin={10}
                      axisLine={false}
                      width={70}
                    />
                    <XAxis dataKey="count" type="number" hide />
                    <ChartTooltip
                      cursor={false}
                      content={<ChartTooltipContent hideLabel />}
                    />
                    <Bar dataKey="count" fill="var(--color-count)" radius={4} />
                  </BarChart>
                </ChartContainer>
              ) : (
                <div className="flex items-center justify-center h-[300px] text-gray-500">
                  暂无事件数据
                </div>
              )}
            </CardContent>
          </Card>

          {/* 系统性能指标 - 条形图 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Cpu className="h-5 w-5" />
                系统性能指标
              </CardTitle>
              <CardDescription>关键系统资源使用情况</CardDescription>
            </CardHeader>
            <CardContent>
              {performanceData.length > 0 ? (
                <ChartContainer config={performanceConfig} className="h-[300px]">
                  <BarChart
                    accessibilityLayer
                    data={performanceData}
                    margin={{
                      top: 20,
                    }}
                  >
                    <CartesianGrid vertical={false} />
                    <XAxis
                      dataKey="metric"
                      tickLine={false}
                      tickMargin={10}
                      axisLine={false}
                    />
                    <ChartTooltip
                      cursor={false}
                      content={<ChartTooltipContent hideLabel />}
                    />
                    <Bar dataKey="value" radius={8} />
                  </BarChart>
                </ChartContainer>
              ) : (
                <div className="flex items-center justify-center h-[300px] text-gray-500">
                  暂无性能数据
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* 系统状态概览 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* 基础设施状态 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Server className="h-5 w-5" />
                基础设施状态
              </CardTitle>
              <CardDescription>核心服务运行状态</CardDescription>
            </CardHeader>
            <CardContent>
              {systemPerformance ? (
                <div className="space-y-4">
                  {/* Kafka */}
                  <div className="flex items-center justify-between p-3 border rounded-lg">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-full ${getStatusColor(systemPerformance.kafka.cluster_status)}`}>
                        <Database className="h-4 w-4" />
                      </div>
                      <div>
                        <p className="font-medium">Kafka 集群</p>
                        <p className="text-sm text-gray-500">
                          {systemPerformance.kafka.brokers} 个 Broker, {systemPerformance.kafka.topics} 个 Topic
                        </p>
                      </div>
                    </div>
                    <Badge variant={systemPerformance.kafka.cluster_status === 'online' ? 'default' : 'destructive'}>
                      {systemPerformance.kafka.cluster_status}
                    </Badge>
                  </div>

                  {/* OpenSearch */}
                  <div className="flex items-center justify-between p-3 border rounded-lg">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-full ${getStatusColor(systemPerformance.opensearch.cluster_status)}`}>
                        <Database className="h-4 w-4" />
                      </div>
                      <div>
                        <p className="font-medium">OpenSearch 集群</p>
                        <p className="text-sm text-gray-500">
                          {systemPerformance.opensearch.docs_count} 个文档, {systemPerformance.opensearch.storage_size}
                        </p>
                      </div>
                    </div>
                    <Badge variant={systemPerformance.opensearch.cluster_status === 'green' ? 'default' : 'destructive'}>
                      {systemPerformance.opensearch.cluster_status}
                    </Badge>
                  </div>

                  {/* Flink */}
                  <div className="flex items-center justify-between p-3 border rounded-lg">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-full ${getStatusColor(systemPerformance.flink.cluster_status)}`}>
                        <Zap className="h-4 w-4" />
                      </div>
                      <div>
                        <p className="font-medium">Flink 集群</p>
                        <p className="text-sm text-gray-500">
                          {systemPerformance.flink.jobs_running} 个作业运行中
                        </p>
                      </div>
                    </div>
                    <Badge variant={systemPerformance.flink.cluster_status === 'healthy' ? 'default' : 'destructive'}>
                      {systemPerformance.flink.cluster_status}
                    </Badge>
                  </div>
                </div>
              ) : (
                <div className="text-center py-8 text-gray-500">加载系统状态...</div>
              )}
            </CardContent>
          </Card>

          {/* 性能指标详情 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                性能指标详情
              </CardTitle>
              <CardDescription>系统关键性能数据</CardDescription>
            </CardHeader>
            <CardContent>
              {systemPerformance ? (
                <div className="space-y-6">
                  {/* Flink 内存使用率 */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium">Flink 内存使用率</span>
                      <span className="text-sm text-gray-500">{systemPerformance.flink.memory_usage}%</span>
                    </div>
                    <Progress value={systemPerformance.flink.memory_usage} className="h-2" />
                  </div>

                  {/* Kafka 磁盘使用率 */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium">Kafka 磁盘使用率</span>
                      <span className="text-sm text-gray-500">{systemPerformance.kafka.disk_usage}%</span>
                    </div>
                    <Progress value={systemPerformance.kafka.disk_usage} className="h-2" />
                  </div>

                  {/* 处理速率 */}
                  <div className="grid grid-cols-2 gap-4">
                    <div className="text-center p-3 bg-blue-50 rounded-lg dark:bg-blue-950/20">
                      <div className="text-2xl font-bold text-blue-600 dark:text-blue-400">
                        {systemPerformance.kafka.messages_per_sec.toFixed(1)}
                      </div>
                      <div className="text-sm text-gray-600 dark:text-gray-400">消息/秒</div>
                    </div>
                    <div className="text-center p-3 bg-green-50 rounded-lg dark:bg-green-950/20">
                      <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                        {systemPerformance.flink.processing_rate.toFixed(1)}
                      </div>
                      <div className="text-sm text-gray-600 dark:text-gray-400">处理速率</div>
                    </div>
                  </div>

                  {/* 查询性能 */}
                  <div className="grid grid-cols-2 gap-4">
                    <div className="text-center p-3 bg-purple-50 rounded-lg dark:bg-purple-950/20">
                      <div className="text-2xl font-bold text-purple-600 dark:text-purple-400">
                        {systemPerformance.opensearch.query_performance.avg_response_time}ms
                      </div>
                      <div className="text-sm text-gray-600 dark:text-gray-400">平均响应时间</div>
                    </div>
                    <div className="text-center p-3 bg-orange-50 rounded-lg dark:bg-orange-950/20">
                      <div className="text-2xl font-bold text-orange-600 dark:text-orange-400">
                        {systemPerformance.opensearch.query_performance.queries_per_sec.toFixed(1)}
                      </div>
                      <div className="text-sm text-gray-600 dark:text-gray-400">查询/秒</div>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="text-center py-8 text-gray-500">加载性能数据...</div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
