"use client";

import React, { useState, useEffect } from "react";
import { apiClient, type KafkaBroker } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  CheckCircle,
  Server,
  Activity,
  Database,
  AlertTriangle,
  RefreshCw,
  Search,
  HardDrive,
  Cpu,
  Network,
  MessageSquare,
} from "lucide-react";

export default function KafkaPage() {
  const [brokers, setBrokers] = useState<KafkaBroker[]>([]);
  const [clusterStats, setClusterStats] = useState<any>({});
  const [topics, setTopics] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");

  const fetchData = async () => {
    try {
      setLoading(true);
      const [brokersResponse, clusterResponse, topicsResponse] = await Promise.all([
        fetch('/api/v1/services/kafka/brokers').then(res => res.json()).catch(() => ({ data: [] })),
        fetch('/api/v1/services/kafka/clusters').then(res => res.json()).catch(() => ({ data: [] })),
        fetch('/api/v1/services/kafka/topics').then(res => res.json()).catch(() => ({ data: { topics: [] } })),
      ]);
      
      console.log('Kafka API responses:', { brokersResponse, clusterResponse, topicsResponse });
      
      setBrokers(brokersResponse.data || []);
      setClusterStats(clusterResponse.data || []);
      
      // 处理真实的 topics 数据
      const topicsData = topicsResponse.data?.topics || [];
      const processedTopics = topicsData.map((topic: any) => ({
        name: topic.name,
        partitions: topic.partition_count,
        replicas: topic.replication_factor || 1,
        messages: topic.messages_total || 0,
        size: topic.bytes_total_formatted || "0 B",
        retention: topic.retention || "7 days"
      }));
      setTopics(processedTopics);
    } catch (error) {
      console.error("Failed to fetch Kafka data:", error);
      setBrokers([]);
      setClusterStats({});
      setTopics([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const clusterInfo = clusterStats?.data?.[0] || {};
  const {
    broker_count: brokerCount = 1,
    active_controllers: activeControllers = 1,
    version = "3.4-IV0",
    online_partition_count: onlinePartitionCount = 0,
  } = clusterInfo;

  const filteredBrokers = brokers.filter(
    (broker) =>
      broker.host.toLowerCase().includes(searchTerm.toLowerCase()) ||
      broker.id.toString().includes(searchTerm)
  );

  const filteredTopics = topics.filter((topic) =>
    topic.name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <MessageSquare className="h-4 w-4 text-primary" />
              消息队列
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              Kafka 集群状态、Broker 节点和 Topic 管理
            </p>
          </div>
          <Button
            onClick={fetchData}
            disabled={loading}
            variant="outline"
            size="sm"
          >
            {loading ? (
              <RefreshCw className="h-4 w-4 animate-spin mr-2" />
            ) : (
              <RefreshCw className="h-4 w-4 mr-2" />
            )}
            刷新
          </Button>
        </div>
      </div>

      {/* 集群概览卡片 */}
      <div className="px-4 lg:px-6 py-4 border-b border-border">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardHeader>
              <CardDescription>Broker 节点</CardDescription>
              <CardTitle className="text-2xl font-semibold text-blue-600">
                {brokerCount}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-blue-100">
                  <Server className="h-5 w-5 text-blue-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                集群节点数 <Server className="size-4" />
              </div>
              <div className="text-muted-foreground">活跃的 Kafka 节点</div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>Topic 数量</CardDescription>
              <CardTitle className="text-2xl font-semibold text-green-600">
                {topics.length}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  <Database className="h-5 w-5 text-green-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                消息主题 <Database className="size-4" />
              </div>
              <div className="text-muted-foreground">配置的消息主题数</div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>在线分区</CardDescription>
              <CardTitle className="text-2xl font-semibold text-purple-600">
                {onlinePartitionCount}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-purple-100">
                  <Activity className="h-5 w-5 text-purple-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                分区状态 <Activity className="size-4" />
              </div>
              <div className="text-muted-foreground">正常工作的分区数</div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>Kafka 版本</CardDescription>
              <CardTitle className="text-2xl font-semibold text-orange-600">
                {version}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-orange-100">
                  <CheckCircle className="h-5 w-5 text-orange-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                版本信息 <CheckCircle className="size-4" />
              </div>
              <div className="text-muted-foreground">当前运行版本</div>
            </CardFooter>
          </Card>
        </div>
      </div>

      {/* 搜索框 */}
      <div className="px-4 lg:px-6 py-3 border-b border-border">
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            type="text"
            className="w-full pl-10 pr-4 py-2 text-sm border border-input rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"
            placeholder="搜索 Broker 或 Topic..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>

      {/* Tabs 内容 */}
      <div className="flex-1 overflow-hidden">
        <Tabs defaultValue="brokers" className="h-full flex flex-col">
          <div className="px-4 lg:px-6 py-3 border-b border-border bg-muted/20">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="brokers" className="flex-1">Broker 节点</TabsTrigger>
              <TabsTrigger value="topics" className="flex-1">Topic 主题</TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="brokers" className="flex-1 overflow-auto mt-0">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-primary" />
                <span className="text-sm text-muted-foreground">
                  加载 Broker 信息...
                </span>
              </div>
            ) : filteredBrokers.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                <Server className="h-8 w-8 mx-auto mb-3 text-muted-foreground/50" />
                <p className="text-sm font-medium text-foreground">
                  未找到 Broker 节点
                </p>
                <p className="text-xs text-muted-foreground">
                  {searchTerm ? "尝试调整搜索条件" : "没有在线的 Broker 节点"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Broker ID</TableHead>
                      <TableHead>主机地址</TableHead>
                      <TableHead>端口</TableHead>
                      <TableHead>磁盘使用率</TableHead>
                      <TableHead>分区数</TableHead>
                      <TableHead>状态</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredBrokers.map((broker) => (
                      <TableRow key={broker.id}>
                        <TableCell className="font-medium">
                          <div className="flex items-center gap-2">
                            {broker.id}
                            {broker.controller && (
                              <Badge variant="secondary" className="text-xs">
                                Controller
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {broker.host}
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {broker.port}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <div className="w-16 bg-gray-200 rounded-full h-2">
                              <div
                                className="bg-blue-600 h-2 rounded-full"
                                style={{
                                  width: `${Math.min(broker.disk_usage.usage_percentage, 100)}%`
                                }}
                              ></div>
                            </div>
                            <span className="text-sm font-mono">
                              {broker.disk_usage.usage_percentage.toFixed(1)}%
                            </span>
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          {broker.in_sync_partitions}
                        </TableCell>
                        <TableCell>
                          <Badge variant={broker.controller ? "default" : "secondary"}>
                            {broker.controller ? "Controller" : "Follower"}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </TabsContent>

          <TabsContent value="topics" className="flex-1 overflow-auto mt-0">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-primary" />
                <span className="text-sm text-muted-foreground">
                  加载 Topic 信息...
                </span>
              </div>
            ) : filteredTopics.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                <Database className="h-8 w-8 mx-auto mb-3 text-muted-foreground/50" />
                <p className="text-sm font-medium text-foreground">
                  未找到 Topic 主题
                </p>
                <p className="text-xs text-muted-foreground">
                  {searchTerm ? "尝试调整搜索条件" : "没有配置的 Topic 主题"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Topic 名称</TableHead>
                      <TableHead>分区数</TableHead>
                      <TableHead>副本数</TableHead>
                      <TableHead>消息数</TableHead>
                      <TableHead>大小</TableHead>
                      <TableHead>保留时间</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredTopics.map((topic) => (
                      <TableRow key={topic.name}>
                        <TableCell className="font-medium">
                          <div className="flex items-center gap-2">
                            <MessageSquare className="h-4 w-4 text-primary" />
                            <span className="font-mono text-sm">{topic.name}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          {topic.partitions}
                        </TableCell>
                        <TableCell className="text-center">
                          {topic.replicas}
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {topic.messages.toLocaleString()}
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {topic.size}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline">{topic.retention}</Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
