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
import { apiClient, ApiError } from "@/lib/api";
import {
  RefreshCw,
  Server,
  Activity,
  Database,
  Users,
  TrendingUp,
  AlertTriangle,
  CheckCircle,
  Clock,
  BarChart3,
} from "lucide-react";

export function Dashboard() {
  const [stats, setStats] = useState({
    totalCollectors: 0,
    activeCollectors: 0,
    totalTopics: 0,
    totalConsumerGroups: 0,
    systemHealth: "healthy",
    lastUpdated: null as Date | null,
  });
  const [loading, setLoading] = useState(true);
  const [recentCollectors, setRecentCollectors] = useState<any[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<
    "connected" | "disconnected" | "connecting"
  >("connecting");

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      setError(null);
      setConnectionStatus("connecting");

      // 获取 Collector 统计 - 使用单独的 try-catch 处理可能的失败
      let collectorsResponse: any = { data: [], total: 0 };
      let collectors: any[] = [];
      let collectorsError = false;
      try {
        collectorsResponse = await apiClient.getCollectors({ limit: 5 });
        collectors = collectorsResponse.data || [];
      } catch (collectorsErr) {
        console.warn("Failed to fetch collectors:", collectorsErr);
        collectorsError = true;
      }

      // 获取 Kafka Topics 统计 - 使用单独的 try-catch 处理可能的失败
      let topics: any[] = [];
      let topicsError = false;
      try {
        const topicsResponse = await apiClient.getKafkaTopics();
        topics = topicsResponse.data || [];
      } catch (topicsErr) {
        console.warn("Failed to fetch Kafka topics:", topicsErr);
        topicsError = true;
      }

      // 获取 Kafka Consumer Groups 统计 - 使用单独的 try-catch 处理可能的失败
      let consumerGroups: any[] = [];
      let consumerGroupsError = false;
      try {
        const consumerGroupsResponse = await apiClient.getKafkaConsumerGroups();
        consumerGroups = consumerGroupsResponse.data || [];
      } catch (consumerGroupsErr) {
        console.warn("Failed to fetch consumer groups:", consumerGroupsErr);
        consumerGroupsError = true;
      }

      // 获取系统健康状态 - 使用单独的 try-catch 处理可能的失败
      let healthResponse: any = { status: "unknown" };
      try {
        healthResponse = await apiClient.getHealth();
      } catch (healthErr) {
        console.warn("Failed to fetch health status:", healthErr);
      }

      setStats({
        totalCollectors: collectorsResponse.total || 0,
        activeCollectors: collectors.filter((c) => c.status === "active")
          .length,
        totalTopics: topics.length,
        totalConsumerGroups: consumerGroups.length,
        systemHealth: healthResponse.status,
        lastUpdated: new Date(),
      });

      setRecentCollectors(collectors.slice(0, 5));
      setConnectionStatus("connected");

      // 如果部分服务失败，显示警告
      if (collectorsError || topicsError || consumerGroupsError) {
        setError("部分服务连接失败，数据可能不完整");
      }
    } catch (error) {
      console.error("Failed to fetch dashboard data:", error);
      setConnectionStatus("disconnected");

      if (error instanceof ApiError) {
        setError(`连接失败: ${error.message}`);
      } else {
        setError("无法连接到后端服务，请检查网络连接");
      }

      // 如果 API 调用失败，设置默认值
      setStats((prev) => ({
        ...prev,
        totalTopics: 0,
        totalConsumerGroups: 0,
        systemHealth: "unhealthy",
        lastUpdated: new Date(),
      }));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // 检查当前路径，只在Dashboard页面执行数据获取
    const currentPath = window.location.pathname;
    if (currentPath === '/' || currentPath === '/dashboard') {
      fetchDashboardData();
      // 设置定时刷新，每30秒更新一次  
      const interval = setInterval(fetchDashboardData, 30000);
      return () => clearInterval(interval);
    } else {
      console.log(`🛑 [DASHBOARD] 跳过数据获取，当前页面: ${currentPath}`);
      setLoading(false);
    }
  }, []);

  const getStatusBadge = (status: string) => {
    const statusMap: Record<
      string,
      {
        variant: "default" | "secondary" | "destructive" | "outline";
        label: string;
      }
    > = {
      active: { variant: "default", label: "在线" },
      inactive: { variant: "destructive", label: "离线" },
      unknown: { variant: "secondary", label: "未知" },
    };

    const statusInfo = statusMap[status] || {
      variant: "outline",
      label: status,
    };
    return <Badge variant={statusInfo.variant}>{statusInfo.label}</Badge>;
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-auto p-4 lg:p-6">
        {/* 统计卡片 */}
        <div className="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 mb-8 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs md:grid-cols-2 lg:grid-cols-4">
          <Card className="@container/card">
            <CardHeader>
              <CardDescription>总终端数</CardDescription>
              <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-blue-600">
                {stats.totalCollectors}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-blue-100">
                  <Server className="h-5 w-5 text-blue-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                系统中注册的终端总数 <Server className="size-4" />
              </div>
              <div className="text-muted-foreground">
                包含所有部署类型的终端
              </div>
            </CardFooter>
          </Card>

          <Card className="@container/card">
            <CardHeader>
              <CardDescription>在线终端</CardDescription>
              <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-green-600">
                {stats.activeCollectors}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  <Activity className="h-5 w-5 text-green-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                当前活跃的终端数量 <Activity className="size-4" />
              </div>
              <div className="text-muted-foreground">正在正常工作的终端</div>
            </CardFooter>
          </Card>

          <Card className="@container/card">
            <CardHeader>
              <CardDescription>Kafka Topics</CardDescription>
              <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-purple-600">
                {stats.totalTopics}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-purple-100">
                  <Database className="h-5 w-5 text-purple-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                消息队列主题数量 <Database className="size-4" />
              </div>
              <div className="text-muted-foreground">数据传输通道总数</div>
            </CardFooter>
          </Card>

          <Card className="@container/card">
            <CardHeader>
              <CardDescription>系统状态</CardDescription>
              <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-green-600">
                {stats.systemHealth === "healthy" ? "正常" : "异常"}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  {stats.systemHealth === "healthy" ? (
                    <CheckCircle className="h-5 w-5 text-green-600" />
                  ) : (
                    <AlertTriangle className="h-5 w-5 text-red-600" />
                  )}
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                整体系统运行状态{" "}
                {stats.systemHealth === "healthy" ? (
                  <CheckCircle className="size-4" />
                ) : (
                  <AlertTriangle className="size-4" />
                )}
              </div>
              <div className="text-muted-foreground">所有核心服务状态</div>
            </CardFooter>
          </Card>
        </div>

        {/* 内容区域 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* 最近注册的终端 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Server className="h-5 w-5" />
                最近注册的终端
              </CardTitle>
              <CardDescription>最近注册的 5 个 Collector 终端</CardDescription>
            </CardHeader>
            <CardContent>
              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <RefreshCw className="h-6 w-6 animate-spin mr-2" />
                  <span>加载中...</span>
                </div>
              ) : recentCollectors.length === 0 ? (
                <div className="text-center py-8 text-gray-500">
                  暂无终端数据
                </div>
              ) : (
                <div className="space-y-4">
                  {recentCollectors.map((collector, index) => (
                    <div
                      key={collector.collector_id || index}
                      className="flex items-center justify-between p-3 border rounded-lg"
                    >
                      <div className="flex items-center gap-3">
                        <div className="p-2 rounded-full bg-blue-100">
                          <Server className="h-4 w-4 text-blue-600" />
                        </div>
                        <div>
                          <p className="font-medium">{collector.hostname}</p>
                          <p className="text-sm text-gray-500">
                            {collector.ip_address}
                          </p>
                        </div>
                      </div>
                      <div className="text-right">
                        {getStatusBadge(collector.status)}
                        <p className="text-xs text-gray-500 mt-1">
                          {collector.created_at
                            ? new Date(
                                collector.created_at
                              ).toLocaleDateString()
                            : "未知"}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* 系统概览 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                系统概览
              </CardTitle>
              <CardDescription>系统关键指标和状态信息</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-full bg-green-100">
                      <CheckCircle className="h-4 w-4 text-green-600" />
                    </div>
                    <div>
                      <p className="font-medium">系统健康状态</p>
                      <p className="text-sm text-gray-500">所有服务正常运行</p>
                    </div>
                  </div>
                  <Badge variant="default">正常</Badge>
                </div>

                <div className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-full bg-blue-100">
                      <Database className="h-4 w-4 text-blue-600" />
                    </div>
                    <div>
                      <p className="font-medium">Kafka 集群</p>
                      <p className="text-sm text-gray-500">消息队列服务</p>
                    </div>
                  </div>
                  <Badge variant="default">运行中</Badge>
                </div>

                <div className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-full bg-purple-100">
                      <Users className="h-4 w-4 text-purple-600" />
                    </div>
                    <div>
                      <p className="font-medium">数据收集</p>
                      <p className="text-sm text-gray-500">终端数据采集状态</p>
                    </div>
                  </div>
                  <Badge variant="default">活跃</Badge>
                </div>

                <div className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-full bg-orange-100">
                      <AlertTriangle className="h-4 w-4 text-orange-600" />
                    </div>
                    <div>
                      <p className="font-medium">告警监控</p>
                      <p className="text-sm text-gray-500">安全事件监控</p>
                    </div>
                  </div>
                  <Badge variant="secondary">待配置</Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
