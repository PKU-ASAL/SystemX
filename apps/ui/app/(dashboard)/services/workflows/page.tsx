"use client";

import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Search,
  RefreshCw,
  Play,
  Pause,
  Settings,
  Eye,
  Trash2,
  GitBranch,
  Clock,
  CheckCircle,
  XCircle,
  AlertCircle,
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

// Mock data for workflows
const mockWorkflows = [
  {
    id: "wf-001",
    name: "实时威胁检测",
    description: "基于 Vector 的实时日志分析，检测潜在的安全威胁",
    status: "running",
    kafkaTopic: "sysarmor-agentless-748154f5",
    vectorConfig: "threat-detection.toml",
    lastRun: "2025-08-07T20:30:00Z",
    createdAt: "2025-08-01T10:00:00Z",
    processedEvents: 15420,
    alertsGenerated: 23,
  },
  {
    id: "wf-002",
    name: "用户行为分析",
    description: "分析用户登录模式和异常行为",
    status: "paused",
    kafkaTopic: "sysarmor-agentless-a1b2c3d4",
    vectorConfig: "user-behavior.toml",
    lastRun: "2025-08-07T18:15:00Z",
    createdAt: "2025-08-02T14:30:00Z",
    processedEvents: 8750,
    alertsGenerated: 5,
  },
  {
    id: "wf-003",
    name: "网络流量监控",
    description: "监控异常网络连接和数据传输",
    status: "error",
    kafkaTopic: "sysarmor-host-web-server-01",
    vectorConfig: "network-monitor.toml",
    lastRun: "2025-08-07T16:45:00Z",
    createdAt: "2025-08-03T09:20:00Z",
    processedEvents: 3200,
    alertsGenerated: 12,
  },
];

const getStatusBadge = (status: string) => {
  switch (status) {
    case "running":
      return (
        <Badge className="bg-green-100 text-green-800 border-green-200">
          <CheckCircle className="h-3 w-3 mr-1" />
          运行中
        </Badge>
      );
    case "paused":
      return (
        <Badge className="bg-yellow-100 text-yellow-800 border-yellow-200">
          <Pause className="h-3 w-3 mr-1" />
          已暂停
        </Badge>
      );
    case "error":
      return (
        <Badge className="bg-red-100 text-red-800 border-red-200">
          <XCircle className="h-3 w-3 mr-1" />
          错误
        </Badge>
      );
    default:
      return (
        <Badge className="bg-gray-100 text-gray-800 border-gray-200">
          <AlertCircle className="h-3 w-3 mr-1" />
          未知
        </Badge>
      );
  }
};

