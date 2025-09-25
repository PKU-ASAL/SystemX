"use client";

import React from "react";
import { apiClient, type KafkaTopic } from "@/lib/api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ArrowLeft,
  Database,
  Users,
  Settings,
  MessageSquare,
  Copy,
  BarChart3,
  HardDrive,
  Search,
  RefreshCw,
  ChevronRight,
  ChevronDown,
} from "lucide-react";
import { CardAction, CardFooter } from "@/components/ui/card";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

interface KafkaTopicDetailProps {
  topicName: string;
  onBack: () => void;
}

// Helper function to get configuration parameter descriptions
function getConfigDescription(key: string): string {
  const descriptions: Record<string, string> = {
    "retention.ms": "消息保留时间（毫秒）",
    "retention.bytes": "每个分区的最大数据保留大小",
    "segment.ms": "日志段滚动时间间隔（毫秒）",
    "segment.bytes": "日志段的最大大小",
    "cleanup.policy": "日志清理策略（delete 或 compact）",
    "compression.type": "消息压缩类型",
    "min.insync.replicas": "最小同步副本数",
    "max.message.bytes": "单条消息的最大大小",
    "flush.messages": "强制刷新到磁盘的消息数",
    "flush.ms": "强制刷新到磁盘的时间间隔",
    "index.interval.bytes": "索引条目之间的字节间隔",
    "delete.retention.ms": "删除记录的保留时间",
    "file.delete.delay.ms": "文件删除延迟时间",
    "min.cleanable.dirty.ratio": "日志压缩的最小脏比率",
    "min.compaction.lag.ms": "消息在日志中保持未压缩的最小时间",
    "max.compaction.lag.ms": "消息在日志中保持未压缩的最大时间",
    "segment.index.bytes": "段索引文件的最大大小",
    "segment.jitter.ms": "段滚动时间的随机抖动",
    preallocate: "是否预分配日志段文件",
    "message.format.version": "消息格式版本",
    "message.timestamp.type": "消息时间戳类型",
    "message.timestamp.difference.max.ms": "消息时间戳的最大差异",
    "unclean.leader.election.enable": "是否允许不干净的 leader 选举",
    "follower.replication.throttled.replicas": "限流的 follower 副本",
    "leader.replication.throttled.replicas": "限流的 leader 副本",
  };

  return descriptions[key] || "Kafka Topic 配置参数";
}

