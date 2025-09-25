"use client";

import { AttackTimelineGraph } from "@/components/threats/attack-timeline-graph";
import { GitBranch } from "lucide-react";

export default function AttackTimelinePage() {
  return (
    <div className="flex flex-col h-full bg-background">
      {/* 页面标题 */}
      <div className="border-b border-border px-4 lg:px-6 py-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <GitBranch className="h-6 w-6 text-primary" />
            攻击溯源图
          </h1>
          <p className="text-muted-foreground">
            基于威胁情报的攻击时间线可视化分析工具
          </p>
        </div>
      </div>
      
      {/* 攻击时间线图表组件 */}
      <div className="flex-1 overflow-auto">
        <AttackTimelineGraph />
      </div>
    </div>
  );
}