export default function WorkflowsPage() {
  const router = useRouter();
  const [searchTerm, setSearchTerm] = useState("");
  const [workflows] = useState(mockWorkflows);

  const filteredWorkflows = workflows.filter(
    (workflow) =>
      workflow.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      workflow.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

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
                    <BreadcrumbPage>分析工作流</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                管理基于 Vector 的 Kafka Topic 数据分析工作流
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => router.push("/workflows/templates")}
              >
                <Settings className="h-4 w-4 mr-2" />
                模板管理
              </Button>
            </div>
          </div>

          {/* 统计卡片 */}
          <div className="px-4 lg:px-6 py-4 border-b">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    总工作流数
                  </CardTitle>
                  <GitBranch className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{workflows.length}</div>
                  <p className="text-xs text-muted-foreground">
                    活跃的分析工作流
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">运行中</CardTitle>
                  <Play className="h-4 w-4 text-green-600" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-green-600">
                    {workflows.filter((w) => w.status === "running").length}
                  </div>
                  <p className="text-xs text-muted-foreground">正在处理数据</p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    处理事件数
                  </CardTitle>
                  <CheckCircle className="h-4 w-4 text-blue-600" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-blue-600">
                    {workflows
                      .reduce((sum, w) => sum + w.processedEvents, 0)
                      .toLocaleString()}
                  </div>
                  <p className="text-xs text-muted-foreground">累计处理事件</p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    生成告警数
                  </CardTitle>
                  <AlertCircle className="h-4 w-4 text-orange-600" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-orange-600">
                    {workflows.reduce((sum, w) => sum + w.alertsGenerated, 0)}
                  </div>
                  <p className="text-xs text-muted-foreground">安全告警总数</p>
                </CardContent>
              </Card>
            </div>
          </div>

          {/* 搜索和控制面板 */}
          <div className="bg-white border-b border-gray-200 px-6 py-3">
            <div className="flex items-center gap-3 w-full">
              <div className="flex-1 min-w-0">
                <div className="relative">
                  <div className="flex items-stretch border border-gray-300 rounded-md bg-white shadow-sm hover:shadow-md transition-all focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500 h-9">
                    <div className="flex-1 relative">
                      <input
                        type="text"
                        className="w-full px-3 py-2 text-sm border-0 bg-transparent focus:outline-none focus:ring-0 placeholder-gray-500 pr-12 h-full"
                        placeholder="搜索工作流名称或描述..."
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
              <Button variant="outline" size="sm">
                <RefreshCw className="h-4 w-4 mr-2" />
                刷新
              </Button>
            </div>
          </div>

          {/* 工作流表格 */}
          <div className="flex-1 overflow-auto">
            {filteredWorkflows.length === 0 ? (
              <div className="text-center py-12">
                <GitBranch className="h-12 w-12 mx-auto mb-4 text-gray-300" />
                <h3 className="text-lg font-medium text-gray-900 mb-2">
                  暂无工作流
                </h3>
                <p className="text-gray-500">
                  {searchTerm
                    ? "没有找到匹配的工作流"
                    : "工作流功能正在开发中，敬请期待"}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-full">
                  <TableHeader className="bg-gray-50/80 sticky top-0 border-b border-gray-200">
                    <TableRow className="hover:bg-gray-50/80">
                      <TableHead className="px-4 py-3 text-left min-w-[200px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <GitBranch className="h-3.5 w-3.5" />
                          工作流名称
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-left min-w-[100px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <CheckCircle className="h-3.5 w-3.5" />
                          状态
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-left min-w-[150px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          Kafka Topic
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-center min-w-[120px]">
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          处理事件数
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-center min-w-[100px]">
                        <div className="flex items-center justify-center gap-1.5 text-xs font-medium text-gray-700">
                          告警数
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-left min-w-[150px]">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Clock className="h-3.5 w-3.5" />
                          最后运行
                        </div>
                      </TableHead>
                      <TableHead className="px-4 py-3 text-center min-w-[120px]">
                        操作
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredWorkflows.map((workflow) => (
                      <TableRow
                        key={workflow.id}
                        className="border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors"
                      >
                        <TableCell className="px-4 py-3">
                          <div>
                            <div className="font-medium text-gray-900">
                              {workflow.name}
                            </div>
                            <div className="text-sm text-gray-500 mt-1">
                              {workflow.description}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="px-4 py-3">
                          {getStatusBadge(workflow.status)}
                        </TableCell>
                        <TableCell className="px-4 py-3">
                          <span className="font-mono text-sm bg-gray-100 px-2 py-1 rounded">
                            {workflow.kafkaTopic}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-center">
                          <span className="font-mono text-sm">
                            {workflow.processedEvents.toLocaleString()}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-center">
                          <span className="font-mono text-sm">
                            {workflow.alertsGenerated}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3">
                          <span className="text-sm text-gray-600">
                            {new Date(workflow.lastRun).toLocaleString()}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3">
                          <div className="flex items-center justify-center gap-1">
                            <Button variant="ghost" size="sm">
                              <Eye className="h-4 w-4" />
                            </Button>
                            <Button variant="ghost" size="sm">
                              {workflow.status === "running" ? (
                                <Pause className="h-4 w-4" />
                              ) : (
                                <Play className="h-4 w-4" />
                              )}
                            </Button>
                            <Button variant="ghost" size="sm">
                              <Settings className="h-4 w-4" />
                            </Button>
                            <Button variant="ghost" size="sm">
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
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
