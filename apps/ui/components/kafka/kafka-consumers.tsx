"use client";

import React, { useState, useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { apiClient, type KafkaConsumerGroup } from "@/lib/api";
import {
  RefreshCw,
  Users,
  Search,
  Eye,
  Activity,
  Clock,
  TrendingUp,
  Server,
  Database,
  AlertTriangle,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Network,
  Settings,
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

export function KafkaConsumers() {
  const [consumerGroups, setConsumerGroups] = useState<KafkaConsumerGroup[]>(
    []
  );
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedGroup, setSelectedGroup] = useState<KafkaConsumerGroup | null>(
    null
  );
  const [detailDialogOpen, setDetailDialogOpen] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [selectedGroups, setSelectedGroups] = useState<string[]>([]);
  const [sortConfig, setSortConfig] = useState<SortConfig>({
    field: "group_id",
    direction: "asc",
  });

  const fetchConsumerGroups = async () => {
    try {
      setLoading(true);
      const response = await apiClient.getKafkaConsumerGroups();
      setConsumerGroups(response.data);
      setLastUpdated(new Date());
    } catch (error) {
      console.error("Failed to fetch consumer groups:", error);
      setConsumerGroups([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchConsumerGroups();
  }, []);

  // Filter and sort consumer groups
  const filteredGroups = React.useMemo(() => {
    let filtered = consumerGroups.filter((group) =>
      group.group_id.toLowerCase().includes(searchTerm.toLowerCase())
    );

    // 排序
    filtered.sort((a, b) => {
      let aValue: any = a[sortConfig.field as keyof KafkaConsumerGroup];
      let bValue: any = b[sortConfig.field as keyof KafkaConsumerGroup];

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
  }, [consumerGroups, searchTerm, sortConfig]);

  const handleSort = (field: string) => {
    setSortConfig((prev) => ({
      field,
      direction:
        prev.field === field && prev.direction === "desc" ? "asc" : "desc",
    }));
  };

  const handleSelectGroup = (groupId: string) => {
    setSelectedGroups((prev) =>
      prev.includes(groupId)
        ? prev.filter((id) => id !== groupId)
        : [...prev, groupId]
    );
  };

  const handleSelectAll = () => {
    const allGroupIds = filteredGroups.map((g) => g.group_id);
    setSelectedGroups(
      selectedGroups.length === allGroupIds.length ? [] : allGroupIds
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

  const getStateBadge = (state: string) => {
    const stateMap: Record<
      string,
      {
        variant: "default" | "secondary" | "destructive" | "outline";
        label: string;
      }
    > = {
      Stable: { variant: "default", label: "稳定" },
      PreparingRebalance: { variant: "secondary", label: "准备重平衡" },
      CompletingRebalance: { variant: "secondary", label: "完成重平衡" },
      Dead: { variant: "destructive", label: "已停止" },
      Empty: { variant: "outline", label: "空闲" },
    };

    const stateInfo = stateMap[state] || {
      variant: "outline",
      label: state,
    };

    return <Badge variant={stateInfo.variant}>{stateInfo.label}</Badge>;
  };

  const getLagBadge = (lag: number) => {
    if (lag === 0) {
      return <Badge variant="default">无延迟</Badge>;
    } else if (lag < 1000) {
      return <Badge variant="secondary">{lag}</Badge>;
    } else if (lag < 10000) {
      return <Badge variant="outline">{lag.toLocaleString()}</Badge>;
    } else {
      return <Badge variant="destructive">{lag.toLocaleString()}</Badge>;
    }
  };

  const showGroupDetail = async (groupId: string) => {
    try {
      const response = await apiClient.getKafkaConsumerGroup(groupId);
      setSelectedGroup(response.data);
      setDetailDialogOpen(true);
    } catch (error) {
      console.error("Failed to fetch group detail:", error);
    }
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
                    <BreadcrumbPage>Consumer Groups</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                监控 Kafka 消费者组的状态、延迟和成员信息
              </p>
            </div>
            <div className="flex items-center gap-2">
              {lastUpdated && (
                <div className="flex items-center gap-1 text-sm text-gray-500">
                  <Clock className="h-4 w-4" />
                  {lastUpdated.toLocaleTimeString()}
                </div>
              )}
              <Button
                onClick={fetchConsumerGroups}
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

          {/* Metrics Cards */}
          <div className="bg-white px-6 py-4 border-b border-gray-200">
            <div className="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs md:grid-cols-2 lg:grid-cols-4">
              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>总消费者组</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-blue-600">
                    {consumerGroups.length}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-blue-100">
                      <Users className="h-5 w-5 text-blue-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    集群中的消费者组数量 <Users className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    Kafka 消费者组总数
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>活跃组数</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-green-600">
                    {consumerGroups.filter((g) => g.state === "Stable").length}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-green-100">
                      <Activity className="h-5 w-5 text-green-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    稳定运行的消费者组 <Activity className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    正常消费数据的组数
                  </div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>总成员数</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-purple-600">
                    {consumerGroups.reduce(
                      (sum, group) => sum + group.members.length,
                      0
                    )}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-purple-100">
                      <Server className="h-5 w-5 text-purple-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    所有消费者实例总数 <Server className="size-4" />
                  </div>
                  <div className="text-muted-foreground">消费者客户端数量</div>
                </CardFooter>
              </Card>

              <Card className="@container/card">
                <CardHeader>
                  <CardDescription>总延迟</CardDescription>
                  <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl text-orange-600">
                    {consumerGroups
                      .reduce((sum, group) => sum + group.lag, 0)
                      .toLocaleString()}
                  </CardTitle>
                  <CardAction>
                    <div className="p-2 rounded-full bg-orange-100">
                      <TrendingUp className="h-5 w-5 text-orange-600" />
                    </div>
                  </CardAction>
                </CardHeader>
                <CardFooter className="flex-col items-start gap-1.5 text-sm">
                  <div className="line-clamp-1 flex gap-2 font-medium">
                    消费延迟消息总数 <TrendingUp className="size-4" />
                  </div>
                  <div className="text-muted-foreground">
                    需要处理的积压消息
                  </div>
                </CardFooter>
              </Card>
            </div>
          </div>

          {/* 搜索框 */}
          <div className="bg-white border-b border-gray-200 px-6 py-3">
            <div className="flex items-center gap-3 w-full">
              <div className="flex-1 min-w-0">
                <div className="relative">
                  <div className="flex items-stretch border border-gray-300 rounded-md bg-white shadow-sm hover:shadow-md transition-all focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500 h-9">
                    <div className="flex-1 relative">
                      <input
                        type="text"
                        className="w-full px-3 py-2 text-sm border-0 bg-transparent focus:outline-none focus:ring-0 placeholder-gray-500 pr-12 h-full"
                        placeholder="Search consumer groups by ID..."
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
            </div>
          </div>

          {/* Consumer Groups Table */}
          <div className="flex-1 overflow-auto">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-blue-600" />
                <span className="text-sm text-gray-600">
                  Loading consumer groups...
                </span>
              </div>
            ) : filteredGroups.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <Users className="h-8 w-8 mx-auto mb-3 text-gray-300" />
                <p className="text-sm font-medium text-gray-600">
                  No consumer groups found
                </p>
                <p className="text-xs text-gray-500">
                  {searchTerm
                    ? "Try adjusting your search query"
                    : "No consumer groups available"}
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
                            selectedGroups.length === filteredGroups.length
                          }
                          onCheckedChange={handleSelectAll}
                          className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                        />
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[200px]"
                        onClick={() => handleSort("group_id")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Users className="h-3.5 w-3.5" />
                          Group ID
                          {getSortIcon("group_id")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("state")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Activity className="h-3.5 w-3.5" />
                          状态
                          {getSortIcon("state")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[100px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Settings className="h-3.5 w-3.5" />
                          协议类型
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[80px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Server className="h-3.5 w-3.5" />
                          成员数
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[150px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Database className="h-3.5 w-3.5" />
                          订阅 Topic
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("lag")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <TrendingUp className="h-3.5 w-3.5" />
                          总延迟
                          {getSortIcon("lag")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[120px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Network className="h-3.5 w-3.5" />
                          协调器
                        </div>
                      </TableHead>
                      <TableHead className="w-10 px-3 py-2 text-left">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          Actions
                        </div>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredGroups.map((group) => (
                      <TableRow
                        key={group.group_id}
                        className={`border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors ${
                          selectedGroups.includes(group.group_id)
                            ? "bg-blue-50/60"
                            : ""
                        }`}
                      >
                        <TableCell className="px-3 py-2">
                          <Checkbox
                            checked={selectedGroups.includes(group.group_id)}
                            onCheckedChange={() =>
                              handleSelectGroup(group.group_id)
                            }
                            className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                          />
                        </TableCell>
                        <TableCell className="px-3 py-2 font-mono text-sm">
                          {group.group_id}
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          {getStateBadge(group.state)}
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                            {group.protocol_type}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium border bg-blue-100/80 text-blue-700 border-blue-200/80">
                            {group.members.length}
                          </span>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <div className="flex flex-wrap gap-1">
                            {group.topics.slice(0, 2).map((topic) => (
                              <span
                                key={topic}
                                className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-purple-100/80 text-purple-700 border-purple-200/80"
                              >
                                {topic}
                              </span>
                            ))}
                            {group.topics.length > 2 && (
                              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                                +{group.topics.length - 2}
                              </span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          {getLagBadge(group.lag)}
                        </TableCell>
                        <TableCell className="px-3 py-2 font-mono text-sm">
                          {group.coordinator.host}:{group.coordinator.port}
                        </TableCell>
                        <TableCell className="px-3 py-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => showGroupDetail(group.group_id)}
                            className="h-6 w-6 p-0 hover:bg-gray-100/80"
                          >
                            <Eye className="h-3.5 w-3.5" />
                          </Button>
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

      {/* Consumer Group 详情对话框 */}
      <Dialog open={detailDialogOpen} onOpenChange={setDetailDialogOpen}>
        <DialogContent className="sm:max-w-[800px] max-h-[80vh]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Users className="h-5 w-5" />
              Consumer Group 详情
            </DialogTitle>
            <DialogDescription>
              {selectedGroup?.group_id} 的详细成员和分区分配信息
            </DialogDescription>
          </DialogHeader>
          <div className="py-4 max-h-96 overflow-auto">
            {selectedGroup && (
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <h4 className="font-semibold mb-2">基本信息</h4>
                    <div className="space-y-2 text-sm">
                      <div>状态: {getStateBadge(selectedGroup.state)}</div>
                      <div>协议: {selectedGroup.protocol}</div>
                      <div>成员数: {selectedGroup.members.length}</div>
                      <div>总延迟: {getLagBadge(selectedGroup.lag)}</div>
                    </div>
                  </div>
                  <div>
                    <h4 className="font-semibold mb-2">订阅 Topics</h4>
                    <div className="flex flex-wrap gap-1">
                      {selectedGroup.topics.map((topic) => (
                        <Badge
                          key={topic}
                          variant="outline"
                          className="text-xs"
                        >
                          {topic}
                        </Badge>
                      ))}
                    </div>
                  </div>
                </div>

                <div>
                  <h4 className="font-semibold mb-2">成员列表</h4>
                  <div className="border rounded-lg">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Member ID</TableHead>
                          <TableHead>Client ID</TableHead>
                          <TableHead>Host</TableHead>
                          <TableHead>分配分区</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {selectedGroup.members.map((member) => (
                          <TableRow key={member.member_id}>
                            <TableCell className="font-mono text-xs">
                              {member.member_id.substring(0, 16)}...
                            </TableCell>
                            <TableCell>{member.client_id}</TableCell>
                            <TableCell>{member.client_host}</TableCell>
                            <TableCell>
                              <div className="flex flex-wrap gap-1">
                                {member.assignments.map((assignment, idx) => (
                                  <Badge
                                    key={idx}
                                    variant="outline"
                                    className="text-xs"
                                  >
                                    {assignment.topic}:{assignment.partition}
                                  </Badge>
                                ))}
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </div>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