export function KafkaTopicDetail({ topicName, onBack }: KafkaTopicDetailProps) {
  const [topic, setTopic] = React.useState<KafkaTopic | null>(null);
  const [messages, setMessages] = React.useState<any[]>([]);
  const [topicConfig, setTopicConfig] = React.useState<Record<string, any>>({});
  const [loading, setLoading] = React.useState(true);
  const [messagesLoading, setMessagesLoading] = React.useState(false);
  const [configLoading, setConfigLoading] = React.useState(false);
  const [activeTab, setActiveTab] = React.useState("overview");
  const [expandedMessages, setExpandedMessages] = React.useState<Set<number>>(
    new Set()
  );

  const toggleMessageExpansion = (index: number) => {
    setExpandedMessages((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(index)) {
        newSet.delete(index);
      } else {
        newSet.add(index);
      }
      return newSet;
    });
  };

  React.useEffect(() => {
    const fetchTopicDetail = async () => {
      try {
        setLoading(true);
        const response = await apiClient.getKafkaTopicDetail(topicName);
        setTopic(response.data);
      } catch (error) {
        console.error("Failed to fetch topic detail:", error);
      } finally {
        setLoading(false);
      }
    };

    fetchTopicDetail();
  }, [topicName]);

  const fetchMessages = async () => {
    try {
      setMessagesLoading(true);
      const response = await apiClient.getKafkaTopicMessages(topicName, {
        limit: 50,
        offset: "latest",
      });
      setMessages(response.data);
    } catch (error) {
      console.error("Failed to fetch messages:", error);
      setMessages([]);
    } finally {
      setMessagesLoading(false);
    }
  };

  const fetchTopicConfig = async () => {
    try {
      setConfigLoading(true);
      const response = await apiClient.getKafkaTopicConfig(topicName);
      setTopicConfig(response.data);
    } catch (error) {
      console.error("Failed to fetch topic config:", error);
      setTopicConfig({});
    } finally {
      setConfigLoading(false);
    }
  };

  React.useEffect(() => {
    if (activeTab === "messages" && topic) {
      fetchMessages();
    } else if (activeTab === "settings" && topic) {
      fetchTopicConfig();
    }
  }, [activeTab, topic, topicName]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="text-gray-500">Loading...</div>
      </div>
    );
  }

  if (!topic) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="text-gray-500">Topic not found</div>
      </div>
    );
  }

  return (
    <div className="@container/main flex flex-1 flex-col overflow-hidden">
      <div className="flex h-full bg-gray-50">
        {/* 主内容区域 */}
        <div className="flex-1 bg-white flex flex-col min-w-0">
          {/* Header */}
          <div className="flex items-center justify-between px-4 lg:px-6 py-4 border-b">
            <div className="space-y-2">
              <Breadcrumb>
                <BreadcrumbList>
                  <BreadcrumbItem>
                    <BreadcrumbLink
                      href="#"
                      onClick={(e) => {
                        e.preventDefault();
                        if (typeof window !== "undefined") {
                          window.history.pushState(null, "", "/dashboard");
                          window.location.reload();
                        }
                      }}
                    >
                      Dashboard
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbLink
                      href="#"
                      onClick={(e) => {
                        e.preventDefault();
                        if (typeof window !== "undefined") {
                          window.location.href = "/kafka/topics";
                        }
                      }}
                    >
                      Topics
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbPage className="flex items-center gap-2">
                      <span className="font-mono">{topicName}</span>
                      {(topic.is_internal || topic.internal) && (
                        <Badge variant="secondary" className="text-xs">
                          IN
                        </Badge>
                      )}
                    </BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
            </div>
            <Button variant="outline" size="sm">
              Produce Message
            </Button>
          </div>

          {/* Topic Info Cards */}
          <div className="px-4 lg:px-6 py-4 border-b">
            <div className="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs md:grid-cols-2 lg:grid-cols-4">
              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Partitions</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-blue-600">
                    {topic.partition_count || 0}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-blue-100">
                      <Database className="h-5 w-5 text-blue-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    Topic 分区数量 <Database className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    数据分布的分区总数
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Replication Factor</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-green-600">
                    {topic.replication_factor || 0}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-green-100">
                      <Copy className="h-5 w-5 text-green-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    副本因子 <Copy className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    每个分区的副本数量
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Total Messages</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-purple-600">
                    {(
                      topic.messages_total || topic.messageCount
                    )?.toLocaleString() || "0"}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-purple-100">
                      <BarChart3 className="h-5 w-5 text-purple-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    消息总数 <BarChart3 className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    Topic 中的消息数量
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Total Size</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-orange-600">
                    {topic.size_formatted || topic.size || "0 Bytes"}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-orange-100">
                      <HardDrive className="h-5 w-5 text-orange-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    存储大小 <HardDrive className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    Topic 占用的存储空间
                  </div>
                </CardFooter>
              </Card>
            </div>
          </div>

          {/* EUI Style Tabs */}
          <div className="bg-white border-b border-gray-200 px-4 lg:px-6">
            <div className="flex space-x-8">
              <button
                className={`py-3 px-1 border-b-2 font-medium text-sm ${
                  activeTab === "overview"
                    ? "border-blue-500 text-blue-600"
                    : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
                }`}
                onClick={() => setActiveTab("overview")}
              >
                <div className="flex items-center gap-2">
                  <Database className="h-4 w-4" />
                  Overview
                </div>
              </button>
              <button
                className={`py-3 px-1 border-b-2 font-medium text-sm ${
                  activeTab === "messages"
                    ? "border-blue-500 text-blue-600"
                    : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
                }`}
                onClick={() => setActiveTab("messages")}
              >
                <div className="flex items-center gap-2">
                  <MessageSquare className="h-4 w-4" />
                  Messages
                </div>
              </button>
              <button
                className={`py-3 px-1 border-b-2 font-medium text-sm ${
                  activeTab === "consumers"
                    ? "border-blue-500 text-blue-600"
                    : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
                }`}
                onClick={() => setActiveTab("consumers")}
              >
                <div className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  Consumers
                </div>
              </button>
              <button
                className={`py-3 px-1 border-b-2 font-medium text-sm ${
                  activeTab === "settings"
                    ? "border-blue-500 text-blue-600"
                    : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
                }`}
                onClick={() => setActiveTab("settings")}
              >
                <div className="flex items-center gap-2">
                  <Settings className="h-4 w-4" />
                  Settings
                </div>
              </button>
            </div>
          </div>

          {/* Tab Content */}
          <div className="flex-1 overflow-auto">
            {activeTab === "overview" && (
              <div className="h-full">
                {/* Search and Controls */}
                <div className="bg-white border-b border-gray-200 px-4 lg:px-6 py-3">
                  <div className="flex items-center gap-3 w-full">
                    <div className="flex-1 min-w-0">
                      <div className="relative">
                        <div className="flex items-stretch border border-gray-300 rounded-md bg-white shadow-sm hover:shadow-md transition-all focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500 h-9">
                          <div className="flex-1 relative">
                            <input
                              type="text"
                              className="w-full px-3 py-2 text-sm border-0 bg-transparent focus:outline-none focus:ring-0 placeholder-gray-500 pr-12 h-full"
                              placeholder="Search partitions..."
                            />
                            <div className="absolute right-3 top-1/2 transform -translate-y-1/2">
                              <Search className="h-4 w-4 text-gray-400" />
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <Button variant="outline" size="sm">
                      <RefreshCw className="h-4 w-4 mr-2" />
                      刷新
                    </Button>
                  </div>
                </div>

                {/* Partition Details Table */}
                <div className="flex-1 overflow-auto">
                  <div className="overflow-x-auto">
                    <Table className="min-w-full">
                      <TableHeader className="bg-gray-50/80 sticky top-0 border-b border-gray-200">
                        <TableRow className="hover:bg-gray-50/80">
                          <TableHead className="w-10 px-3 py-2 text-left">
                            <Checkbox className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600" />
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[100px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <Database className="h-3.5 w-3.5" />
                              Partition ID
                            </div>
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[80px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <Users className="h-3.5 w-3.5" />
                              Leader
                            </div>
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[120px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <Copy className="h-3.5 w-3.5" />
                              Replicas
                            </div>
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[120px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <Settings className="h-3.5 w-3.5" />
                              In-Sync Replicas
                            </div>
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[80px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <HardDrive className="h-3.5 w-3.5" />
                              Size
                            </div>
                          </TableHead>
                          <TableHead className="px-3 py-2 text-left min-w-[100px]">
                            <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                              <BarChart3 className="h-3.5 w-3.5" />
                              Offset Lag
                            </div>
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {Array.isArray(topic.partitions) &&
                        topic.partitions.length > 0
                          ? topic.partitions.map((partition: any) => (
                              <TableRow
                                key={partition.partition_id}
                                className="border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors"
                              >
                                <TableCell className="px-3 py-2">
                                  <Checkbox className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600" />
                                </TableCell>
                                <TableCell className="px-3 py-2 font-mono text-sm">
                                  {partition.partition_id}
                                </TableCell>
                                <TableCell className="px-3 py-2">
                                  <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-blue-100/80 text-blue-700 border-blue-200/80">
                                    {partition.leader >= 0
                                      ? partition.leader
                                      : "N/A"}
                                  </span>
                                </TableCell>
                                <TableCell className="px-3 py-2">
                                  <div className="flex flex-wrap gap-1">
                                    {Array.isArray(partition.replicas) &&
                                    partition.replicas.length > 0 ? (
                                      partition.replicas.map(
                                        (replica: number, idx: number) => (
                                          <span
                                            key={`replica-${idx}`}
                                            className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border ${
                                              replica === partition.leader
                                                ? "bg-green-100/80 text-green-700 border-green-200/80"
                                                : "bg-gray-100/80 text-gray-700 border-gray-200/80"
                                            }`}
                                          >
                                            {replica}
                                            {replica === partition.leader &&
                                              " (L)"}
                                          </span>
                                        )
                                      )
                                    ) : (
                                      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                        N/A
                                      </span>
                                    )}
                                  </div>
                                </TableCell>
                                <TableCell className="px-3 py-2">
                                  <div className="flex flex-wrap gap-1">
                                    {Array.isArray(
                                      partition.in_sync_replicas
                                    ) &&
                                    partition.in_sync_replicas.length > 0 ? (
                                      partition.in_sync_replicas.map(
                                        (replica: number, idx: number) => (
                                          <span
                                            key={`isr-${idx}`}
                                            className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-green-100/80 text-green-700 border-green-200/80"
                                          >
                                            {replica}
                                          </span>
                                        )
                                      )
                                    ) : (
                                      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                        N/A
                                      </span>
                                    )}
                                  </div>
                                </TableCell>
                                <TableCell className="px-3 py-2 font-mono text-sm">
                                  {partition.size_formatted || "N/A"}
                                </TableCell>
                                <TableCell className="px-3 py-2">
                                  <span
                                    className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${
                                      partition.offset_lag > 1000
                                        ? "bg-red-100/80 text-red-700 border-red-200/80"
                                        : "bg-gray-100/80 text-gray-700 border-gray-200/80"
                                    }`}
                                  >
                                    {partition.offset_lag >= 0
                                      ? partition.offset_lag.toLocaleString()
                                      : "N/A"}
                                  </span>
                                </TableCell>
                              </TableRow>
                            ))
                          : Array.from(
                              {
                                length:
                                  topic.partition_count ||
                                  (typeof topic.partitions === "number"
                                    ? topic.partitions
                                    : topic.partitions?.length) ||
                                  1,
                              },
                              (_, i) => (
                                <TableRow
                                  key={i}
                                  className="border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors"
                                >
                                  <TableCell className="px-3 py-2">
                                    <Checkbox className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600" />
                                  </TableCell>
                                  <TableCell className="px-3 py-2 font-mono text-sm">
                                    {i}
                                  </TableCell>
                                  <TableCell className="px-3 py-2">
                                    <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                      N/A
                                    </span>
                                  </TableCell>
                                  <TableCell className="px-3 py-2">
                                    <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                      {topic.replication_factor} replicas
                                    </span>
                                  </TableCell>
                                  <TableCell className="px-3 py-2">
                                    <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                      N/A
                                    </span>
                                  </TableCell>
                                  <TableCell className="px-3 py-2 font-mono text-sm">
                                    N/A
                                  </TableCell>
                                  <TableCell className="px-3 py-2">
                                    <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                      N/A
                                    </span>
                                  </TableCell>
                                </TableRow>
                              )
                            )}
                      </TableBody>
                    </Table>
                  </div>
                </div>
              </div>
            )}

            {activeTab === "messages" && (
              <div className="h-full">
                <div className="bg-white border-b border-gray-200 px-4 lg:px-6 py-3">
                  <div className="flex items-center justify-between">
                    <h3 className="text-lg font-medium">Messages</h3>
                    <Button
                      onClick={fetchMessages}
                      disabled={messagesLoading}
                      variant="outline"
                      size="sm"
                    >
                      {messagesLoading ? "Loading..." : "Refresh"}
                    </Button>
                  </div>
                </div>
                <div className="flex-1 overflow-auto p-4">
                  {messagesLoading ? (
                    <div className="text-center py-8 text-gray-500">
                      Loading messages...
                    </div>
                  ) : messages.length === 0 ? (
                    <div className="text-center py-8 text-gray-500">
                      No messages to display
                    </div>
                  ) : (
                    <div className="border rounded-lg">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead className="w-10"></TableHead>
                            <TableHead>Partition</TableHead>
                            <TableHead>Offset</TableHead>
                            <TableHead>Timestamp</TableHead>
                            <TableHead>Key</TableHead>
                            <TableHead>Value Preview</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {messages.map((message, index) => (
                            <React.Fragment key={index}>
                              <TableRow
                                className="cursor-pointer hover:bg-gray-50"
                                onClick={() => toggleMessageExpansion(index)}
                              >
                                <TableCell className="w-10">
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-6 w-6 p-0"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      toggleMessageExpansion(index);
                                    }}
                                  >
                                    {expandedMessages.has(index) ? (
                                      <ChevronDown className="h-4 w-4" />
                                    ) : (
                                      <ChevronRight className="h-4 w-4" />
                                    )}
                                  </Button>
                                </TableCell>
                                <TableCell className="font-mono">
                                  {message.partition || 0}
                                </TableCell>
                                <TableCell className="font-mono">
                                  {message.offset || 0}
                                </TableCell>
                                <TableCell className="font-mono text-sm">
                                  {message.timestamp
                                    ? new Date(
                                        message.timestamp
                                      ).toLocaleString()
                                    : "N/A"}
                                </TableCell>
                                <TableCell className="font-mono text-sm max-w-32 truncate">
                                  {message.key || "null"}
                                </TableCell>
                                <TableCell className="font-mono text-sm max-w-64 truncate">
                                  {typeof message.value === "string"
                                    ? message.value
                                    : JSON.stringify(message.value)}
                                </TableCell>
                              </TableRow>
                              {expandedMessages.has(index) && (
                                <TableRow>
                                  <TableCell colSpan={6} className="p-0">
                                    <div className="bg-gray-50 border-t">
                                      <div className="p-4">
                                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                                          {/* Message Headers */}
                                          <div>
                                            <h4 className="text-sm font-medium text-gray-900 mb-3">
                                              Message Headers
                                            </h4>
                                            <div className="bg-white rounded border p-3 space-y-2">
                                              <div className="flex justify-between text-sm">
                                                <span className="text-gray-600">
                                                  Partition:
                                                </span>
                                                <span className="font-mono">
                                                  {message.partition || 0}
                                                </span>
                                              </div>
                                              <div className="flex justify-between text-sm">
                                                <span className="text-gray-600">
                                                  Offset:
                                                </span>
                                                <span className="font-mono">
                                                  {message.offset || 0}
                                                </span>
                                              </div>
                                              <div className="flex justify-between text-sm">
                                                <span className="text-gray-600">
                                                  Timestamp:
                                                </span>
                                                <span className="font-mono text-xs">
                                                  {message.timestamp
                                                    ? new Date(
                                                        message.timestamp
                                                      ).toISOString()
                                                    : "N/A"}
                                                </span>
                                              </div>
                                              <div className="flex justify-between text-sm">
                                                <span className="text-gray-600">
                                                  Key:
                                                </span>
                                                <span className="font-mono break-all">
                                                  {message.key || "null"}
                                                </span>
                                              </div>
                                              {message.headers &&
                                                Object.keys(message.headers)
                                                  .length > 0 && (
                                                  <div className="pt-2 border-t">
                                                    <span className="text-gray-600 text-sm">
                                                      Headers:
                                                    </span>
                                                    <div className="mt-1 space-y-1">
                                                      {Object.entries(
                                                        message.headers
                                                      ).map(([key, value]) => (
                                                        <div
                                                          key={key}
                                                          className="flex justify-between text-xs"
                                                        >
                                                          <span className="text-gray-500">
                                                            {key}:
                                                          </span>
                                                          <span className="font-mono">
                                                            {String(value)}
                                                          </span>
                                                        </div>
                                                      ))}
                                                    </div>
                                                  </div>
                                                )}
                                            </div>
                                          </div>

                                          {/* Message Value (JSON) */}
                                          <div>
                                            <h4 className="text-sm font-medium text-gray-900 mb-3">
                                              Message Value (JSON)
                                            </h4>
                                            <div className="bg-white rounded border">
                                              <pre className="p-3 text-xs font-mono overflow-auto max-h-96 whitespace-pre-wrap">
                                                {typeof message.value ===
                                                "string"
                                                  ? (() => {
                                                      try {
                                                        return JSON.stringify(
                                                          JSON.parse(
                                                            message.value
                                                          ),
                                                          null,
                                                          2
                                                        );
                                                      } catch {
                                                        return message.value;
                                                      }
                                                    })()
                                                  : JSON.stringify(
                                                      message.value,
                                                      null,
                                                      2
                                                    )}
                                              </pre>
                                            </div>
                                          </div>
                                        </div>

                                        {/* Raw Message Data */}
                                        <div className="mt-4">
                                          <h4 className="text-sm font-medium text-gray-900 mb-3">
                                            Raw Message Data
                                          </h4>
                                          <div className="bg-white rounded border">
                                            <pre className="p-3 text-xs font-mono overflow-auto max-h-48 whitespace-pre-wrap">
                                              {JSON.stringify(message, null, 2)}
                                            </pre>
                                          </div>
                                        </div>
                                      </div>
                                    </div>
                                  </TableCell>
                                </TableRow>
                              )}
                            </React.Fragment>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </div>
              </div>
            )}

            {activeTab === "consumers" && (
              <div className="h-full">
                <div className="bg-white border-b border-gray-200 px-4 lg:px-6 py-3">
                  <h3 className="text-lg font-medium">Consumer Groups</h3>
                </div>
                <div className="flex-1 overflow-auto p-4">
                  <div className="text-center py-8 text-gray-500">
                    No consumer groups found
                  </div>
                </div>
              </div>
            )}

            {activeTab === "settings" && (
              <div className="h-full">
                <div className="bg-white border-b border-gray-200 px-4 lg:px-6 py-3">
                  <div className="flex items-center justify-between">
                    <h3 className="text-lg font-medium">Topic Configuration</h3>
                    <Button
                      onClick={fetchTopicConfig}
                      disabled={configLoading}
                      variant="outline"
                      size="sm"
                    >
                      {configLoading ? "Loading..." : "Refresh"}
                    </Button>
                  </div>
                </div>
                <div className="flex-1 overflow-auto p-4">
                  {configLoading ? (
                    <div className="text-center py-8 text-gray-500">
                      Loading configuration...
                    </div>
                  ) : Object.keys(topicConfig).length > 0 ? (
                    <div className="space-y-4">
                      {Object.entries(topicConfig).map(([key, configItem]) => (
                        <div
                          key={key}
                          className="flex justify-between items-center py-3 px-4 bg-gray-50 rounded-lg border"
                        >
                          <div className="flex flex-col">
                            <div className="font-medium text-gray-900">
                              {key}
                            </div>
                            <div className="text-xs text-gray-500 mt-1">
                              {getConfigDescription(key)}
                            </div>
                            {configItem?.default === false && (
                              <div className="text-xs text-blue-600 mt-1">
                                Custom Value
                              </div>
                            )}
                          </div>
                          <div className="flex flex-col items-end">
                            <div className="text-muted-foreground font-mono text-sm bg-white px-3 py-1 rounded border">
                              {configItem?.value || "N/A"}
                            </div>
                            {configItem?.sensitive && (
                              <div className="text-xs text-red-500 mt-1">
                                Sensitive
                              </div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-center py-8 text-gray-500">
                      No configuration parameters available
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
