"use client";

import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ArrowLeft,
  Plus,
  Settings,
  Eye,
  Edit,
  Trash2,
  FileText,
  Download,
  Upload,
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

// Mock data for Vector templates
const mockTemplates = [
  {
    id: "threat-detection",
    name: "威胁检测模板",
    description: "检测潜在的安全威胁和异常行为，包括登录失败、权限提升等",
    category: "安全分析",
    version: "1.2.0",
    author: "SysArmor Team",
    createdAt: "2025-07-15T10:00:00Z",
    updatedAt: "2025-08-01T14:30:00Z",
    usageCount: 15,
    isBuiltIn: true,
  },
  {
    id: "user-behavior",
    name: "用户行为分析",
    description: "分析用户登录模式和异常行为，检测可疑的用户活动",
    category: "行为分析",
    version: "1.1.0",
    author: "SysArmor Team",
    createdAt: "2025-07-20T09:15:00Z",
    updatedAt: "2025-07-28T16:45:00Z",
    usageCount: 8,
    isBuiltIn: true,
  },
  {
    id: "network-monitor",
    name: "网络流量监控",
    description: "监控异常网络连接和数据传输，检测大流量传输和可疑连接",
    category: "网络安全",
    version: "1.0.1",
    author: "SysArmor Team",
    createdAt: "2025-07-25T11:30:00Z",
    updatedAt: "2025-08-03T13:20:00Z",
    usageCount: 12,
    isBuiltIn: true,
  },
  {
    id: "custom-malware",
    name: "恶意软件检测",
    description: "自定义恶意软件检测规则，基于文件哈希和行为特征",
    category: "恶意软件",
    version: "2.0.0",
    author: "Admin User",
    createdAt: "2025-08-01T08:00:00Z",
    updatedAt: "2025-08-05T10:15:00Z",
    usageCount: 3,
    isBuiltIn: false,
  },
];

const getCategoryBadge = (category: string) => {
  const colors: Record<string, string> = {
    安全分析: "bg-red-100 text-red-800 border-red-200",
    行为分析: "bg-blue-100 text-blue-800 border-blue-200",
    网络安全: "bg-green-100 text-green-800 border-green-200",
    恶意软件: "bg-purple-100 text-purple-800 border-purple-200",
  };

  return (
    <Badge
      className={
        colors[category] || "bg-gray-100 text-gray-800 border-gray-200"
      }
    >
      {category}
    </Badge>
  );
};

export default function TemplatesPage() {
  const router = useRouter();
  const [templates] = useState(mockTemplates);

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
                      onClick={(e: React.MouseEvent) => {
                        e.preventDefault();
                        window.location.href = "/dashboard";
                      }}
                    >
                      Dashboard
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbLink
                      href="#"
                      onClick={(e: React.MouseEvent) => {
                        e.preventDefault();
                        router.push("/workflows");
                      }}
                    >
                      分析工作流
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbPage>模板管理</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                管理 Vector 分析模板，创建和编辑自定义分析规则
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => router.push("/workflows")}
              >
                <ArrowLeft className="h-4 w-4 mr-2" />
                返回工作流
              </Button>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-2" />
                创建模板
              </Button>
            </div>
          </div>

          {/* 模板网格 */}
          <div className="flex-1 overflow-auto p-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {templates.map((template) => (
                <Card
                  key={template.id}
                  className="hover:shadow-md transition-shadow"
                >
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="space-y-2">
                        <CardTitle className="text-lg">
                          {template.name}
                        </CardTitle>
                        <div className="flex items-center gap-2">
                          {getCategoryBadge(template.category)}
                          {template.isBuiltIn && (
                            <Badge variant="secondary" className="text-xs">
                              内置
                            </Badge>
                          )}
                        </div>
                      </div>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="sm">
                          <Eye className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="sm">
                          <Edit className="h-4 w-4" />
                        </Button>
                        {!template.isBuiltIn && (
                          <Button variant="ghost" size="sm">
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <CardDescription className="mb-4">
                      {template.description}
                    </CardDescription>

                    <div className="space-y-3">
                      <div className="flex items-center justify-between text-sm">
                        <span className="text-gray-500">版本</span>
                        <span className="font-mono">{template.version}</span>
                      </div>

                      <div className="flex items-center justify-between text-sm">
                        <span className="text-gray-500">作者</span>
                        <span>{template.author}</span>
                      </div>

                      <div className="flex items-center justify-between text-sm">
                        <span className="text-gray-500">使用次数</span>
                        <span className="font-mono">{template.usageCount}</span>
                      </div>

                      <div className="flex items-center justify-between text-sm">
                        <span className="text-gray-500">更新时间</span>
                        <span>
                          {new Date(template.updatedAt).toLocaleDateString()}
                        </span>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 mt-4 pt-4 border-t">
                      <Button
                        variant="outline"
                        size="sm"
                        className="flex-1"
                        onClick={() =>
                          router.push(
                            `/workflows/create?template=${template.id}`
                          )
                        }
                      >
                        <Plus className="h-4 w-4 mr-1" />
                        使用模板
                      </Button>
                      <Button variant="outline" size="sm">
                        <Download className="h-4 w-4" />
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}

              {/* 创建新模板卡片 */}
              <Card className="border-dashed border-2 hover:border-blue-300 transition-colors cursor-pointer">
                <CardContent className="flex flex-col items-center justify-center h-full py-12">
                  <div className="w-12 h-12 rounded-full bg-blue-100 flex items-center justify-center mb-4">
                    <Plus className="h-6 w-6 text-blue-600" />
                  </div>
                  <h3 className="text-lg font-medium text-gray-900 mb-2">
                    创建新模板
                  </h3>
                  <p className="text-sm text-gray-500 text-center mb-4">
                    创建自定义的 Vector 分析模板
                  </p>
                  <Button>
                    <FileText className="h-4 w-4 mr-2" />
                    开始创建
                  </Button>
                </CardContent>
              </Card>

              {/* 导入模板卡片 */}
              <Card className="border-dashed border-2 hover:border-green-300 transition-colors cursor-pointer">
                <CardContent className="flex flex-col items-center justify-center h-full py-12">
                  <div className="w-12 h-12 rounded-full bg-green-100 flex items-center justify-center mb-4">
                    <Upload className="h-6 w-6 text-green-600" />
                  </div>
                  <h3 className="text-lg font-medium text-gray-900 mb-2">
                    导入模板
                  </h3>
                  <p className="text-sm text-gray-500 text-center mb-4">
                    从文件导入现有的模板配置
                  </p>
                  <Button variant="outline">
                    <Upload className="h-4 w-4 mr-2" />
                    选择文件
                  </Button>
                </CardContent>
              </Card>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
