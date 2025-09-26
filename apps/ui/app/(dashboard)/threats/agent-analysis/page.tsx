"use client";

import { useState, useRef, useEffect } from "react";
import { Brain, AlertTriangle, Zap, PaperclipIcon, CpuIcon, ChevronDownIcon, Maximize2, X, Search } from "lucide-react";
import { Message, MessageContent } from "@/components/ai-elements/message";
import {
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  ConversationScrollButton,
} from "@/components/ai-elements/conversation";
import {
  PromptInput,
  PromptInputAttachments,
  PromptInputAttachment,
  PromptInputBody,
  PromptInputTextarea,
  PromptInputToolbar,
  PromptInputTools,
  PromptInputButton,
  PromptInputSubmit,
} from "@/components/ai-elements/prompt-input";
import {
  Task,
  TaskContent,
  TaskItem,
  TaskTrigger,
} from "@/components/ai-elements/task";
import {
  Tool,
  ToolContent,
  ToolHeader,
  ToolInput,
  ToolOutput,
} from "@/components/ai-elements/tool";
import {
  ChainOfThought,
  ChainOfThoughtContent,
  ChainOfThoughtHeader,
  ChainOfThoughtStep,
} from "@/components/ai-elements/chain-of-thought";
import {
  Artifact,
  ArtifactAction,
  ArtifactActions,
  ArtifactContent,
  ArtifactDescription,
  ArtifactHeader,
  ArtifactTitle,
} from "@/components/ai-elements/artifact";
import {
  Source,
  Sources,
  SourcesContent,
  SourcesTrigger,
} from "@/components/ai-elements/sources";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Alert {
  id: string;
  severity: "critical" | "high" | "medium" | "low";
  title: string;
  description: string;
  timestamp: string;
  host: string;
  technique?: string;
  riskScore: number;
}

interface AnalysisStep {
  step: string;
  description: string;
  status: "complete" | "active" | "pending";
  details: any;
}

interface AnalysisResult {
  query: string;
  timestamp: string;
  steps: AnalysisStep[];
  summary: {
    totalSteps: number;
    completedSteps: number;
    overallRisk: string;
    confidence: number;
    recommendation: string;
  };
  artifacts: Array<{
    type: string;
    title: string;
    content: string;
    createdAt: string;
  }>;
}

interface StreamingData {
  thinking?: string[];
  tasks?: Array<{
    id: string;
    title: string;
    status: 'active' | 'complete' | 'pending';
    items: Array<{ text: string; status: 'active' | 'complete' | 'pending' }>;
  }>;
  tools?: Array<{
    name: string;
    state: 'input-available' | 'output-available' | 'input-streaming' | 'output-error';
    input?: any;
    output?: any;
    errorText?: string;
  }>;
  chainOfThought?: Array<{
    id: string;
    label: string;
    description: string;
    status: 'active' | 'complete' | 'pending';
    details: any;
  }>;
}

interface ChatMessage {
  id: string;
  type: "user" | "assistant";
  content: string;
  timestamp: Date;
  analysisResult?: AnalysisResult;
  relatedAlert?: Alert;
  streamingData?: StreamingData;
  isStreaming?: boolean;
}

