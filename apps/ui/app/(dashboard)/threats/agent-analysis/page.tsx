"use client";

import { useState } from "react";
import { Brain, AlertTriangle, Zap, PaperclipIcon, CpuIcon, ChevronDownIcon } from "lucide-react";
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

interface ChatMessage {
  id: string;
  type: "user" | "assistant";
  content: string;
  timestamp: Date;
  analysisResult?: AnalysisResult;
  relatedAlert?: Alert;
}

export default function AgentAnalysisPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);

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
    setIsAnalyzing(true);

    try {
      const response = await fetch("/api/agent-analysis", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          query: message.text.trim(),
          alertData: selectedAlert,
        }),
      });

      const result = await response.json();

      if (result.success) {
        const assistantMessage: ChatMessage = {
          id: (Date.now() + 1).toString(),
          type: "assistant",
          content: "我已经完成了对告警数据的分析，以下是详细的分析过程和结果：",
          timestamp: new Date(),
          analysisResult: result.data,
        };
        setMessages(prev => [...prev, assistantMessage]);
      } else {
        throw new Error(result.error || "分析失败");
      }
    } catch (error) {
      console.error("Analysis error:", error);
      const errorMessage: ChatMessage = {
        id: (Date.now() + 1).toString(),
        type: "assistant",
        content: "抱歉，分析过程中出现了错误。请稍后重试。",
        timestamp: new Date(),
      };
      setMessages(prev => [...prev, errorMessage]);
    } finally {
      setIsAnalyzing(false);
    }
  };

  return (
    <div className="overscroll-behavior-contain flex h-dvh min-w-0 touch-pan-y flex-col bg-background">
      {/* 页面标题 */}
      <div className="border-b border-border px-4 lg:px-6 py-4 flex-shrink-0 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold flex items-center gap-3">
              <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10">
                <Brain className="h-4 w-4 text-primary" />
              </div>
              智能体分析
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              基于AI的威胁检测和行为分析系统
            </p>
          </div>
        </div>
      </div>

      {/* 消息区域 - 完全按照 AI chatbot 的布局 */}
      <Conversation className="flex-1">
        <ConversationContent className="mx-auto max-w-4xl pb-16">
          {messages.length === 0 ? (
            <ConversationEmptyState
              icon={<Brain className="size-12 text-primary" />}
              title="开始智能分析"
              description="描述您想要分析的安全事件或告警，AI 将为您提供详细的威胁分析和攻击链重构"
            >
              <div className="flex flex-wrap gap-2 justify-center max-w-2xl mt-4">
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => {
                    handleSubmit({ text: "分析最近的高危告警，重点关注进程注入攻击" });
                  }}
                >
                  进程注入分析
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => {
                    handleSubmit({ text: "检查网络异常连接和横向移动行为" });
                  }}
                >
                  网络行为分析
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => {
                    handleSubmit({ text: "分析恶意软件感染路径和影响范围" });
                  }}
                >
                  恶意软件分析
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => {
                    handleSubmit({ text: "评估当前系统的整体安全态势" });
                  }}
                >
                  安全态势评估
                </Button>
              </div>
            </ConversationEmptyState>
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

                            {/* 分析总结 - 使用 Artifact */}
                            <Artifact>
                              <ArtifactHeader>
                                <div>
                                  <ArtifactTitle>威胁分析总结</ArtifactTitle>
                                  <ArtifactDescription>
                                    基于 MITRE ATT&CK 框架的威胁评估和攻击链分析
                                  </ArtifactDescription>
                                </div>
                                <ArtifactActions>
                                  <ArtifactAction
                                    icon={AlertTriangle}
                                    label="复制分析"
                                    tooltip="复制分析结果"
                                    onClick={() => {
                                      if (message.analysisResult) {
                                        const summary = `威胁分析总结
总体风险: ${message.analysisResult.summary.overallRisk}
置信度: ${(message.analysisResult.summary.confidence * 100).toFixed(1)}%
建议: ${message.analysisResult.summary.recommendation}`;
                                        navigator.clipboard.writeText(summary);
                                      }
                                    }}
                                  />
                                </ArtifactActions>
                              </ArtifactHeader>
                              <ArtifactContent>
                                {/* 攻击链关系图 */}
                                <div className="mb-6">
                                  <h4 className="text-sm font-medium mb-3 flex items-center gap-2">
                                    <svg className="h-4 w-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                                    </svg>
                                    攻击链关系图
                                  </h4>
                                  <div className="bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-950/30 dark:to-purple-950/30 rounded-xl p-6 border">
                                    <div className="flex items-center justify-between space-x-4">
                                      <div className="flex flex-col items-center space-y-2">
                                        <div className="w-12 h-12 bg-red-100 dark:bg-red-900/30 rounded-full flex items-center justify-center">
                                          <svg className="h-6 w-6 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v3m0 0v3m0-3h3m-3 0H9m12 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                                          </svg>
                                        </div>
                                        <span className="text-xs font-medium">初始访问</span>
                                      </div>
                                      <div className="flex-1 h-0.5 bg-gradient-to-r from-red-300 to-orange-300"></div>
                                      <div className="flex flex-col items-center space-y-2">
                                        <div className="w-12 h-12 bg-orange-100 dark:bg-orange-900/30 rounded-full flex items-center justify-center">
                                          <svg className="h-6 w-6 text-orange-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                                          </svg>
                                        </div>
                                        <span className="text-xs font-medium">权限提升</span>
                                      </div>
                                      <div className="flex-1 h-0.5 bg-gradient-to-r from-orange-300 to-yellow-300"></div>
                                      <div className="flex flex-col items-center space-y-2">
                                        <div className="w-12 h-12 bg-yellow-100 dark:bg-yellow-900/30 rounded-full flex items-center justify-center">
                                          <svg className="h-6 w-6 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m0 0a2 2 0 012 2m-2-2a2 2 0 00-2 2m2-2V5a2 2 0 00-2-2" />
                                          </svg>
                                        </div>
                                        <span className="text-xs font-medium">凭据访问</span>
                                      </div>
                                      <div className="flex-1 h-0.5 bg-gradient-to-r from-yellow-300 to-green-300"></div>
                                      <div className="flex flex-col items-center space-y-2">
                                        <div className="w-12 h-12 bg-green-100 dark:bg-green-900/30 rounded-full flex items-center justify-center">
                                          <svg className="h-6 w-6 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                          </svg>
                                        </div>
                                        <span className="text-xs font-medium">横向移动</span>
                                      </div>
                                    </div>
                                  </div>
                                </div>

                                {/* 风险评估指标 */}
                                <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
                                  <div className="text-center p-4 bg-gradient-to-br from-red-50 to-red-100 dark:from-red-950/30 dark:to-red-900/20 rounded-xl border border-red-200 dark:border-red-800">
                                    <div className="text-2xl font-bold text-red-600 mb-1">
                                      {message.analysisResult.summary.overallRisk}
                                    </div>
                                    <div className="text-xs text-red-600 dark:text-red-400">总体风险</div>
                                  </div>
                                  <div className="text-center p-4 bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-950/30 dark:to-blue-900/20 rounded-xl border border-blue-200 dark:border-blue-800">
                                    <div className="text-2xl font-bold text-blue-600 mb-1">
                                      {(message.analysisResult.summary.confidence * 100).toFixed(1)}%
                                    </div>
                                    <div className="text-xs text-blue-600 dark:text-blue-400">分析置信度</div>
                                  </div>
                                  <div className="text-center p-4 bg-gradient-to-br from-green-50 to-green-100 dark:from-green-950/30 dark:to-green-900/20 rounded-xl border border-green-200 dark:border-green-800">
                                    <div className="text-2xl font-bold text-green-600 mb-1">
                                      {message.analysisResult.summary.completedSteps}/{message.analysisResult.summary.totalSteps}
                                    </div>
                                    <div className="text-xs text-green-600 dark:text-green-400">分析步骤</div>
                                  </div>
                                </div>

                                {/* 建议措施 */}
                                <div className="bg-gradient-to-r from-amber-50 to-orange-50 dark:from-amber-950/30 dark:to-orange-950/30 rounded-xl p-4 border border-amber-200 dark:border-amber-800">
                                  <div className="flex items-start gap-3">
                                    <div className="w-8 h-8 bg-amber-100 dark:bg-amber-900/30 rounded-full flex items-center justify-center flex-shrink-0">
                                      <svg className="h-4 w-4 text-amber-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                      </svg>
                                    </div>
                                    <div>
                                      <h4 className="font-medium text-sm text-amber-800 dark:text-amber-200 mb-2">安全建议</h4>
                                      <p className="text-sm text-amber-700 dark:text-amber-300 leading-relaxed">
                                        {message.analysisResult.summary.recommendation}
                                      </p>
                                    </div>
                                  </div>
                                </div>
                              </ArtifactContent>
                            </Artifact>
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
            <PromptInputTextarea
              placeholder="描述您想要分析的安全事件或告警..."
              disabled={isAnalyzing}
            />
            <PromptInputToolbar>
              <PromptInputTools>
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
                disabled={isAnalyzing}
                status={isAnalyzing ? "submitted" : "ready"}
              />
            </PromptInputToolbar>
          </PromptInputBody>
        </PromptInput>
      </div>
    </div>
  );
}
