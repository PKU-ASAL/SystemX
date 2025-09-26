"use client";

import { BarChart3 } from "lucide-react";

export default function LogAnalysisPage() {
  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 - 统一布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <BarChart3 className="h-4 w-4 text-primary" />
              日志分析
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              分析日志数据，生成统计报告和趋势图表
            </p>
          </div>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-4xl mx-auto p-4 lg:p-6">
          <div className="text-center py-12">
            <BarChart3 className="h-16 w-16 mx-auto text-muted-foreground mb-4" />
            <h2 className="text-xl font-semibold mb-2">日志分析</h2>
            <p className="text-muted-foreground">
              日志分析功能正在开发中，敬请期待...
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
