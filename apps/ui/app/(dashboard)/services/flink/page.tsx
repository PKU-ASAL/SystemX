"use client";

import React, { useState, useEffect } from "react";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  CheckCircle,
  Server,
  Activity,
  Database,
  AlertTriangle,
  RefreshCw,
  Search,
  Play,
  Pause,
  Settings,
  Eye,
  Trash2,
  GitBranch,
  Clock,
  XCircle,
  Cpu,
  HardDrive,
  Zap,
  MoreHorizontal,
} from "lucide-react";

// 空的初始状态，将从 API 获取真实数据

const getJobStatusBadge = (status: string) => {
  switch (status) {
    case "RUNNING":
      return (
        <Badge className="bg-green-100 text-green-800 border-green-200">
          <Play className="h-3 w-3 mr-1" />
          运行中
        </Badge>
      );
    case "FINISHED":
      return (
        <Badge className="bg-blue-100 text-blue-800 border-blue-200">
          <CheckCircle className="h-3 w-3 mr-1" />
          已完成
        </Badge>
      );
    case "FAILED":
      return (
        <Badge className="bg-red-100 text-red-800 border-red-200">
          <XCircle className="h-3 w-3 mr-1" />
          失败
        </Badge>
      );
    case "CANCELED":
      return (
        <Badge className="bg-gray-100 text-gray-800 border-gray-200">
          <Pause className="h-3 w-3 mr-1" />
          已取消
        </Badge>
      );
    default:
      return (
        <Badge className="bg-yellow-100 text-yellow-800 border-yellow-200">
          <AlertTriangle className="h-3 w-3 mr-1" />
          未知
        </Badge>
      );
  }
};

