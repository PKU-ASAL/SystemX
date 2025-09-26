"use client";

import React, { useState, useEffect } from "react";
import { apiClient, type KafkaTopic } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import {
  Plus,
  Search,
  RefreshCw,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Database,
  Copy,
  Settings,
  BarChart3,
  HardDrive,
  Eye,
} from "lucide-react";
import { useRouter } from "next/navigation";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

interface SortConfig {
  field: string;
  direction: "asc" | "desc";
}

export function KafkaTopicsList() {
  const router = useRouter();
  const [topics, setTopics] = useState<KafkaTopic[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [showInternal, setShowInternal] = useState(false);
  const [selectedTopics, setSelectedTopics] = useState<string[]>([]);
  const [sortConfig, setSortConfig] = useState<SortConfig>({
    field: "name",
    direction: "asc",
  });

  const fetchTopics = async () => {
    try {
      setLoading(true);
      const response = await apiClient.getKafkaTopics();
      console.log("API Response:", response);
      console.log("Topics data:", response.data);

      // 根据实际 API 响应格式处理数据
      const topicsData = (response.data as any)?.topics || response.data || [];
      console.log("Processed topics:", topicsData);

      if (topicsData.length > 0) {
        console.log("First topic structure:", topicsData[0]);
      }

      setTopics(topicsData);
    } catch (error) {
      console.error("Failed to fetch topics:", error);
      setTopics([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTopics();
  }, []);

  // Filter and sort topics
  const filteredTopics = React.useMemo(() => {
    let filtered = topics.filter((topic) => {
      const matchesSearch = topic.name
        .toLowerCase()
        .includes(searchTerm.toLowerCase());
      const matchesInternal = showInternal || !topic.internal;
      return matchesSearch && matchesInternal;
    });

    // 排序
    filtered.sort((a, b) => {
      let aValue: any;
      let bValue: any;

      // 特殊处理不同字段的排序
      switch (sortConfig.field) {
        case "partitions":
          aValue =
            a.partition_count ||
            (typeof a.partitions === "number"
              ? a.partitions
              : a.partitions?.length || 0);
          bValue =
            b.partition_count ||
            (typeof b.partitions === "number"
              ? b.partitions
              : b.partitions?.length || 0);
          break;
        case "messages_total":
          aValue = a.messages_total || a.total_messages || a.messageCount || 0;
          bValue = b.messages_total || b.total_messages || b.messageCount || 0;
          break;
        case "bytes_total":
          aValue = a.bytes_total || a.total_size_bytes || a.segment_size || 0;
          bValue = b.bytes_total || b.total_size_bytes || b.segment_size || 0;
          break;
        case "messages_per_sec":
          aValue = a.messages_per_sec || a.messages_per_second || 0;
          bValue = b.messages_per_sec || b.messages_per_second || 0;
          break;
        case "bytes_per_sec":
          aValue = a.bytes_per_sec || 0;
          bValue = b.bytes_per_sec || 0;
          break;
        default:
          aValue = a[sortConfig.field as keyof KafkaTopic];
          bValue = b[sortConfig.field as keyof KafkaTopic];
      }

      // 处理 undefined 和 null 值
      if (aValue == null) aValue = 0;
      if (bValue == null) bValue = 0;

      if (typeof aValue === "string") {
        aValue = aValue.toLowerCase();
        bValue = bValue.toLowerCase();
      }

      if (aValue < bValue) {
        return sortConfig.direction === "asc" ? -1 : 1;
      }
      if (aValue > bValue) {
        return sortConfig.direction === "asc" ? 1 : -1;
      }
      return 0;
    });

    return filtered;
  }, [topics, searchTerm, showInternal, sortConfig]);

  const handleSort = (field: string) => {
    setSortConfig((prev) => ({
      field,
      direction:
        prev.field === field && prev.direction === "desc" ? "asc" : "desc",
    }));
  };

  const handleSelectTopic = (topicName: string) => {
    setSelectedTopics((prev) =>
      prev.includes(topicName)
        ? prev.filter((name) => name !== topicName)
        : [...prev, topicName]
    );
  };

  const handleSelectAll = () => {
    const allTopicNames = filteredTopics.map((t) => t.name);
    setSelectedTopics(
      selectedTopics.length === allTopicNames.length ? [] : allTopicNames
    );
  };

  const getSortIcon = (field: string) => {
    if (sortConfig.field !== field) {
      return <ArrowUpDown className="h-3.5 w-3.5 text-gray-400" />;
    }
    return sortConfig.direction === "asc" ? (
      <ArrowUp className="h-3.5 w-3.5 text-blue-600" />
    ) : (
      <ArrowDown className="h-3.5 w-3.5 text-blue-600" />
    );
  };

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
                        window.location.href = "/dashboard";
                      }}
                    >
                      Dashboard
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbPage>Topics</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                管理和监控 Kafka 集群中的所有 Topic 主题
              </p>
            </div>
            <Button
              onClick={fetchTopics}
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

          {/* 搜索框和控制面板 */}
          <div className="bg-white border-b border-gray-200 px-6 py-3">
            <div className="flex items-center gap-3 w-full">
              <div className="flex-1 min-w-0">
                <div className="relative">
                  <div className="flex items-stretch border border-gray-300 rounded-md bg-white shadow-sm hover:shadow-md transition-all focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500 h-9">
                    <div className="flex-1 relative">
                      <input
                        type="text"
                        className="w-full px-3 py-2 text-sm border-0 bg-transparent focus:outline-none focus:ring-0 placeholder-gray-500 pr-12 h-full"
                        placeholder="Search topics by name..."
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                      />
                      <div className="absolute right-3 top-1/2 transform -translate-y-1/2">
                        <Search className="h-4 w-4 text-gray-400" />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <div className="flex items-center space-x-2 flex-shrink-0">
                <Switch
                  id="show-internal"
                  checked={showInternal}
                  onCheckedChange={setShowInternal}
                />
                <label htmlFor="show-internal" className="text-sm font-medium">
                  Show Internal Topics
                </label>
              </div>
            </div>
          </div>

          {/* Topics Table */}
          <div className="flex-1 overflow-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-blue-600" />
                <span className="text-sm text-gray-600">Loading topics...</span>
              </div>
            ) : filteredTopics.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <Database className="h-8 w-8 mx-auto mb-3 text-gray-300" />
                <p className="text-sm font-medium text-gray-600">
                  No topics found
                </p>
                <p className="text-xs text-gray-500">
                  {searchTerm
                    ? "Try adjusting your search query or toggle internal topics"
                    : "No topics available"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-full">
                  <TableHeader className="bg-gray-50/80 sticky top-0 border-b border-gray-200">
                    <TableRow className="hover:bg-gray-50/80">
                      <TableHead className="w-10 px-3 py-2 text-left">
                        <Checkbox
                          checked={
                            selectedTopics.length === filteredTopics.length
                          }
                          onCheckedChange={handleSelectAll}
                          className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                        />
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[200px]"
                        onClick={() => handleSort("name")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Database className="h-3.5 w-3.5" />
                          Topic Name
                          {getSortIcon("name")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-center cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("partitions")}
                      >
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <Copy className="h-3.5 w-3.5" />
                          Partitions
                          {getSortIcon("partitions")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-center min-w-[120px]">
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <Settings className="h-3.5 w-3.5" />
                          Out of Sync
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-center cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[120px]"
                        onClick={() => handleSort("replication_factor")}
                      >
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <Copy className="h-3.5 w-3.5" />
                          Replication Factor
                          {getSortIcon("replication_factor")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-center cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[120px]"
                        onClick={() => handleSort("messages_total")}
                      >
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <BarChart3 className="h-3.5 w-3.5" />
                          Messages
                          {getSortIcon("messages_total")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-center min-w-[80px]">
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <HardDrive className="h-3.5 w-3.5" />
                          Size
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-center cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("messages_per_sec")}
                      >
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <BarChart3 className="h-3.5 w-3.5" />
                          Msg/sec
                          {getSortIcon("messages_per_sec")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-center cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("bytes_per_sec")}
                      >
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          <HardDrive className="h-3.5 w-3.5" />
                          Bytes/sec
                          {getSortIcon("bytes_per_sec")}
                        </div>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredTopics.map((topic) => (
                      <TableRow
                        key={topic.name}
                        className={`border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors cursor-pointer ${
                          selectedTopics.includes(topic.name)
                            ? "bg-blue-50/60"
                            : ""
                        }`}
                        onClick={() => {
                          router.push(
                            `/kafka/topics/${encodeURIComponent(topic.name)}`
                          );
                        }}
                      >
                        <TableCell
                          className="px-3 py-2"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Checkbox
                            checked={selectedTopics.includes(topic.name)}
                            onCheckedChange={() =>
                              handleSelectTopic(topic.name)
                            }
                            className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                          />
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <div className="flex items-center gap-2">
                            {topic.internal && (
                              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                Internal
                              </span>
                            )}
                            <span className="font-mono text-sm">
                              {topic.name}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center">
                          <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-blue-100/80 text-blue-700 border-blue-200/80">
                            {topic.partition_count ||
                              (typeof topic.partitions === "number"
                                ? topic.partitions
                                : topic.partitions?.length || 0)}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center">
                          <span
                            className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${
                              (topic as any).under_replicated_partitions > 0
                                ? "bg-red-100/80 text-red-700 border-red-200/80"
                                : "bg-green-100/80 text-green-700 border-green-200/80"
                            }`}
                          >
                            {(topic as any).under_replicated_partitions || 0}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center">
                          <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-purple-100/80 text-purple-700 border-purple-200/80">
                            {topic.replication_factor}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center font-mono text-sm">
                          {(() => {
                            // 优先使用实际 API 字段 messages_total
                            if (
                              topic.messages_total !== undefined &&
                              topic.messages_total !== null
                            ) {
                              return topic.messages_total.toLocaleString();
                            }

                            // 回退到详情接口字段 total_messages
                            if (
                              topic.total_messages !== undefined &&
                              topic.total_messages !== null
                            ) {
                              return topic.total_messages.toLocaleString();
                            }

                            // 回退到旧的字段
                            if (
                              topic.messageCount !== undefined &&
                              topic.messageCount !== null
                            ) {
                              return topic.messageCount.toLocaleString();
                            }

                            // 如果 partitions 是数组，计算总消息数
                            if (Array.isArray(topic.partitions)) {
                              const totalMessages = topic.partitions.reduce(
                                (sum: number, p: any) => {
                                  const messages =
                                    (p.offset_max || 0) - (p.offset_min || 0);
                                  return sum + Math.max(0, messages);
                                },
                                0
                              );
                              return totalMessages.toLocaleString();
                            }

                            return "0";
                          })()}
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center font-mono text-sm">
                          {(() => {
                            // 优先使用实际 API 字段 bytes_total_formatted
                            if (topic.bytes_total_formatted) {
                              return topic.bytes_total_formatted;
                            }

                            // 使用详情接口字段 total_size_formatted
                            if (topic.total_size_formatted) {
                              return topic.total_size_formatted;
                            }

                            // 使用 bytes_total 字段并格式化
                            if (
                              topic.bytes_total !== undefined &&
                              topic.bytes_total !== null &&
                              topic.bytes_total > 0
                            ) {
                              const bytes = topic.bytes_total;
                              if (bytes >= 1024 * 1024 * 1024) {
                                return `${(
                                  bytes /
                                  (1024 * 1024 * 1024)
                                ).toFixed(2)} GB`;
                              } else if (bytes >= 1024 * 1024) {
                                return `${(bytes / (1024 * 1024)).toFixed(
                                  2
                                )} MB`;
                              } else if (bytes >= 1024) {
                                return `${(bytes / 1024).toFixed(2)} KB`;
                              } else {
                                return `${bytes} B`;
                              }
                            }

                            // 使用 total_size_bytes 字段并格式化
                            if (
                              topic.total_size_bytes &&
                              topic.total_size_bytes > 0
                            ) {
                              const bytes = topic.total_size_bytes;
                              if (bytes >= 1024 * 1024 * 1024) {
                                return `${(
                                  bytes /
                                  (1024 * 1024 * 1024)
                                ).toFixed(2)} GB`;
                              } else if (bytes >= 1024 * 1024) {
                                return `${(bytes / (1024 * 1024)).toFixed(
                                  2
                                )} MB`;
                              } else if (bytes >= 1024) {
                                return `${(bytes / 1024).toFixed(2)} KB`;
                              } else {
                                return `${bytes} B`;
                              }
                            }

                            // 回退到旧的字段
                            if (
                              topic.size &&
                              typeof topic.size === "string" &&
                              topic.size !== "0 Bytes"
                            ) {
                              return topic.size;
                            }

                            return "0 B";
                          })()}
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center font-mono text-sm">
                          {(() => {
                            // 显示每秒消息数
                            const msgPerSec =
                              topic.messages_per_sec ||
                              topic.messages_per_second ||
                              0;
                            if (msgPerSec === 0) {
                              return "0";
                            }
                            // 如果数值很小，显示科学计数法或保留更多小数位
                            if (msgPerSec < 0.01) {
                              return msgPerSec.toExponential(2);
                            }
                            // 如果数值较大，使用千分位分隔符
                            if (msgPerSec >= 1000) {
                              return msgPerSec.toLocaleString(undefined, {
                                maximumFractionDigits: 1,
                              });
                            }
                            // 普通数值保留2位小数
                            return msgPerSec.toFixed(2);
                          })()}
                        </TableCell>
                        <TableCell className="px-3 py-2 text-center font-mono text-sm">
                          {(() => {
                            // 显示每秒字节数并格式化
                            const bytesPerSec = topic.bytes_per_sec || 0;
                            if (bytesPerSec === 0) {
                              return "0 B/s";
                            }

                            // 如果数值很小，显示科学计数法
                            if (bytesPerSec < 0.01) {
                              return `${bytesPerSec.toExponential(2)} B/s`;
                            }

                            // 格式化字节数
                            if (bytesPerSec >= 1024 * 1024 * 1024) {
                              return `${(
                                bytesPerSec /
                                (1024 * 1024 * 1024)
                              ).toFixed(2)} GB/s`;
                            } else if (bytesPerSec >= 1024 * 1024) {
                              return `${(bytesPerSec / (1024 * 1024)).toFixed(
                                2
                              )} MB/s`;
                            } else if (bytesPerSec >= 1024) {
                              return `${(bytesPerSec / 1024).toFixed(2)} KB/s`;
                            } else {
                              return `${bytesPerSec.toFixed(2)} B/s`;
                            }
                          })()}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
