"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import {
  Cpu,
  HardDrive,
  Network,
  Activity,
  TrendingUp,
  TrendingDown,
  Database,
  Zap,
} from "lucide-react";

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

interface PerformanceMetricsProps {
  systemPerformance: SystemPerformance | null;
}

// 格式化数字
const formatNumber = (num: number) => {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
  return num.toString();
};

export function PerformanceMetrics({ systemPerformance }: PerformanceMetricsProps) {
  if (!systemPerformance) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Cpu className="h-5 w-5" />
            系统性能指标
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <div className="text-center space-y-2">
              <Cpu className="h-6 w-6 mx-auto animate-pulse" />
              <p className="text-sm">加载性能数据...</p>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Cpu className="h-5 w-5" />
          系统性能指标
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
          {/* Kafka 磁盘使用率 */}
          <div className="text-center p-4 bg-secondary/30 rounded-lg border border-secondary">
            <div className="flex items-center justify-center mb-1">
              <span className="text-xs font-medium text-muted-foreground bg-secondary/50 px-2 py-1 rounded">Kafka</span>
            </div>
            <HardDrive className="h-6 w-6 text-primary mx-auto mb-2" />
            <div className="text-2xl font-bold text-primary">
              {systemPerformance.kafka.disk_usage}%
            </div>
            <div className="text-xs text-muted-foreground mb-2">磁盘使用率</div>
            <Progress 
              value={systemPerformance.kafka.disk_usage} 
              className="h-1.5"
            />
          </div>

          {/* Kafka 消息速率 */}
          <div className="text-center p-4 bg-secondary/30 rounded-lg border border-secondary">
            <div className="flex items-center justify-center mb-1">
              <span className="text-xs font-medium text-muted-foreground bg-secondary/50 px-2 py-1 rounded">Kafka</span>
            </div>
            <Network className="h-6 w-6 text-primary mx-auto mb-2" />
            <div className="text-2xl font-bold text-primary">
              {formatNumber(systemPerformance.kafka.messages_per_sec)}
            </div>
            <div className="text-xs text-muted-foreground">消息/秒</div>
          </div>

          {/* Kafka Brokers */}
          <div className="text-center p-4 bg-secondary/30 rounded-lg border border-secondary">
            <div className="flex items-center justify-center mb-1">
              <span className="text-xs font-medium text-muted-foreground bg-secondary/50 px-2 py-1 rounded">Kafka</span>
            </div>
            <Database className="h-6 w-6 text-primary mx-auto mb-2" />
            <div className="text-2xl font-bold text-primary">
              {systemPerformance.kafka.brokers}
            </div>
            <div className="text-xs text-muted-foreground">Brokers</div>
          </div>

          {/* Flink 内存使用率 */}
          <div className="text-center p-4 bg-accent/30 rounded-lg border border-accent">
            <div className="flex items-center justify-center mb-1">
              <span className="text-xs font-medium text-muted-foreground bg-accent/50 px-2 py-1 rounded">Flink</span>
            </div>
            <Cpu className="h-6 w-6 text-primary mx-auto mb-2" />
            <div className="text-2xl font-bold text-primary">
              {systemPerformance.flink.memory_usage}%
            </div>
            <div className="text-xs text-muted-foreground mb-2">内存使用率</div>
            <Progress 
              value={systemPerformance.flink.memory_usage} 
              className="h-1.5"
            />
          </div>

          {/* Flink 处理速率 */}
          <div className="text-center p-4 bg-accent/30 rounded-lg border border-accent">
            <div className="flex items-center justify-center mb-1">
              <span className="text-xs font-medium text-muted-foreground bg-accent/50 px-2 py-1 rounded">Flink</span>
            </div>
            <TrendingUp className="h-6 w-6 text-primary mx-auto mb-2" />
            <div className="text-2xl font-bold text-primary">
              {formatNumber(systemPerformance.flink.processing_rate)}
            </div>
            <div className="text-xs text-muted-foreground">处理/秒</div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