export default function FlinkPage() {
  const [jobs, setJobs] = useState<any[]>([]);
  const [clusterInfo, setClusterInfo] = useState<any>({
    taskManagers: 0,
    availableSlots: 0,
    usedSlots: 0,
    totalJobs: 0,
    runningJobs: 0,
    failedJobs: 0,
    version: "1.18.1",
    uptime: "0h 0m",
    cpuUsage: 0,
    memoryUsage: 0,
    diskUsage: 0
  });
  const [taskManagers, setTaskManagers] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");

  const fetchData = async () => {
    try {
      setLoading(true);
      
      // 完全参照 Kafka 页面的 API 调用方式
      const [overviewResponse, jobsResponse, taskManagersResponse] = await Promise.all([
        fetch('/api/v1/services/flink/overview').then(res => res.json()).catch(() => ({ data: null })),
        fetch('/api/v1/services/flink/jobs').then(res => res.json()).catch(() => ({ data: null })),
        fetch('/api/v1/services/flink/taskmanagers/overview').then(res => res.json()).catch(() => ({ data: null })),
      ]);
      
      console.log('Flink API responses:', { overviewResponse, jobsResponse, taskManagersResponse });
      
      // 处理集群概览数据
      const overview = overviewResponse.data || {};
      const newClusterInfo = {
        taskManagers: overview.taskmanagers || 0,
        availableSlots: overview['slots-total'] || 0,
        usedSlots: (overview['slots-total'] || 0) - (overview['slots-available'] || 0),
        totalJobs: (overview['jobs-running'] || 0) + (overview['jobs-finished'] || 0) + (overview['jobs-cancelled'] || 0) + (overview['jobs-failed'] || 0),
        runningJobs: overview['jobs-running'] || 0,
        failedJobs: overview['jobs-failed'] || 0,
        version: overview['flink-version'] || "1.18.1",
        uptime: "运行中",
        cpuUsage: 0,
        memoryUsage: 0,
        diskUsage: 0
      };
      
      // 处理作业数据 - 只使用 API 返回的真实数据
      const jobsData = jobsResponse.data?.jobs || [];
      const processedJobs = jobsData.map((job: any) => ({
        id: job.id,
        name: job.name,
        description: `作业 ID: ${job.id.substring(0, 8)}...`,
        status: job.state,
        startTime: new Date(job['start-time']).toISOString(),
        duration: formatDuration(job.duration || 0),
        lastModification: new Date(job['last-modification']).toISOString(),
        endTime: job['end-time'] === -1 ? null : new Date(job['end-time']).toISOString()
      }));
      setJobs(processedJobs);
      
      // 处理 TaskManager 数据
      const tmData = taskManagersResponse.data || {};
      if (tmData.cpu && tmData.memory) {
        newClusterInfo.cpuUsage = (tmData.cpu.used / tmData.cpu.total) * 100;
        newClusterInfo.memoryUsage = (tmData.memory.used / tmData.memory.total) * 100;
        newClusterInfo.diskUsage = 45.8; // 暂时使用固定值
      }
      
      setClusterInfo(newClusterInfo);
      setTaskManagers(tmData.taskmanagers || []);
      
    } catch (error) {
      console.error("Failed to fetch Flink data:", error);
      setJobs([]);
      setTaskManagers([]);
    } finally {
      setLoading(false);
    }
  };

  const formatDuration = (ms: number) => {
    const hours = Math.floor(ms / (1000 * 60 * 60));
    const minutes = Math.floor((ms % (1000 * 60 * 60)) / (1000 * 60));
    return `${hours}h ${minutes}m`;
  };

  useEffect(() => {
    fetchData();
  }, []);

  const filteredJobs = jobs.filter(
    (job: any) =>
      job.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      job.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <Zap className="h-4 w-4 text-primary" />
              工作流管理
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              Flink 集群状态、作业管理和资源监控
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
              <CardDescription>TaskManager 节点</CardDescription>
              <CardTitle className="text-2xl font-semibold text-blue-600">
                {clusterInfo.taskManagers}
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
              <div className="text-muted-foreground">活跃的 TaskManager</div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>任务槽位</CardDescription>
              <CardTitle className="text-2xl font-semibold text-green-600">
                {clusterInfo.usedSlots}/{clusterInfo.availableSlots}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-green-100">
                  <Cpu className="h-5 w-5 text-green-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                槽位使用情况 <Cpu className="size-4" />
              </div>
              <div className="text-muted-foreground">
                使用率: {((clusterInfo.usedSlots / clusterInfo.availableSlots) * 100).toFixed(1)}%
              </div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>运行作业</CardDescription>
              <CardTitle className="text-2xl font-semibold text-purple-600">
                {clusterInfo.runningJobs}/{clusterInfo.totalJobs}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-purple-100">
                  <Play className="h-5 w-5 text-purple-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                作业状态 <Play className="size-4" />
              </div>
              <div className="text-muted-foreground">正在运行的作业数</div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardDescription>集群运行时间</CardDescription>
              <CardTitle className="text-2xl font-semibold text-orange-600">
                {clusterInfo.uptime}
              </CardTitle>
              <CardAction>
                <div className="p-2 rounded-full bg-orange-100">
                  <Clock className="h-5 w-5 text-orange-600" />
                </div>
              </CardAction>
            </CardHeader>
            <CardFooter className="flex-col items-start gap-1.5 text-sm">
              <div className="line-clamp-1 flex gap-2 font-medium">
                运行时间 <Clock className="size-4" />
              </div>
              <div className="text-muted-foreground">集群持续运行时间</div>
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
            placeholder="搜索作业或资源..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>

      {/* Tabs 内容 */}
      <div className="flex-1 overflow-hidden">
        <Tabs defaultValue="jobs" className="h-full flex flex-col">
          <div className="px-4 lg:px-6 py-3 border-b border-border bg-muted/20">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="jobs" className="flex-1">作业管理</TabsTrigger>
              <TabsTrigger value="resources" className="flex-1">资源监控</TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="jobs" className="flex-1 overflow-auto mt-0">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-primary" />
                <span className="text-sm text-muted-foreground">
                  加载作业信息...
                </span>
              </div>
            ) : filteredJobs.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                <GitBranch className="h-8 w-8 mx-auto mb-3 text-muted-foreground/50" />
                <p className="text-sm font-medium text-foreground">
                  未找到作业
                </p>
                <p className="text-xs text-muted-foreground">
                  {searchTerm ? "尝试调整搜索条件" : "没有运行的 Flink 作业"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>作业名称</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>运行时间</TableHead>
                      <TableHead>开始时间</TableHead>
                      <TableHead>最后修改</TableHead>
                      <TableHead>操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredJobs.map((job) => (
                      <TableRow key={job.id}>
                        <TableCell>
                          <div>
                            <div className="font-medium text-sm">{job.name}</div>
                            <div className="text-xs text-muted-foreground font-mono">
                              {job.id.substring(0, 8)}...
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          {getJobStatusBadge(job.status)}
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {job.duration}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {new Date(job.startTime).toLocaleString()}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {new Date(job.lastModification).toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem>
                                <Eye className="mr-2 h-4 w-4" />
                                查看详情
                              </DropdownMenuItem>
                              <DropdownMenuItem>
                                {job.status === "RUNNING" ? (
                                  <>
                                    <Pause className="mr-2 h-4 w-4" />
                                    暂停作业
                                  </>
                                ) : (
                                  <>
                                    <Play className="mr-2 h-4 w-4" />
                                    启动作业
                                  </>
                                )}
                              </DropdownMenuItem>
                              <DropdownMenuItem>
                                <Settings className="mr-2 h-4 w-4" />
                                作业配置
                              </DropdownMenuItem>
                              <DropdownMenuItem className="text-red-600">
                                <Trash2 className="mr-2 h-4 w-4" />
                                删除作业
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </TabsContent>

          <TabsContent value="resources" className="flex-1 overflow-auto mt-0">
            <div className="p-4 lg:p-6 space-y-6">
              {/* 资源使用情况 */}
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <Card>
                  <CardHeader>
                    <CardDescription>CPU 使用率</CardDescription>
                    <CardTitle className="text-2xl font-semibold text-blue-600">
                      {clusterInfo.cpuUsage.toFixed(1)}%
                    </CardTitle>
                    <CardAction>
                      <div className="p-2 rounded-full bg-blue-100">
                        <Cpu className="h-5 w-5 text-blue-600" />
                      </div>
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                        style={{ width: `${clusterInfo.cpuUsage}%` }}
                      ></div>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardDescription>内存使用率</CardDescription>
                    <CardTitle className="text-2xl font-semibold text-green-600">
                      {clusterInfo.memoryUsage.toFixed(1)}%
                    </CardTitle>
                    <CardAction>
                      <div className="p-2 rounded-full bg-green-100">
                        <Database className="h-5 w-5 text-green-600" />
                      </div>
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className="bg-green-600 h-2 rounded-full transition-all duration-300"
                        style={{ width: `${clusterInfo.memoryUsage}%` }}
                      ></div>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardDescription>磁盘使用率</CardDescription>
                    <CardTitle className="text-2xl font-semibold text-purple-600">
                      {clusterInfo.diskUsage.toFixed(1)}%
                    </CardTitle>
                    <CardAction>
                      <div className="p-2 rounded-full bg-purple-100">
                        <HardDrive className="h-5 w-5 text-purple-600" />
                      </div>
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className="bg-purple-600 h-2 rounded-full transition-all duration-300"
                        style={{ width: `${clusterInfo.diskUsage}%` }}
                      ></div>
                    </div>
                  </CardContent>
                </Card>
              </div>

              {/* 集群详细信息 */}
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">集群信息</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">Flink 版本</span>
                      <span className="text-sm font-mono">{clusterInfo.version}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">运行时间</span>
                      <span className="text-sm font-mono">{clusterInfo.uptime}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">TaskManager 数量</span>
                      <span className="text-sm font-mono">{clusterInfo.taskManagers}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">可用槽位</span>
                      <span className="text-sm font-mono">
                        {clusterInfo.usedSlots}/{clusterInfo.availableSlots}
                      </span>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">作业统计</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">总作业数</span>
                      <span className="text-sm font-mono">{clusterInfo.totalJobs}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">运行中</span>
                      <span className="text-sm font-mono text-green-600">{clusterInfo.runningJobs}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">失败作业</span>
                      <span className="text-sm font-mono text-red-600">{clusterInfo.failedJobs}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-muted-foreground">成功率</span>
                      <span className="text-sm font-mono">
                        {(((clusterInfo.totalJobs - clusterInfo.failedJobs) / clusterInfo.totalJobs) * 100).toFixed(1)}%
                      </span>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
