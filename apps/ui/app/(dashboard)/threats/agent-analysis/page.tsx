"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Brain,
  Bot,
  Activity,
  TrendingUp,
  AlertTriangle,
  CheckCircle,
  RefreshCw,
  Settings,
  Play,
  Pause,
  BarChart3,
} from "lucide-react";

export default function AgentAnalysisPage() {
  const [loading, setLoading] = useState(false);

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-auto p-4 lg:p-6">
        {/* 页面标题 */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold flex items-center gap-2">
              <Brain className="h-6 w-6 text-primary" />
              智能体分析
            </h1>
            <p className="text-muted-foreground">
              基于AI的威胁检测和行为分析系统
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm">
              <Settings className="h-4 w-4 mr-2" />
              配置
            </Button>
            <Button variant="outline" size="sm">
              <RefreshCw className="h-4 w-4 mr-2" />
              刷新
            </Button>
          </div>
        </div>

        {/* 智能体状态概览 */}
        <div className="grid grid-cols-1 gap-4 mb-8 md:grid-cols-2 lg:grid-cols-4">
          {/* 活跃智能体 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium">活跃智能体</CardTitle>
                <Bot className="h-4 w-4 text-primary" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-primary">12</div>
              <p className="text-xs text-muted-foreground">
                总计 15 个智能体
              </p>
            </CardContent>
          </Card>

          {/* 检测准确率 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium">检测准确率</CardTitle>
                <TrendingUp className="h-4 w-4 text-primary" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-primary">94.2%</div>
              <p className="text-xs text-muted-foreground">
                过去24小时
              </p>
            </CardContent>
          </Card>

          {/* 威胁识别 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium">威胁识别</CardTitle>
                <AlertTriangle className="h-4 w-4 text-destructive" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-destructive">23</div>
              <p className="text-xs text-muted-foreground">
                今日新增威胁
              </p>
            </CardContent>
          </Card>

          {/* 处理状态 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium">处理状态</CardTitle>
                <Activity className="h-4 w-4 text-primary" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-primary">运行中</div>
              <p className="text-xs text-muted-foreground">
                所有智能体正常
              </p>
            </CardContent>
          </Card>
        </div>

        {/* 智能体列表 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
          {/* 威胁检测智能体 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Bot className="h-5 w-5 text-primary" />
                威胁检测智能体
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* 恶意软件检测 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">恶意软件检测</div>
                    <div className="text-xs text-muted-foreground">实时文件扫描</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="default" className="text-xs">运行中</Badge>
                  <Button variant="ghost" size="sm">
                    <Pause className="h-3 w-3" />
                  </Button>
                </div>
              </div>

              {/* 异常行为检测 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">异常行为检测</div>
                    <div className="text-xs text-muted-foreground">用户行为分析</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="default" className="text-xs">运行中</Badge>
                  <Button variant="ghost" size="sm">
                    <Pause className="h-3 w-3" />
                  </Button>
                </div>
              </div>

              {/* 网络入侵检测 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-yellow-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">网络入侵检测</div>
                    <div className="text-xs text-muted-foreground">流量监控分析</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="text-xs">维护中</Badge>
                  <Button variant="ghost" size="sm">
                    <Play className="h-3 w-3" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* 响应处理智能体 */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Brain className="h-5 w-5 text-primary" />
                响应处理智能体
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* 自动隔离 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">自动隔离</div>
                    <div className="text-xs text-muted-foreground">威胁自动处理</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="default" className="text-xs">运行中</Badge>
                  <Button variant="ghost" size="sm">
                    <Pause className="h-3 w-3" />
                  </Button>
                </div>
              </div>

              {/* 告警生成 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">告警生成</div>
                    <div className="text-xs text-muted-foreground">智能告警推送</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="default" className="text-xs">运行中</Badge>
                  <Button variant="ghost" size="sm">
                    <Pause className="h-3 w-3" />
                  </Button>
                </div>
              </div>

              {/* 报告生成 */}
              <div className="flex items-center justify-between p-3 bg-secondary/30 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <div>
                    <div className="font-medium text-sm">报告生成</div>
                    <div className="text-xs text-muted-foreground">安全分析报告</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="default" className="text-xs">运行中</Badge>
                  <Button variant="ghost" size="sm">
                    <Pause className="h-3 w-3" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* 分析统计 */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <BarChart3 className="h-5 w-5" />
              智能体分析统计
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-center py-12 text-muted-foreground">
              <div className="text-center space-y-2">
                <BarChart3 className="h-12 w-12 mx-auto" />
                <p className="text-lg font-medium">分析图表区域</p>
                <p className="text-sm">智能体性能和检测效果统计图表将在此显示</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
