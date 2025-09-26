"use client";

import { AttackTimelineGraph } from "@/components/threats/attack-timeline-graph";
import { GitBranch } from "lucide-react";

export default function AttackTimelinePage() {
  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 - 统一布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <GitBranch className="h-4 w-4 text-primary" />
              攻击溯源图
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              基于威胁情报的攻击时间线可视化分析工具
            </p>
          </div>
        </div>
      </div>
      
      {/* 攻击时间线图表组件 */}
      <div className="flex-1 overflow-auto">
        <AttackTimelineGraph />
      </div>
    </div>
  );
}