export default function AgentAnalysisPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
  const [showAlertSearch, setShowAlertSearch] = useState(false);
  const [expandedArtifact, setExpandedArtifact] = useState<string | null>(null);
  const [inputText, setInputText] = useState("");
  
  // 模拟告警数据
  const [mockAlerts] = useState<Alert[]>([
    {
      id: "alert-001",
      severity: "critical",
      title: "检测到进程注入攻击",
      description: "在主机 web-server-01 上检测到可疑的进程注入行为",
      timestamp: "2024-01-15T10:30:00Z",
      host: "web-server-01",
      technique: "T1055",
      riskScore: 95
    },
    {
      id: "alert-002", 
      severity: "high",
      title: "异常网络连接",
      description: "检测到与已知恶意 IP 的异常网络连接",
      timestamp: "2024-01-15T10:25:00Z",
      host: "db-server-02",
      technique: "T1071",
      riskScore: 85
    },
    {
      id: "alert-003",
      severity: "medium",
      title: "权限提升尝试",
      description: "检测到用户权限提升尝试",
      timestamp: "2024-01-15T10:20:00Z", 
      host: "app-server-03",
      technique: "T1548",
      riskScore: 70
    }
  ]);

  const handleSubmit = async (message: { text?: string; files?: any[] }) => {
    if (!message.text?.trim() || isAnalyzing) return;

    const userMessage: ChatMessage = {
      id: Date.now().toString(),
      type: "user",
      content: message.text.trim(),
      timestamp: new Date(),
      relatedAlert: selectedAlert || undefined,
    };

    setMessages(prev => [...prev, userMessage]);
    setInputText(""); // 清空输入框
    setIsAnalyzing(true);

    // 创建流式分析消息
    const assistantMessageId = (Date.now() + 1).toString();
    const assistantMessage: ChatMessage = {
      id: assistantMessageId,
      type: "assistant",
      content: "正在分析中...",
      timestamp: new Date(),
      streamingData: {
        thinking: [],
        tasks: [],
        tools: [],
        chainOfThought: []
      },
      isStreaming: true,
    };

    setMessages(prev => [...prev, assistantMessage]);

    try {
      const response = await fetch("/api/agent-analysis-stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          query: message.text.trim(),
          alertData: selectedAlert,
        }),
      });

      if (!response.body) {
        throw new Error("No response body");
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));
              
              setMessages(prev => prev.map(msg => {
                if (msg.id === assistantMessageId) {
                  const updatedStreamingData = { ...msg.streamingData };
                  
                  switch (data.type) {
                    case 'thinking':
                      updatedStreamingData.thinking = [
                        ...(updatedStreamingData.thinking || []),
                        data.content
                      ];
                      break;
                    case 'task':
                      const existingTaskIndex = updatedStreamingData.tasks?.findIndex(t => t.id === data.task.id);
                      if (existingTaskIndex !== undefined && existingTaskIndex >= 0) {
                        updatedStreamingData.tasks![existingTaskIndex] = data.task;
                      } else {
                        updatedStreamingData.tasks = [
                          ...(updatedStreamingData.tasks || []),
                          data.task
                        ];
                      }
                      break;
                    case 'tool':
                      const existingToolIndex = updatedStreamingData.tools?.findIndex(t => t.name === data.tool.name);
                      if (existingToolIndex !== undefined && existingToolIndex >= 0) {
                        updatedStreamingData.tools![existingToolIndex] = data.tool;
                      } else {
                        updatedStreamingData.tools = [
                          ...(updatedStreamingData.tools || []),
                          data.tool
                        ];
                      }
                      break;
                    case 'chain_of_thought':
                      updatedStreamingData.chainOfThought = [
                        ...(updatedStreamingData.chainOfThought || []),
                        data.step
                      ];
                      break;
                    case 'analysis_result':
                      return {
                        ...msg,
                        content: "我已经完成了对告警数据的分析，以下是详细的分析过程和结果：",
                        analysisResult: data.result,
                        isStreaming: false
                      };
                    case 'done':
                      return {
                        ...msg,
                        isStreaming: false
                      };
                  }
                  
                  return {
                    ...msg,
                    streamingData: updatedStreamingData
                  };
                }
                return msg;
              }));
            } catch (e) {
              console.error('Error parsing SSE data:', e);
            }
          }
        }
      }
    } catch (error) {
      console.error("Analysis error:", error);
      setMessages(prev => prev.map(msg => {
        if (msg.id === assistantMessageId) {
          return {
            ...msg,
            content: "抱歉，分析过程中出现了错误。请稍后重试。",
            isStreaming: false
          };
        }
        return msg;
      }));
    } finally {
      setIsAnalyzing(false);
    }
  };

  return (
    <div className="overscroll-behavior-contain flex h-full min-w-0 touch-pan-y flex-col bg-background">
      {/* 智能体分析页面不需要 nav，直接开始聊天界面 */}

      {/* 消息区域 - 完全按照 AI chatbot 的布局 */}
      <Conversation className="flex-1">
        <ConversationContent className="mx-auto max-w-4xl pb-4">
          {messages.length === 0 ? (
            <ConversationEmptyState
              icon={<Brain className="size-12 text-primary" />}
              title="开始智能分析"
              description="描述您想要分析的安全事件或告警，AI 将为您提供详细的威胁分析和攻击链重构"
            />
          ) : (
            <div className="space-y-6">
              {messages.map((message) => (
                <div key={message.id} className="space-y-4">
                  {/* 显示相关告警源 */}
                  {message.relatedAlert && (
                    <Sources>
                      <SourcesTrigger count={1} />
                      <SourcesContent>
                        <Source 
                          title={`${message.relatedAlert.title} (${message.relatedAlert.host})`}
                          href="#"
                        />
                      </SourcesContent>
                    </Sources>
                  )}
                  
                  <Message from={message.type}>
                    <MessageContent variant="flat">
                      <div className="space-y-4">
                        <p className="text-sm leading-relaxed">{message.content}</p>
                        
                        {/* 流式分析过程 */}
                        {message.streamingData && (
                          <>
                            {/* Thinking 阶段 */}
                            {message.streamingData.thinking && message.streamingData.thinking.length > 0 && (
                              <ChainOfThought defaultOpen={message.isStreaming}>
                                <ChainOfThoughtHeader>思考过程</ChainOfThoughtHeader>
                                <ChainOfThoughtContent>
                                  {message.streamingData.thinking.map((thought, index) => (
                                    <ChainOfThoughtStep
                                      key={index}
                                      label={`思考 ${index + 1}`}
                                      description={thought}
                                      status={message.isStreaming && index === (message.streamingData?.thinking?.length || 0) - 1 ? "active" : "complete"}
                                    />
                                  ))}
                                </ChainOfThoughtContent>
                              </ChainOfThought>
                            )}

                            {/* Task 阶段 */}
                            {message.streamingData.tasks && message.streamingData.tasks.length > 0 && (
                              <div className="space-y-3">
                                <div className="flex items-center gap-2">
                                  <Zap className="h-4 w-4 text-primary" />
                                  <h3 className="font-medium text-sm">执行任务</h3>
                                </div>
                                {message.streamingData.tasks.map((task, index) => (
                                  <Task key={task.id} defaultOpen={task.status === 'active'}>
                                    <TaskTrigger title={task.title} />
                                    <TaskContent>
                                      {task.items.map((item, itemIndex) => (
                                        <TaskItem key={itemIndex}>
                                          <div className="flex items-center gap-2">
                                            <div className={`w-2 h-2 rounded-full ${
                                              item.status === 'complete' ? 'bg-green-500' :
                                              item.status === 'active' ? 'bg-blue-500 animate-pulse' :
                                              'bg-gray-300'
                                            }`} />
                                            <span className="text-sm">{item.text}</span>
                                          </div>
                                        </TaskItem>
                                      ))}
                                    </TaskContent>
                                  </Task>
                                ))}
                              </div>
                            )}

                            {/* Tool 调用 */}
                            {message.streamingData.tools && message.streamingData.tools.length > 0 && (
                              <div className="space-y-3">
                                {message.streamingData.tools.map((tool, index) => (
                                  <Tool key={`${tool.name}-${index}`} defaultOpen={tool.state === 'output-available' || tool.state === 'output-error'}>
                                    <ToolHeader 
                                      type={`tool-${tool.name}` as any}
                                      state={tool.state}
                                      title={tool.name}
                                    />
                                    <ToolContent>
                                      {tool.input && <ToolInput input={tool.input} />}
                                      {(tool.output || tool.errorText) && (
                                        <ToolOutput 
                                          output={tool.output}
                                          errorText={tool.errorText}
                                        />
                                      )}
                                    </ToolContent>
                                  </Tool>
                                ))}
                              </div>
                            )}

                            {/* Chain of Thought 推理 */}
                            {message.streamingData.chainOfThought && message.streamingData.chainOfThought.length > 0 && (
                              <ChainOfThought defaultOpen={true}>
                                <ChainOfThoughtHeader>推理分析</ChainOfThoughtHeader>
                                <ChainOfThoughtContent>
                                  {message.streamingData.chainOfThought.map((step, index) => (
                                    <ChainOfThoughtStep
                                      key={step.id}
                                      label={step.label}
                                      description={step.description}
                                      status={step.status}
                                    >
                                      <div className="bg-muted/50 rounded-lg p-3 border mt-2">
                                        <pre className="text-xs overflow-auto whitespace-pre-wrap">
                                          {JSON.stringify(step.details, null, 2)}
                                        </pre>
                                      </div>
                                    </ChainOfThoughtStep>
                                  ))}
                                </ChainOfThoughtContent>
                              </ChainOfThought>
                            )}
                          </>
                        )}
                        
                        {/* 分析结果 */}
                        {message.analysisResult && (
                          <>
                            {/* CoT 分析步骤 */}
                            <div className="space-y-3">
                              <div className="flex items-center gap-2">
                                <Zap className="h-4 w-4 text-primary" />
                                <h3 className="font-medium text-sm">分析推理过程</h3>
                              </div>
                              {message.analysisResult.steps.map((step, index) => (
                                <Task key={index} defaultOpen={index === 0}>
                                  <TaskTrigger title={`${index + 1}. ${step.step}`} />
                                  <TaskContent>
                                    <TaskItem>
                                      <div className="space-y-2">
                                        <p className="text-sm text-muted-foreground leading-relaxed">
                                          {step.description}
                                        </p>
                                        <div className="bg-muted/50 rounded-lg p-3 border">
                                          <pre className="text-xs overflow-auto whitespace-pre-wrap">
                                            {JSON.stringify(step.details, null, 2)}
                                          </pre>
                                        </div>
                                      </div>
                                    </TaskItem>
                                  </TaskContent>
                                </Task>
                              ))}
                            </div>

                            {/* 威胁分析总结 - 简洁版 Artifact */}
                            <div className="relative rounded-lg border bg-background shadow-sm overflow-hidden">
                              {/* 简洁的标题栏 */}
                              <div className="flex items-center justify-between px-4 py-3 border-b bg-muted/30">
                                <div className="flex items-center gap-2">
                                  <Brain className="h-4 w-4 text-primary" />
                                  <span className="font-medium text-sm">威胁分析总结</span>
                                </div>
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  className="h-6 w-6 p-0"
                                  onClick={() => {
                                    setExpandedArtifact(expandedArtifact === message.id ? null : message.id);
                                  }}
                                >
                                  <Maximize2 className="h-3 w-3" />
                                </Button>
                              </div>
                              
                              {/* 内容区域 */}
                              <div className={`p-6 ${expandedArtifact === message.id ? "" : "max-h-96 overflow-auto"}`}>
                                {/* Mermaid 风格的攻击链图 */}
                                <div className="mb-8">
                                  <div className="bg-gradient-to-r from-slate-50 to-slate-100 dark:from-slate-900/50 dark:to-slate-800/50 rounded-lg p-6 border">
                                    <div className="font-mono text-sm space-y-4">
                                      <div className="text-center text-muted-foreground mb-6">
                                        攻击链分析 (MITRE ATT&CK)
                                      </div>
                                      
                                      <div className="flex items-center justify-center space-x-8">
                                        <div className="text-center">
                                          <div className="w-16 h-16 rounded-lg bg-red-100 dark:bg-red-900/30 border-2 border-red-300 flex items-center justify-center mb-2">
                                            <span className="text-red-600 font-bold text-xs">T1055</span>
                                          </div>
                                          <div className="text-xs font-medium">初始访问</div>
                                          <div className="text-xs text-muted-foreground">进程注入</div>
                                        </div>
                                        
                                        <div className="flex items-center">
                                          <div className="w-8 h-0.5 bg-gradient-to-r from-red-300 to-orange-300"></div>
                                          <div className="w-2 h-2 bg-orange-400 rounded-full mx-1"></div>
                                          <div className="w-8 h-0.5 bg-gradient-to-r from-orange-300 to-yellow-300"></div>
                                        </div>
                                        
                                        <div className="text-center">
                                          <div className="w-16 h-16 rounded-lg bg-orange-100 dark:bg-orange-900/30 border-2 border-orange-300 flex items-center justify-center mb-2">
                                            <span className="text-orange-600 font-bold text-xs">T1071</span>
                                          </div>
                                          <div className="text-xs font-medium">权限提升</div>
                                          <div className="text-xs text-muted-foreground">网络连接</div>
                                        </div>
                                        
                                        <div className="flex items-center">
                                          <div className="w-8 h-0.5 bg-gradient-to-r from-yellow-300 to-green-300"></div>
                                          <div className="w-2 h-2 bg-green-400 rounded-full mx-1"></div>
                                          <div className="w-8 h-0.5 bg-gradient-to-r from-green-300 to-blue-300"></div>
                                        </div>
                                        
                                        <div className="text-center">
                                          <div className="w-16 h-16 rounded-lg bg-green-100 dark:bg-green-900/30 border-2 border-green-300 flex items-center justify-center mb-2">
                                            <span className="text-green-600 font-bold text-xs">T1548</span>
                                          </div>
                                          <div className="text-xs font-medium">横向移动</div>
                                          <div className="text-xs text-muted-foreground">权限提升</div>
                                        </div>
                                      </div>
                                    </div>
                                  </div>
                                </div>

                                {/* 简洁的风险指标 */}
                                <div className="grid grid-cols-3 gap-6 mb-8">
                                  <div className="text-center">
                                    <div className="text-3xl font-bold text-red-600 mb-1">
                                      {message.analysisResult.summary.overallRisk}
                                    </div>
                                    <div className="text-sm text-muted-foreground">总体风险</div>
                                  </div>
                                  <div className="text-center">
                                    <div className="text-3xl font-bold text-blue-600 mb-1">
                                      {(message.analysisResult.summary.confidence * 100).toFixed(0)}%
                                    </div>
                                    <div className="text-sm text-muted-foreground">置信度</div>
                                  </div>
                                  <div className="text-center">
                                    <div className="text-3xl font-bold text-green-600 mb-1">
                                      {message.analysisResult.summary.completedSteps}
                                    </div>
                                    <div className="text-sm text-muted-foreground">完成步骤</div>
                                  </div>
                                </div>

                                {/* 简洁的建议 */}
                                <div className="bg-amber-50 dark:bg-amber-950/20 rounded-lg p-4 border-l-4 border-amber-400">
                                  <div className="flex items-start gap-3">
                                    <AlertTriangle className="h-5 w-5 text-amber-600 mt-0.5" />
                                    <div>
                                      <h4 className="font-medium text-sm mb-2">安全建议</h4>
                                      <p className="text-sm text-muted-foreground leading-relaxed">
                                        {message.analysisResult.summary.recommendation}
                                      </p>
                                    </div>
                                  </div>
                                </div>
                              </div>
                            </div>
                          </>
                        )}
                      </div>
                    </MessageContent>
                  </Message>
                </div>
              ))}
            </div>
          )}
        </ConversationContent>
        <ConversationScrollButton />
      </Conversation>

      {/* 输入区域 - 完全按照 AI chatbot 的布局 */}
      <div className="sticky bottom-0 mx-auto flex w-full max-w-4xl gap-2 border-t-0 bg-background px-2 pb-3 md:px-4 md:pb-4">
        <PromptInput
          onSubmit={(message) => {
            if (!message.text?.trim() || isAnalyzing) return;
            handleSubmit(message);
          }}
          className="w-full"
        >
          <PromptInputBody>
            <PromptInputAttachments>
              {(attachment) => <PromptInputAttachment data={attachment} />}
            </PromptInputAttachments>
            
            {/* 选中的告警上下文显示 - 内嵌在输入框内 */}
            {selectedAlert && (
              <div className="flex items-center gap-2 px-3 py-2 border-b">
                <div className="flex items-center gap-2 bg-secondary/50 rounded-md px-2 py-1">
                  <AlertTriangle className="h-3 w-3 text-orange-500" />
                  <span className="text-xs font-medium truncate max-w-48">
                    {selectedAlert.title}
                  </span>
                  <Badge 
                    variant={selectedAlert.severity === "critical" ? "destructive" : selectedAlert.severity === "high" ? "default" : "secondary"}
                    className="text-xs px-1 py-0"
                  >
                    {selectedAlert.severity}
                  </Badge>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-4 w-4 p-0 hover:bg-destructive/20"
                    onClick={() => setSelectedAlert(null)}
                  >
                    <X className="h-2 w-2" />
                  </Button>
                </div>
                <span className="text-xs text-muted-foreground">作为分析上下文</span>
              </div>
            )}
            
            <PromptInputTextarea
              value={inputText}
              onChange={(e) => setInputText(e.target.value)}
              placeholder={selectedAlert ? 
                `基于告警 "${selectedAlert.title}" 进行分析...` : 
                "描述您想要分析的安全事件或告警... (输入 @ 搜索告警)"
              }
              disabled={isAnalyzing}
              onKeyDown={(e) => {
                if (e.key === "@" && !isAnalyzing) {
                  e.preventDefault();
                  setShowAlertSearch(true);
                }
              }}
            />
            <PromptInputToolbar>
              <PromptInputTools>
                <PromptInputButton 
                  disabled={isAnalyzing}
                  onClick={() => setShowAlertSearch(true)}
                  variant={selectedAlert ? "default" : "ghost"}
                >
                  <Search size={14} />
                  <span className="hidden text-xs sm:block">
                    {selectedAlert ? "更换告警" : "搜索告警"}
                  </span>
                </PromptInputButton>
                <PromptInputButton disabled={isAnalyzing}>
                  <PaperclipIcon size={14} />
                </PromptInputButton>
                <PromptInputButton disabled={isAnalyzing}>
                  <CpuIcon size={16} />
                  <span className="hidden font-medium text-xs sm:block">GPT-4</span>
                  <ChevronDownIcon size={16} />
                </PromptInputButton>
              </PromptInputTools>
              <PromptInputSubmit
                disabled={!inputText.trim() || isAnalyzing}
                status={isAnalyzing ? "submitted" : "ready"}
              />
            </PromptInputToolbar>
          </PromptInputBody>
        </PromptInput>
      </div>

      {/* 告警搜索对话框 */}
      <Dialog open={showAlertSearch} onOpenChange={setShowAlertSearch}>
        <DialogContent className="overflow-hidden p-0 shadow-lg">
          <DialogHeader className="sr-only">
            <DialogTitle>搜索告警数据</DialogTitle>
          </DialogHeader>
          <Command className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-group]]:px-2 [&_[cmdk-input-wrapper]_svg]:h-5 [&_[cmdk-input-wrapper]_svg]:w-5 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3 [&_[cmdk-item]_svg]:h-5 [&_[cmdk-item]_svg]:w-5">
            <CommandInput placeholder="搜索告警数据..." />
            <CommandList>
              <CommandEmpty>未找到相关告警</CommandEmpty>
              <CommandGroup heading="告警列表">
                {mockAlerts.map((alert) => (
                  <CommandItem
                    key={alert.id}
                    onSelect={() => {
                      setSelectedAlert(alert);
                      setShowAlertSearch(false);
                    }}
                    className="flex items-center gap-3 p-3"
                  >
                    <div className="flex items-center gap-2">
                      <AlertTriangle className={`h-4 w-4 ${
                        alert.severity === "critical" ? "text-red-500" :
                        alert.severity === "high" ? "text-orange-500" :
                        alert.severity === "medium" ? "text-yellow-500" : "text-blue-500"
                      }`} />
                      <Badge variant={alert.severity === "critical" ? "destructive" : alert.severity === "high" ? "default" : "secondary"}>
                        {alert.severity}
                      </Badge>
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium truncate">{alert.title}</p>
                      <p className="text-xs text-muted-foreground truncate">{alert.host} • {alert.technique}</p>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      风险: {alert.riskScore}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </DialogContent>
      </Dialog>
    </div>
  );
}
