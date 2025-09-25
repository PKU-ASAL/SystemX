"use client";

import * as React from "react";
import {
  IconHeart,
  IconActivity,
  IconCircleCheck,
  IconRefresh,
  IconDatabase,
  IconSearch,
  IconChartLine,
  IconVector,
  IconAlertTriangle,
  IconClock,
  IconServer,
} from "@tabler/icons-react";
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
import { apiClient, type HealthStatus, type WorkerStatus } from "@/lib/api";

interface ComponentStatus {
  name: string;
  status: "healthy" | "unhealthy" | "checking";
  responseTime?: number;
  lastChecked: Date;
  error?: string;
  icon: React.ComponentType<any>;
  description: string;
}

interface HealthData {
  data: {
    components: {
      database?: {
        healthy: boolean;
        response_time: number;
        status: string;
      };
      kafka?: {
        healthy: boolean;
        response_time: number;
        status: string;
      };
      opensearch?: {
        healthy: boolean;
        response_time: number;
        status: string;
      };
      prometheus?: {
        healthy: boolean;
        response_time: number;
        status: string;
      };
    };
  };
  checked_at?: string;
  healthy?: boolean;
  status?: string;
}

export function HealthStatus() {
  const [components, setComponents] = React.useState<ComponentStatus[]>([]);
  const [loading, setLoading] = React.useState(true);

  const fetchHealthStatus = React.useCallback(async () => {
    try {
      setLoading(true);

      // 使用正确的健康检查API端点
      const rawResponse = await fetch("/api/v1/health");
      const healthData = await rawResponse.json();
      
      console.log("Health API response:", healthData);

      const componentConfigs = [
        {
          service: "manager",
          component: "database",
          name: "数据库",
          icon: IconDatabase,
          description: "PostgreSQL 数据库连接状态",
        },
        {
          service: "indexer",
          component: "opensearch",
          name: "OpenSearch",
          icon: IconSearch,
          description: "OpenSearch 搜索引擎状态",
        },
        {
          service: "middleware",
          component: "kafka",
          name: "Kafka",
          icon: IconServer,
          description: "Kafka 消息队列状态",
        },
        {
          service: "middleware",
          component: "prometheus",
          name: "Prometheus",
          icon: IconChartLine,
          description: "Prometheus 监控系统状态",
        },
        {
          service: "middleware",
          component: "vector",
          name: "Vector",
          icon: IconVector,
          description: "Vector 日志收集器状态",
        },
        {
          service: "processor",
          component: "flink",
          name: "Flink",
          icon: IconActivity,
          description: "Flink 流处理引擎状态",
        },
      ];

      const currentTime = new Date();
      const checkedAt = healthData.data?.checked_at
        ? new Date(healthData.data.checked_at)
        : currentTime;

      const componentStatuses: ComponentStatus[] = componentConfigs.map(
        (config) => {
          const serviceData = healthData.data?.services?.[config.service];
          const componentData = serviceData?.components?.[config.component];

          if (!serviceData || !componentData) {
            return {
              name: config.name,
              status: "unhealthy",
              lastChecked: checkedAt,
              error: "组件数据不可用",
              icon: config.icon,
              description: config.description,
            };
          }

          return {
            name: config.name,
            status: componentData.healthy ? "healthy" : "unhealthy",
            responseTime: componentData.response_time ? Math.round(componentData.response_time / 1000000) : undefined, // 转换纳秒为毫秒
            lastChecked: checkedAt,
            error: !componentData.healthy
              ? `状态: ${componentData.status}`
              : undefined,
            icon: config.icon,
            description: config.description,
          };
        }
      );

      setComponents(componentStatuses);
    } catch (error) {
      console.error("Failed to fetch health status:", error);
      setComponents([]);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    fetchHealthStatus();
  }, []);

  const getStatusBadge = (status: string) => {
    const statusMap: Record<
      string,
      {
        variant: "default" | "secondary" | "destructive" | "outline";
        label: string;
      }
    > = {
      healthy: { variant: "default", label: "健康" },
      unhealthy: { variant: "destructive", label: "异常" },
      unknown: { variant: "secondary", label: "未知" },
    };

    const statusInfo = statusMap[status] || {
      variant: "outline",
      label: status,
    };
    return <Badge variant={statusInfo.variant}>{statusInfo.label}</Badge>;
  };

  const formatResponseTime = (responseTime: number) => {
    if (responseTime < 1000) {
      return `${responseTime}ms`;
    }
    return `${(responseTime / 1000).toFixed(2)}s`;
  };

  const getResponseTimeColor = (responseTime: number) => {
    if (responseTime > 5000) return "text-red-600";
    if (responseTime > 2000) return "text-yellow-600";
    return "text-green-600";
  };

  const formatLastCheck = (lastCheck: string) => {
    try {
      const date = new Date(lastCheck);
      return date.toLocaleString();
    } catch {
      return lastCheck;
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 lg:px-6 py-4 border-b">
        <div>
          <h1 className="text-2xl font-semibold flex items-center gap-2">
            <IconHeart className="h-6 w-6" />
            系统健康状态
          </h1>
          <p className="text-muted-foreground mt-1">
            核心系统组件的健康状态监控
          </p>
        </div>
        <Button
          onClick={fetchHealthStatus}
          variant="outline"
          size="sm"
          disabled={loading}
        >
          <IconRefresh
            className={`h-4 w-4 mr-2 ${loading ? "animate-spin" : ""}`}
          />
          刷新
        </Button>
      </div>

      {/* 组件健康状态 */}
      <div className="flex-1 px-4 lg:px-6 py-6">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <IconRefresh className="h-6 w-6 animate-spin" />
            <span className="ml-2">加载中...</span>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-5">
            {components.map((component) => {
              const IconComponent = component.icon;
              return (
                <Card key={component.name} className="relative">
                  <CardHeader className="pb-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <div
                          className={`p-2 rounded-full ${
                            component.status === "healthy"
                              ? "bg-green-100"
                              : component.status === "unhealthy"
                              ? "bg-red-100"
                              : "bg-yellow-100"
                          }`}
                        >
                          <IconComponent
                            className={`h-4 w-4 ${
                              component.status === "healthy"
                                ? "text-green-600"
                                : component.status === "unhealthy"
                                ? "text-red-600"
                                : "text-yellow-600"
                            }`}
                          />
                        </div>
                        <div>
                          <CardTitle className="text-sm font-medium">
                            {component.name}
                          </CardTitle>
                        </div>
                      </div>
                      <div
                        className={`w-2 h-2 rounded-full ${
                          component.status === "healthy"
                            ? "bg-green-500"
                            : component.status === "unhealthy"
                            ? "bg-red-500"
                            : "bg-yellow-500 animate-pulse"
                        }`}
                      />
                    </div>
                  </CardHeader>
                  <CardContent className="pt-0">
                    <div className="space-y-1">
                      <div className="flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">状态</span>
                        {getStatusBadge(component.status)}
                      </div>
                      {component.responseTime && (
                        <div className="flex items-center justify-between text-xs">
                          <span className="text-muted-foreground">
                            响应时间
                          </span>
                          <span
                            className={getResponseTimeColor(
                              component.responseTime
                            )}
                          >
                            {formatResponseTime(component.responseTime)}
                          </span>
                        </div>
                      )}
                      <div className="flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">最后检查</span>
                        <span className="text-muted-foreground">
                          {component.lastChecked.toLocaleTimeString()}
                        </span>
                      </div>
                    </div>
                    {component.error && (
                      <div className="mt-2 p-2 bg-red-50 rounded text-xs text-red-600">
                        <div className="flex items-start gap-1">
                          <IconAlertTriangle className="h-3 w-3 mt-0.5 flex-shrink-0" />
                          <span className="break-words">{component.error}</span>
                        </div>
                      </div>
                    )}
                  </CardContent>
                  <CardFooter className="pt-0">
                    <p className="text-xs text-muted-foreground">
                      {component.description}
                    </p>
                  </CardFooter>
                </Card>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
