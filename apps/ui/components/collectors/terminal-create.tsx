"use client";

import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { apiClient } from "@/lib/api";
import {
  Server,
  Shield,
  Cloud,
  Plus,
  RefreshCw,
  Copy,
  CheckCircle,
  Lock,
} from "lucide-react";
import { DeploymentInstructions } from "@/components/collectors/deployment-instructions";

export function TerminalCreate() {
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createLoading, setCreateLoading] = useState(false);
  const [createSuccess, setCreateSuccess] = useState(false);
  const [createResult, setCreateResult] = useState<{
    collector_id: string;
    worker_url: string;
    script_download_url: string;
  } | null>(null);

  // 创建 Collector 表单状态
  const [formData, setFormData] = useState({
    hostname: "",
    ip_address: "",
    os_type: "",
    os_version: "",
    deployment_type: "agentless",
    metadata: {
      group: "",
      environment: "",
      owner: "",
      description: "",
    },
  });

  const deploymentTypes = {
    primary: [
      {
        id: "agentless",
        title: "Agentless",
        description: "无代理部署模式，通过网络协议收集数据",
        icon: Cloud,
        available: true,
        features: ["无需安装客户端", "网络协议收集", "快速部署", "轻量级监控"],
        color: "blue",
      },
      {
        id: "sysarmor-stack",
        title: "SysArmor Stack",
        description: "完整的 SysArmor 技术栈部署",
        icon: Server,
        available: false,
        features: ["全功能 EDR", "深度行为分析", "实时响应", "高级分析引擎"],
        color: "purple",
      },
    ],
    compatible: [
      {
        id: "wazuh",
        title: "Wazuh",
        description: "兼容 Wazuh SIEM 平台",
        icon: Shield,
        available: false,
        features: [
          "完整的 SIEM 功能",
          "高级威胁检测",
          "合规性监控",
          "集中化管理",
        ],
        color: "green",
      },
      {
        id: "elkeid",
        title: "Elkeid",
        description: "兼容字节跳动 Elkeid 平台",
        icon: Shield,
        available: false,
        features: ["云原生安全", "容器监控", "主机入侵检测", "威胁狩猎"],
        color: "orange",
      },
      {
        id: "other-edr",
        title: "其他 EDR",
        description: "兼容其他主流 EDR 终端",
        icon: Server,
        available: false,
        features: ["多平台兼容", "标准化接口", "灵活配置", "统一管理"],
        color: "gray",
        customLabel: "持续拓展",
      },
    ],
  };

  const handleCardClick = (typeId: string, available: boolean) => {
    if (!available) return;

    setSelectedType(typeId);
    setFormData({ ...formData, deployment_type: typeId });
    setCreateDialogOpen(true);
  };

  const handleCreateCollector = async () => {
    try {
      setCreateLoading(true);

      // 清理空的 metadata 字段
      const cleanedFormData = {
        ...formData,
        metadata: Object.fromEntries(
          Object.entries(formData.metadata).filter(([_, value]) => value !== "")
        ),
      };

      console.log("Sending collector data:", cleanedFormData);
      const response = await apiClient.registerCollector(cleanedFormData);
      console.log("Register response:", response);

      if (response.success) {
        // 设置成功状态和结果
        setCreateSuccess(true);
        setCreateResult(response.data);
      }
    } catch (error) {
      console.error("Failed to create collector:", error);
      // 显示更详细的错误信息
      if (error instanceof Error) {
        alert(`注册失败: ${error.message}`);
      }
    } finally {
      setCreateLoading(false);
    }
  };

  const resetForm = () => {
    setFormData({
      hostname: "",
      ip_address: "",
      os_type: "",
      os_version: "",
      deployment_type: "agentless",
      metadata: {
        group: "",
        environment: "",
        owner: "",
        description: "",
      },
    });
  };

  const closeDialog = () => {
    setCreateDialogOpen(false);
    setCreateSuccess(false);
    setCreateResult(null);
    setSelectedType(null);
    resetForm();
  };

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 - 统一布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <Plus className="h-4 w-4 text-primary" />
              新建终端
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              部署新的 EDR Collector 到目标主机
            </p>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-auto p-4 lg:p-6">
        {/* 部署类型选择 */}
        <div className="max-w-6xl mx-auto">
          {/* 第一排：主要部署类型 */}
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-medium mb-4">主要部署类型</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {deploymentTypes.primary.map((type) => (
                  <Card
                    key={type.id}
                    className={`relative cursor-pointer transition-all duration-200 hover:shadow-md flex flex-col h-full ${
                      !type.available
                        ? "opacity-50 cursor-not-allowed"
                        : "hover:scale-[1.02]"
                    } ${
                      selectedType === type.id
                        ? "ring-2 ring-blue-500 shadow-lg"
                        : ""
                    }`}
                    onClick={() => handleCardClick(type.id, type.available)}
                  >
                    <div className="absolute top-3 right-3 z-10">
                      {type.available ? (
                        <Badge
                          variant="default"
                          className="flex items-center gap-1 text-xs"
                        >
                          可用
                        </Badge>
                      ) : (
                        <Badge
                          variant="secondary"
                          className="flex items-center gap-1 text-xs"
                        >
                          <Lock className="h-3 w-3" />
                          {(type as any).customLabel || "即将推出"}
                        </Badge>
                      )}
                    </div>

                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-3">
                        <div
                          className={`p-2 rounded-full ${
                            type.color === "blue"
                              ? "bg-blue-100"
                              : type.color === "green"
                              ? "bg-green-100"
                              : type.color === "purple"
                              ? "bg-purple-100"
                              : type.color === "orange"
                              ? "bg-orange-100"
                              : "bg-gray-100"
                          }`}
                        >
                          <type.icon
                            className={`h-5 w-5 ${
                              type.color === "blue"
                                ? "text-blue-600"
                                : type.color === "green"
                                ? "text-green-600"
                                : type.color === "purple"
                                ? "text-purple-600"
                                : type.color === "orange"
                                ? "text-orange-600"
                                : "text-gray-600"
                            }`}
                          />
                        </div>
                        <div>
                          <CardTitle className="text-base">
                            {type.title}
                          </CardTitle>
                        </div>
                      </div>
                    </CardHeader>

                    <CardContent className="pt-0 flex flex-col h-full">
                      <CardDescription className="mb-3 text-sm">
                        {type.description}
                      </CardDescription>

                      <div className="space-y-2 flex-1">
                        <h4 className="font-medium text-xs text-muted-foreground">
                          主要特性：
                        </h4>
                        <ul className="space-y-1">
                          {type.features.map(
                            (feature: string, index: number) => (
                              <li
                                key={index}
                                className="flex items-center gap-2 text-xs text-muted-foreground"
                              >
                                <div className="w-1 h-1 rounded-full bg-current opacity-60" />
                                {feature}
                              </li>
                            )
                          )}
                        </ul>
                      </div>

                      <div className="mt-3">
                        {type.available ? (
                          <Button
                            className="w-full"
                            variant="outline"
                            size="sm"
                          >
                            选择此类型
                          </Button>
                        ) : (
                          <Button
                            className="w-full"
                            variant="outline"
                            size="sm"
                            disabled
                          >
                            即将推出
                          </Button>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>

            {/* 第二排：兼容其他 EDR 终端 */}
            <div>
              <h3 className="text-lg font-medium mb-4">兼容其他 EDR 终端</h3>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {deploymentTypes.compatible.map((type) => (
                  <Card
                    key={type.id}
                    className={`relative cursor-pointer transition-all duration-200 hover:shadow-md flex flex-col h-full ${
                      !type.available
                        ? "opacity-50 cursor-not-allowed"
                        : "hover:scale-[1.02]"
                    } ${
                      selectedType === type.id
                        ? "ring-2 ring-blue-500 shadow-lg"
                        : ""
                    }`}
                    onClick={() => handleCardClick(type.id, type.available)}
                  >
                    <div className="absolute top-3 right-3 z-10">
                      {type.available ? (
                        <Badge
                          variant="default"
                          className="flex items-center gap-1 text-xs"
                        >
                          可用
                        </Badge>
                      ) : (
                        <Badge
                          variant="secondary"
                          className="flex items-center gap-1 text-xs"
                        >
                          <Lock className="h-3 w-3" />
                          {(type as any).customLabel || "即将推出"}
                        </Badge>
                      )}
                    </div>

                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-3">
                        <div
                          className={`p-2 rounded-full ${
                            type.color === "blue"
                              ? "bg-blue-100"
                              : type.color === "green"
                              ? "bg-green-100"
                              : type.color === "purple"
                              ? "bg-purple-100"
                              : type.color === "orange"
                              ? "bg-orange-100"
                              : "bg-gray-100"
                          }`}
                        >
                          <type.icon
                            className={`h-5 w-5 ${
                              type.color === "blue"
                                ? "text-blue-600"
                                : type.color === "green"
                                ? "text-green-600"
                                : type.color === "purple"
                                ? "text-purple-600"
                                : type.color === "orange"
                                ? "text-orange-600"
                                : "text-gray-600"
                            }`}
                          />
                        </div>
                        <div>
                          <CardTitle className="text-base">
                            {type.title}
                          </CardTitle>
                        </div>
                      </div>
                    </CardHeader>

                    <CardContent className="pt-0 flex flex-col h-full">
                      <CardDescription className="mb-3 text-sm">
                        {type.description}
                      </CardDescription>

                      <div className="space-y-2 flex-1">
                        <h4 className="font-medium text-xs text-muted-foreground">
                          主要特性：
                        </h4>
                        <ul className="space-y-1">
                          {type.features.map(
                            (feature: string, index: number) => (
                              <li
                                key={index}
                                className="flex items-center gap-2 text-xs text-muted-foreground"
                              >
                                <div className="w-1 h-1 rounded-full bg-current opacity-60" />
                                {feature}
                              </li>
                            )
                          )}
                        </ul>
                      </div>

                      <div className="mt-3">
                        {type.available ? (
                          <Button
                            className="w-full"
                            variant="outline"
                            size="sm"
                          >
                            选择此类型
                          </Button>
                        ) : (
                          <Button
                            className="w-full"
                            variant="outline"
                            size="sm"
                            disabled
                          >
                            即将推出
                          </Button>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 创建终端对话框 */}
      <Dialog open={createDialogOpen} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
          {createSuccess && createResult ? (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <CheckCircle className="h-5 w-5 text-green-600" />
                  终端注册成功
                </DialogTitle>
                <DialogDescription>
                  终端已成功注册，请使用以下信息进行部署
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-4 py-4">
                <div className="space-y-2">
                  <Label>Collector ID</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      value={createResult.collector_id}
                      readOnly
                      className="font-mono text-sm"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        navigator.clipboard.writeText(
                          createResult.collector_id
                        );
                      }}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>Worker URL</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      value={createResult.worker_url}
                      readOnly
                      className="font-mono text-sm"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        navigator.clipboard.writeText(createResult.worker_url);
                      }}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>安装脚本下载链接</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      value={`{API_BASE_URL}/api/v1/resources/scripts/agentless/setup-terminal.sh?collector_id=${createResult.collector_id}`}
                      readOnly
                      className="font-mono text-sm bg-gray-50"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        navigator.clipboard.writeText(
                          `{API_BASE_URL}/api/v1/resources/scripts/agentless/setup-terminal.sh?collector_id=${createResult.collector_id}`
                        );
                      }}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    请在目标主机上下载并执行此脚本来完成终端的安装和配置
                  </p>
                </div>

                {/* 使用可复用的部署指令组件 */}
                <DeploymentInstructions
                  collectorId={createResult.collector_id}
                />
              </div>
              <DialogFooter>
                <Button onClick={closeDialog}>完成</Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>注册新的终端</DialogTitle>
                <DialogDescription>
                  填写以下信息来注册一个新的 EDR 终端实例
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-4 py-4">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="hostname">主机名 *</Label>
                    <Input
                      id="hostname"
                      value={formData.hostname}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          hostname: e.target.value,
                        })
                      }
                      placeholder="输入主机名"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="ip_address">IP 地址 *</Label>
                    <Input
                      id="ip_address"
                      value={formData.ip_address}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          ip_address: e.target.value,
                        })
                      }
                      placeholder="192.168.1.100"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="os_type">操作系统类型 *</Label>
                    <Select
                      value={formData.os_type}
                      onValueChange={(value) =>
                        setFormData({ ...formData, os_type: value })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="选择操作系统" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="Windows">Windows</SelectItem>
                        <SelectItem value="Linux">Linux</SelectItem>
                        <SelectItem value="macOS">macOS</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="os_version">操作系统版本 *</Label>
                    <Input
                      id="os_version"
                      value={formData.os_version}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          os_version: e.target.value,
                        })
                      }
                      placeholder="10.0.19041"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="environment">环境</Label>
                    <Select
                      value={formData.metadata.environment}
                      onValueChange={(value) =>
                        setFormData({
                          ...formData,
                          metadata: {
                            ...formData.metadata,
                            environment: value,
                          },
                        })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="选择环境" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="prod">生产环境</SelectItem>
                        <SelectItem value="staging">测试环境</SelectItem>
                        <SelectItem value="dev">开发环境</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="group">分组</Label>
                    <Input
                      id="group"
                      value={formData.metadata.group}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          metadata: {
                            ...formData.metadata,
                            group: e.target.value,
                          },
                        })
                      }
                      placeholder="服务器组名"
                    />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="description">描述</Label>
                  <Input
                    id="description"
                    value={formData.metadata.description}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        metadata: {
                          ...formData.metadata,
                          description: e.target.value,
                        },
                      })
                    }
                    placeholder="终端描述信息"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={closeDialog}>
                  取消
                </Button>
                <Button
                  onClick={handleCreateCollector}
                  disabled={
                    createLoading ||
                    !formData.hostname ||
                    !formData.ip_address ||
                    !formData.os_type
                  }
                >
                  {createLoading ? (
                    <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
                  ) : (
                    <Plus className="h-4 w-4 mr-2" />
                  )}
                  注册
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
