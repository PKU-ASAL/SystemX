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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
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
  Shield,
  Zap,
  Cpu,
  HardDrive,
  Network,
} from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  XAxis,
  Area,
  AreaChart,
} from "recharts";
import { TimelineChartShadcn } from "@/components/opensearch/timeline-chart-shadcn";
import { PerformanceMetrics } from "./performance-metrics";

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

interface CollectorOverview {
  summary: {
    total: number;
    active: number;
    inactive: number;
    offline: number;
    error: number;
  };
  performance: {
    healthyPercentage: number;
    avgResponseTime: number;
    recentlyActive: number;
  };
}

interface SystemPerformance {
  kafka: {
    cluster_status: string;
    brokers: number;
    topics: number;
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
    jobs_running: number;
    memory_usage: number;
    processing_rate: number;
  };
}

// 最近告警终端接口
interface RecentAlertHost {
  hostname: string;
  ip_address: string;
  alert_count: number;
  last_alert_time: string;
  severity: string;
}

export function ChartsDashboard() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [activeTimeRange, setActiveTimeRange] = useState("7d");
  
  // Dashboard 数据状态
  const [alertSeverity, setAlertSeverity] = useState<AlertSeverityDistribution | null>(null);
  const [alertTrends, setAlertTrends] = useState<AlertTrends | null>(null);
  const [collectorsOverview, setCollectorsOverview] = useState<CollectorOverview | null>(null);
  const [systemPerformance, setSystemPerformance] = useState<SystemPerformance | null>(null);
  const [recentAlertHosts, setRecentAlertHosts] = useState<RecentAlertHost[]>([]);

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      setError(null);

      // 并行获取所有 Dashboard API 数据
      const [
        severityResponse,
        trendsResponse,
        collectorsResponse,
        performanceResponse,
      ] = await Promise.allSettled([
        fetch('/api/v1/dashboard/alerts/severity-distribution').then(r => r.json()),
        fetch('/api/v1/dashboard/alerts/trends').then(r => r.json()),
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

      if (collectorsResponse.status === 'fulfilled' && collectorsResponse.value.success) {
        setCollectorsOverview(collectorsResponse.value.data);
      }

      if (performanceResponse.status === 'fulfilled' && performanceResponse.value.success) {
        setSystemPerformance(performanceResponse.value.data);
      }

      // 模拟最近告警终端数据
      setRecentAlertHosts([
        { hostname: "web-server-01", ip_address: "192.168.1.10", alert_count: 5, last_alert_time: "2025-09-25T18:30:00Z", severity: "high" },
        { hostname: "db-server-02", ip_address: "192.168.1.20", alert_count: 3, last_alert_time: "2025-09-25T18:25:00Z", severity: "medium" },
        { hostname: "app-server-03", ip_address: "192.168.1.30", alert_count: 2, last_alert_time: "2025-09-25T18:20:00Z", severity: "low" },
      ]);

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

  // 图表配置 - 和 alerts 页面一致
  const timelineConfig = {
    critical: {
      label: "严重",
      color: "#ef4444", // red-500
    },
    high: {
      label: "高危", 
      color: "#f97316", // orange-500
    },
    medium: {
      label: "中等",
      color: "#eab308", // yellow-500
    },
    low: {
      label: "低危",
      color: "#22c55e", // green-500
    },
  } satisfies ChartConfig;

  // 生成不同时间范围的模拟数据
  const generateTimelineData = (range: string) => {
    const now = new Date();
    const data = [];
    let days = 7;
    
    switch (range) {
      case "7d":
        days = 7;
        break;
      case "30d":
        days = 30;
        break;
      case "3m":
        days = 90;
        break;
    }
    
    for (let i = days - 1; i >= 0; i--) {
      const date = new Date(now);
      date.setDate(date.getDate() - i);
      
      // 模拟数据，基于实际告警数据的模式
      const baseCount = Math.floor(Math.random() * 5) + 1;
      const critical = Math.floor(Math.random() * 2);
      const high = Math.floor(Math.random() * 3);
      const medium = Math.floor(Math.random() * baseCount);
      const low = Math.max(0, baseCount - critical - high - medium);
      
      data.push({
        time: date.toLocaleDateString('zh-CN', { 
          month: '2-digit', 
          day: '2-digit' 
        }),
        count: baseCount,
        critical,
        high,
        medium,
        low,
      });
    }
    
    return data;
  };

  const getSeverityBadge = (severity: string) => {
    const variants = {
      high: "destructive",
      medium: "default", 
      low: "secondary",
    } as const;
    return <Badge variant={variants[severity as keyof typeof variants] || "outline"}>{severity}</Badge>;
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin mr-2 text-primary" />
        <span className="text-muted-foreground">加载 Dashboard 数据...</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-auto p-4 lg:p-6">
        {/* 页面标题和刷新按钮 */}
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold">SysArmor 安全监控中心</h1>
            <p className="text-muted-foreground">
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
          <div className="mb-4 p-4 bg-destructive/10 border border-destructive/20 rounded-lg">
            <div className="flex items-center">
              <AlertTriangle className="h-5 w-5 text-destructive mr-2" />
              <span className="text-destructive">{error}</span>
            </div>
          </div>
        )}

        {/* 概览卡片 */}
        <div className="grid grid-cols-1 gap-4 mb-4 md:grid-cols-2 lg:grid-cols-4">
          {/* 总告警数 */}
          <Card>
            <CardHeader>
              <CardDescription>总告警数</CardDescription>
              <CardTitle className="text-3xl font-bold text-destructive">
                {alertSeverity?.total || 0}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-destructive/10">
                  <Shield className="h-5 w-5 text-destructive" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-muted-foreground">
              过去 {alertSeverity?.timeRange || '24h'} 内的告警
            </CardFooter>
          </Card>

          {/* 严重告警 */}
          <Card>
            <CardHeader>
              <CardDescription>严重告警</CardDescription>
              <CardTitle className="text-3xl font-bold text-destructive">
                {(alertSeverity?.critical || 0) + (alertSeverity?.high || 0)}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-destructive/10">
                  <AlertTriangle className="h-5 w-5 text-destructive" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-muted-foreground">
              需要立即处理的告警
            </CardFooter>
          </Card>

          {/* 活跃终端 */}
          <Card>
            <CardHeader>
              <CardDescription>活跃终端</CardDescription>
              <CardTitle className="text-3xl font-bold text-primary">
                {collectorsOverview?.summary.active || 0}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-primary/10">
                  <Activity className="h-5 w-5 text-primary" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-sm text-muted-foreground">
              总计 {collectorsOverview?.summary.total || 0} 个终端
            </CardFooter>
          </Card>

          {/* 系统健康度 - 合并基础设施状态 */}
          <Card>
            <CardHeader>
              <CardDescription>系统健康度</CardDescription>
              <CardTitle className="text-3xl font-bold text-primary">
                {(() => {
                  // 计算系统健康度：如果所有服务都健康则为100%
                  const kafkaHealthy = systemPerformance?.kafka.cluster_status === 'online';
                  const flinkHealthy = systemPerformance?.flink.cluster_status === 'healthy';
                  const opensearchHealthy = systemPerformance?.opensearch.cluster_status === 'green';
                  
                  const healthyServices = [kafkaHealthy, flinkHealthy, opensearchHealthy].filter(Boolean).length;
                  const totalServices = 3;
                  
                  return Math.round((healthyServices / totalServices) * 100);
                })()}%
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-primary/10">
                  <CheckCircle className="h-5 w-5 text-primary" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="text-xs text-muted-foreground space-y-1">
              <div className="flex items-center gap-2">
                <Database className="h-3 w-3" />
                <span>Kafka: {systemPerformance?.kafka.cluster_status || 'unknown'}</span>
              </div>
              <div className="flex items-center gap-2">
                <Zap className="h-3 w-3" />
                <span>Flink: {systemPerformance?.flink.cluster_status || 'unknown'}</span>
              </div>
              <div className="flex items-center gap-2">
                <Database className="h-3 w-3" />
                <span>OpenSearch: {systemPerformance?.opensearch.cluster_status || 'unknown'}</span>
              </div>
            </CardFooter>
          </Card>
        </div>

        {/* Timeline 图表 - 复用 alerts 页面组件 */}
        <Card className="mb-4">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>告警趋势</CardTitle>
                <CardDescription>不同时间范围的告警变化趋势</CardDescription>
              </div>
              <Tabs value={activeTimeRange} onValueChange={setActiveTimeRange} className="w-auto">
                <TabsList className="grid w-full grid-cols-3">
                  <TabsTrigger value="7d" className="text-xs">最近7天</TabsTrigger>
                  <TabsTrigger value="30d" className="text-xs">最近30天</TabsTrigger>
                  <TabsTrigger value="3m" className="text-xs">最近3个月</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
          </CardHeader>
          <CardContent>
            <TimelineChartShadcn 
              data={generateTimelineData(activeTimeRange)} 
              totalEvents={alertSeverity?.total || 0}
            />
          </CardContent>
        </Card>

        {/* 最近告警终端 - 全宽表格 */}
        <Card className="mb-4">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              最近告警终端
            </CardTitle>
            <CardDescription>产生告警最多的终端列表</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-hidden rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50">
                    <TableHead className="text-xs">主机名</TableHead>
                    <TableHead className="text-xs">IP 地址</TableHead>
                    <TableHead className="text-xs">告警数</TableHead>
                    <TableHead className="text-xs">严重程度</TableHead>
                    <TableHead className="text-xs">最后告警时间</TableHead>
                    <TableHead className="text-xs">状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recentAlertHosts.map((host, index) => (
                    <TableRow key={index} className="hover:bg-muted/50">
                      <TableCell className="font-medium text-sm">{host.hostname}</TableCell>
                      <TableCell className="font-mono text-xs">{host.ip_address}</TableCell>
                      <TableCell className="text-sm">{host.alert_count}</TableCell>
                      <TableCell>{getSeverityBadge(host.severity)}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {new Date(host.last_alert_time).toLocaleString('zh-CN')}
                      </TableCell>
                      <TableCell>
                        <Badge variant="default" className="text-xs">活跃</Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        {/* 系统性能指标 - 使用优化后的组件 */}
        <div className="mb-4">
          <PerformanceMetrics systemPerformance={systemPerformance} />
        </div>

      </div>
    </div>
  );
}
