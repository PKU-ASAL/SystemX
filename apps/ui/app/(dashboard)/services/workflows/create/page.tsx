"use client";

import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  Save,
  FileText,
  Database,
  Settings,
  Play,
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

// Mock data for Kafka topics
const mockKafkaTopics = [
  "sysarmor-agentless-748154f5",
  "sysarmor-agentless-a1b2c3d4",
  "sysarmor-host-web-server-01",
  "sysarmor-unknown",
];

// Mock data for Vector templates
const mockVectorTemplates = [
  {
    id: "threat-detection",
    name: "威胁检测模板",
    description: "检测潜在的安全威胁和异常行为",
  },
  {
    id: "user-behavior",
    name: "用户行为分析",
    description: "分析用户登录模式和异常行为",
  },
  {
    id: "network-monitor",
    name: "网络流量监控",
    description: "监控异常网络连接和数据传输",
  },
  {
    id: "custom",
    name: "自定义配置",
    description: "使用自定义 Vector 配置文件",
  },
];

export default function CreateWorkflowPage() {
  const router = useRouter();
  const [formData, setFormData] = useState({
    name: "",
    description: "",
    kafkaTopic: "",
    vectorTemplate: "",
    customConfig: "",
  });

  const handleInputChange = (field: string, value: string) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // TODO: 实现工作流创建逻辑
    console.log("Creating workflow:", formData);
    router.push("/workflows");
  };

  const getVectorConfigPreview = () => {
    if (formData.vectorTemplate === "custom") {
      return formData.customConfig;
    }

    const template = mockVectorTemplates.find(
      (t) => t.id === formData.vectorTemplate
    );
    if (!template) return "";

    // 生成基于模板的配置预览
    return `# Vector 配置 - ${template.name}
# 自动生成于 ${new Date().toISOString()}

[sources.kafka_input]
type = "kafka"
bootstrap_servers = "localhost:9092"
topics = ["${formData.kafkaTopic}"]
group_id = "workflow-${formData.name.toLowerCase().replace(/\s+/g, "-")}"
decoding.codec = "json"

[transforms.parse_and_analyze]
type = "remap"
inputs = ["kafka_input"]
source = '''
# 解析事件数据
.parsed_at = now()
.workflow_id = "${formData.name}"

# ${template.description}
${getTemplateSpecificConfig(formData.vectorTemplate)}
'''

[sinks.alerts_output]
type = "kafka"
inputs = ["parse_and_analyze"]
bootstrap_servers = "localhost:9092"
topic = "sysarmor-alerts"
encoding.codec = "json"

[sinks.console_debug]
type = "console"
inputs = ["parse_and_analyze"]
encoding.codec = "json"`;
  };

  const getTemplateSpecificConfig = (templateId: string) => {
    switch (templateId) {
      case "threat-detection":
        return `# 威胁检测逻辑
if contains(.message, "failed login") || contains(.message, "authentication failure") {
    .alert_level = "high"
    .alert_type = "authentication_failure"
}

if contains(.message, "privilege escalation") || contains(.message, "sudo") {
    .alert_level = "medium"
    .alert_type = "privilege_escalation"
}`;

      case "user-behavior":
        return `# 用户行为分析
.user = parse_regex(.message, r'user=(?P<username>\w+)')?.username ?? "unknown"
.login_time = parse_timestamp(.timestamp, "%Y-%m-%d %H:%M:%S") ?? now()

# 检测异常登录时间
hour = format_timestamp(.login_time, "%H")
if to_int(hour) < 6 || to_int(hour) > 22 {
    .alert_level = "medium"
    .alert_type = "unusual_login_time"
}`;

      case "network-monitor":
        return `# 网络流量监控
.src_ip = parse_regex(.message, r'src=(?P<ip>\d+\.\d+\.\d+\.\d+)')?.ip
.dst_ip = parse_regex(.message, r'dst=(?P<ip>\d+\.\d+\.\d+\.\d+)')?.ip
.bytes = parse_regex(.message, r'bytes=(?P<bytes>\d+)')?.bytes

# 检测大流量传输
if to_int(.bytes) > 1000000 {
    .alert_level = "high"
    .alert_type = "large_data_transfer"
}`;

      default:
        return "# 自定义分析逻辑";
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
                    <BreadcrumbPage>创建工作流</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                创建新的基于 Vector 的 Kafka Topic 数据分析工作流
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push("/workflows")}
            >
              <ArrowLeft className="h-4 w-4 mr-2" />
              返回列表
            </Button>
          </div>

          {/* 表单内容 */}
          <div className="flex-1 overflow-auto p-6">
            <div className="max-w-4xl mx-auto">
              <form onSubmit={handleSubmit} className="space-y-6">
                {/* 基本信息 */}
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <FileText className="h-5 w-5" />
                      基本信息
                    </CardTitle>
                    <CardDescription>
                      配置工作流的基本信息和描述
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="name">工作流名称 *</Label>
                        <Input
                          id="name"
                          placeholder="例如：实时威胁检测"
                          value={formData.name}
                          onChange={(e) =>
                            handleInputChange("name", e.target.value)
                          }
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="kafkaTopic">Kafka Topic *</Label>
                        <Select
                          value={formData.kafkaTopic}
                          onValueChange={(value) =>
                            handleInputChange("kafkaTopic", value)
                          }
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="选择要分析的 Kafka Topic" />
                          </SelectTrigger>
                          <SelectContent>
                            {mockKafkaTopics.map((topic) => (
                              <SelectItem key={topic} value={topic}>
                                {topic}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="description">工作流描述</Label>
                      <Textarea
                        id="description"
                        placeholder="描述这个工作流的用途和分析目标..."
                        value={formData.description}
                        onChange={(e) =>
                          handleInputChange("description", e.target.value)
                        }
                        rows={3}
                      />
                    </div>
                  </CardContent>
                </Card>

                {/* Vector 配置 */}
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Settings className="h-5 w-5" />
                      Vector 配置
                    </CardTitle>
                    <CardDescription>
                      选择分析模板或自定义 Vector 配置
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="vectorTemplate">分析模板</Label>
                      <Select
                        value={formData.vectorTemplate}
                        onValueChange={(value) =>
                          handleInputChange("vectorTemplate", value)
                        }
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="选择分析模板" />
                        </SelectTrigger>
                        <SelectContent>
                          {mockVectorTemplates.map((template) => (
                            <SelectItem key={template.id} value={template.id}>
                              <div>
                                <div className="font-medium">
                                  {template.name}
                                </div>
                                <div className="text-sm text-gray-500">
                                  {template.description}
                                </div>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    {formData.vectorTemplate === "custom" && (
                      <div className="space-y-2">
                        <Label htmlFor="customConfig">自定义 Vector 配置</Label>
                        <Textarea
                          id="customConfig"
                          placeholder="输入您的 Vector TOML 配置..."
                          value={formData.customConfig}
                          onChange={(e) =>
                            handleInputChange("customConfig", e.target.value)
                          }
                          rows={10}
                          className="font-mono text-sm"
                        />
                      </div>
                    )}

                    {formData.vectorTemplate &&
                      formData.vectorTemplate !== "custom" && (
                        <div className="space-y-2">
                          <Label>配置预览</Label>
                          <div className="bg-gray-50 border rounded-md p-4">
                            <pre className="text-sm text-gray-700 whitespace-pre-wrap">
                              {getVectorConfigPreview()}
                            </pre>
                          </div>
                        </div>
                      )}
                  </CardContent>
                </Card>

                {/* 操作按钮 */}
                <div className="flex items-center justify-end gap-3 pt-6 border-t">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => router.push("/workflows")}
                  >
                    取消
                  </Button>
                  <Button
                    type="submit"
                    disabled={!formData.name || !formData.kafkaTopic}
                  >
                    <Save className="h-4 w-4 mr-2" />
                    创建工作流
                  </Button>
                </div>
              </form>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
