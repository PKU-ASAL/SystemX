"use client";

import React, { useState, useEffect } from "react";
import { apiClient, type KafkaBroker } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  CheckCircle,
  Server,
  Activity,
  Database,
  AlertTriangle,
  RefreshCw,
  Search,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  HardDrive,
  Cpu,
  Network,
} from "lucide-react";
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

export function KafkaBrokersList() {
  const [brokers, setBrokers] = useState<KafkaBroker[]>([]);
  const [clusterStats, setClusterStats] = useState<any>({});
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedBrokers, setSelectedBrokers] = useState<number[]>([]);
  const [sortConfig, setSortConfig] = useState<SortConfig>({
    field: "id",
    direction: "asc",
  });

  const fetchData = async () => {
    try {
      setLoading(true);
      const [brokersResponse, clusterResponse] = await Promise.all([
        apiClient.getKafkaBrokers(),
        apiClient.getKafkaClusterInfo(),
      ]);
      setBrokers(brokersResponse.data);
      setClusterStats(clusterResponse.data);
    } catch (error) {
      console.error("Failed to fetch brokers data:", error);
      setBrokers([]);
      setClusterStats({});
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  // 过滤和排序逻辑
  const filteredBrokers = React.useMemo(() => {
    let filtered = brokers.filter(
      (broker) =>
        broker.host.toLowerCase().includes(searchTerm.toLowerCase()) ||
        broker.id.toString().includes(searchTerm)
    );

    // 排序
    filtered.sort((a, b) => {
      let aValue: any;
      let bValue: any;

      // 处理嵌套字段
      if (sortConfig.field === "disk_usage") {
        aValue = a.disk_usage.usage_percentage;
        bValue = b.disk_usage.usage_percentage;
      } else {
        aValue = a[sortConfig.field as keyof KafkaBroker];
        bValue = b[sortConfig.field as keyof KafkaBroker];
      }

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
  }, [brokers, searchTerm, sortConfig]);

  const handleSort = (field: string) => {
    setSortConfig((prev) => ({
      field,
      direction:
        prev.field === field && prev.direction === "desc" ? "asc" : "desc",
    }));
  };

  const handleSelectBroker = (brokerId: number) => {
    setSelectedBrokers((prev) =>
      prev.includes(brokerId)
        ? prev.filter((id) => id !== brokerId)
        : [...prev, brokerId]
    );
  };

  const handleSelectAll = () => {
    const allBrokerIds = filteredBrokers.map((b) => b.id);
    setSelectedBrokers(
      selectedBrokers.length === allBrokerIds.length ? [] : allBrokerIds
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

  // 从集群数据中提取信息，处理数组结构
  const clusterInfo = clusterStats?.data?.[0] || {};
  const {
    broker_count: brokerCount = 1,
    active_controllers: activeControllers = 1,
    version = "3.4-IV0",
    online_partition_count: onlinePartitionCount = 0,
    partition_stats = {},
    replica_stats = {},
  } = clusterInfo;

  const {
    offline_partitions: offlinePartitionCount = 0,
    under_replicated_partitions: underReplicatedPartitionCount = 0,
  } = partition_stats;

  const {
    in_sync_replicas: inSyncReplicasCount = 0,
    out_of_sync_replicas: outOfSyncReplicasCount = 0,
  } = replica_stats;

  const replicas = (inSyncReplicasCount ?? 0) + (outOfSyncReplicasCount ?? 0);
  const areAllInSync = inSyncReplicasCount && replicas === inSyncReplicasCount;
  const partitionIsOffline = offlinePartitionCount && offlinePartitionCount > 0;
  const isActiveControllerUnKnown = typeof activeControllers === "undefined";

  return (
    <div className="@container/main flex flex-1 flex-col overflow-hidden">
      <div className="flex h-full bg-background">
        {/* 主内容区域 */}
        <div className="flex-1 bg-background flex flex-col min-w-0">
          {/* Header */}
          <div className="flex items-center justify-between px-4 lg:px-6 py-4 border-b border-border">
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
                    <BreadcrumbPage>Brokers</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                管理和监控 Kafka 集群中的所有 Broker 节点
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

          {/* Metrics Cards */}
          <div className="bg-background px-6 py-4 border-b border-border">
            <div className="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs md:grid-cols-2 lg:grid-cols-4">
              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Broker Count</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-blue-600">
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
                    集群中的 Broker 节点数 <Server className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    Kafka 集群节点总数
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Active Controller</CardDescription>
                  <CardTitle
                    className={`text-2xl font-semibold tabular-nums @[250px]/card:text-3xl ${
                      isActiveControllerUnKnown
                        ? "text-red-600"
                        : "text-green-600"
                    }`}
                  >
                    {isActiveControllerUnKnown
                      ? "No Active Controller"
                      : activeControllers}
                  </CardTitle>
                  <CardAction>
                    <div
                      className={`p-2 rounded-full ${
                        isActiveControllerUnKnown
                          ? "bg-red-100"
                          : "bg-green-100"
                      }`}
                    >
                      {isActiveControllerUnKnown ? (
                        <AlertTriangle className="h-5 w-5 text-red-600" />
                      ) : (
                        <CheckCircle className="h-5 w-5 text-green-600" />
                      )}
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    控制器节点状态 <Activity className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    集群控制器节点数量
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Online Partitions</CardDescription>
                  <CardTitle
                    className={`text-2xl font-semibold tabular-nums @[250px]/card:text-3xl ${
                      partitionIsOffline ? "text-red-600" : "text-green-600"
                    }`}
                  >
                    {onlinePartitionCount}
                    <span className="text-sm text-gray-500 ml-1">
                      of {onlinePartitionCount + offlinePartitionCount}
                    </span>
                  </CardTitle>
                  <CardAction>
                    <div
                      className={`p-2 rounded-full ${
                        partitionIsOffline ? "bg-red-100" : "bg-green-100"
                      }`}
                    >
                      <Database
                        className={`h-5 w-5 ${
                          partitionIsOffline ? "text-red-600" : "text-green-600"
                        }`}
                      />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    在线分区数量 <Database className="size-4" />
                  </div>
                  <div className="text-muted-foreground">正常工作的分区</div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>Version</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-purple-600">
                    {version}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-purple-100">
                      <Activity className="h-5 w-5 text-purple-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    Kafka 版本信息 <Activity className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    当前运行的 Kafka 版本
                  </div>
                </CardFooter>
              </Card>
            </div>
          </div>

          {/* 搜索框 */}
          <div className="bg-background border-b border-border px-6 py-3">
            <div className="flex items-center gap-3 w-full">
              <div className="flex-1 min-w-0">
                <div className="relative">
                  <div className="flex items-stretch border border-input rounded-md bg-background shadow-sm hover:shadow-md transition-all focus-within:ring-2 focus-within:ring-ring focus-within:border-ring h-9">
                    <div className="flex-1 relative">
                      <input
                        type="text"
                        className="w-full px-3 py-2 text-sm border-0 bg-transparent focus:outline-none focus:ring-0 placeholder-muted-foreground pr-12 h-full text-foreground"
                        placeholder="Search brokers by ID or host..."
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                      />
                      <div className="absolute right-3 top-1/2 transform -translate-y-1/2">
                        <Search className="h-4 w-4 text-muted-foreground" />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Brokers Table */}
          <div className="flex-1 overflow-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-primary" />
                <span className="text-sm text-muted-foreground">
                  Loading brokers...
                </span>
              </div>
            ) : filteredBrokers.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                <Server className="h-8 w-8 mx-auto mb-3 text-muted-foreground/50" />
                <p className="text-sm font-medium text-foreground">
                  No brokers found
                </p>
                <p className="text-xs text-muted-foreground">
                  {searchTerm
                    ? "Try adjusting your search query"
                    : "No brokers are online"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-full">
                  <TableHeader className="bg-muted/50 sticky top-0 border-b border-border">
                    <TableRow className="hover:bg-muted/80">
                      <TableHead className="w-10 px-3 py-2 text-left">
                        <Checkbox
                          checked={
                            selectedBrokers.length === filteredBrokers.length
                          }
                          onCheckedChange={handleSelectAll}
                          className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                        />
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-muted/60 transition-colors min-w-[100px]"
                        onClick={() => handleSort("id")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Server className="h-3.5 w-3.5" />
                          Broker ID
                          {getSortIcon("id")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-muted/60 transition-colors min-w-[120px]"
                        onClick={() => handleSort("disk_usage")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <HardDrive className="h-3.5 w-3.5" />
                          Disk Usage
                          {getSortIcon("disk_usage")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-muted/60 transition-colors min-w-[120px]"
                        onClick={() => handleSort("partitions_skew")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Cpu className="h-3.5 w-3.5" />
                          Partitions Skew
                          {getSortIcon("partitions_skew")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[80px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Activity className="h-3.5 w-3.5" />
                          Leaders
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[100px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Activity className="h-3.5 w-3.5" />
                          Leader Skew
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[120px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Database className="h-3.5 w-3.5" />
                          Broker Partitions
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[60px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Network className="h-3.5 w-3.5" />
                          Port
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-muted/60 transition-colors min-w-[120px]"
                        onClick={() => handleSort("host")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                          <Server className="h-3.5 w-3.5" />
                          Host
                          {getSortIcon("host")}
                        </div>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredBrokers.map((broker) => (
                      <TableRow
                        key={broker.id}
                        className={`border-b border-border/60 hover:bg-muted/50 transition-colors ${
                          selectedBrokers.includes(broker.id)
                            ? "bg-primary/10"
                            : ""
                        }`}
                      >
                        <TableCell className="px-3 py-2">
                          <Checkbox
                            checked={selectedBrokers.includes(broker.id)}
                            onCheckedChange={() =>
                              handleSelectBroker(broker.id)
                            }
                            className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                          />
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-sm">
                              {broker.id}
                            </span>
                            {broker.controller && (
                              <div className="flex items-center">
                                <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-green-100/80 text-green-700 border-green-200/80">
                                  Controller
                                </span>
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <div className="text-sm">
                            <div className="font-mono">
                              {broker.disk_usage.usage_percentage > 0
                                ? `${broker.disk_usage.usage_percentage.toFixed(
                                    1
                                  )}%`
                                : "N/A"}
                            </div>
                            {broker.segment_count > 0 && (
                              <div className="text-xs text-gray-500">
                                {broker.segment_count} segments (
                                {broker.segment_size})
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <span
                            className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${
                              broker.partitions_skew >= 20
                                ? "bg-red-100 text-red-800 border-red-200"
                                : broker.partitions_skew >= 10
                                ? "bg-yellow-100 text-yellow-800 border-yellow-200"
                                : "bg-gray-100 text-gray-800 border-gray-200"
                            }`}
                          >
                            {broker.partitions_skew !== undefined
                              ? `${broker.partitions_skew.toFixed(2)}%`
                              : "-"}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2 text-sm">
                          {broker.partitions_leader || "-"}
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <span
                            className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${
                              broker.leaders_skew >= 20
                                ? "bg-red-100 text-red-800 border-red-200"
                                : broker.leaders_skew >= 10
                                ? "bg-yellow-100 text-yellow-800 border-yellow-200"
                                : "bg-gray-100 text-gray-800 border-gray-200"
                            }`}
                          >
                            {broker.leaders_skew !== undefined
                              ? `${broker.leaders_skew.toFixed(2)}%`
                              : "-"}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <div className="text-sm">
                            <div className="flex items-center gap-1">
                              <span
                                className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${
                                  broker.out_of_sync_partitions > 0
                                    ? "bg-red-100 text-red-800 border-red-200"
                                    : broker.in_sync_partitions > 0
                                    ? "bg-green-100 text-green-800 border-green-200"
                                    : "bg-gray-100 text-gray-800 border-gray-200"
                                }`}
                              >
                                {broker.in_sync_partitions}
                              </span>
                              {broker.out_of_sync_partitions > 0 && (
                                <span className="text-xs text-red-600">
                                  +{broker.out_of_sync_partitions} out-of-sync
                                </span>
                              )}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-2 font-mono text-sm">
                          {broker.port}
                        </TableCell>
                        <TableCell className="px-3 py-2 font-mono text-sm">
                          {broker.host}
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
