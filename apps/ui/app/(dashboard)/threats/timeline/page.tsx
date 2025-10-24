"use client";

import { useState } from "react";
import { AttackTimelineGraph } from "@/components/threats/attack-timeline-graph";
import { ThreatListTable } from "@/components/threats/threat-list-table";
import { GitBranch, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function AttackTimelinePage() {
  const [selectedThreatId, setSelectedThreatId] = useState<string | null>(null);

  const handleThreatSelect = (threatId: string) => {
    setSelectedThreatId(threatId);
  };

  const handleBackToList = () => {
    setSelectedThreatId(null);
  };

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 - 统一布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {selectedThreatId && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleBackToList}
                className="text-muted-foreground hover:text-foreground"
              >
                <ArrowLeft className="h-4 w-4 mr-1" />
                返回列表
              </Button>
            )}
            <div>
              <h1 className="text-lg font-semibold flex items-center gap-2">
                <GitBranch className="h-4 w-4 text-primary" />
                {selectedThreatId ? `攻击溯源图 - ${selectedThreatId.toUpperCase()}` : '攻击溯源图'}
              </h1>
              <p className="text-sm text-muted-foreground mt-1">
                {selectedThreatId 
                  ? '威胁时间线可视化分析与威胁报告'
                  : '选择威胁事件查看详细的攻击时间线分析'
                }
              </p>
            </div>
          </div>
        </div>
      </div>
      
      {/* 内容区域 */}
      <div className="flex-1 overflow-auto">
        {!selectedThreatId ? (
          // 显示威胁列表
          <div className="p-4 lg:p-6">
            <ThreatListTable onThreatSelect={handleThreatSelect} />
          </div>
        ) : (
          // 显示选中威胁的时间线图表
          <div className="h-full">
            <AttackTimelineGraph threatId={selectedThreatId} />
          </div>
        )}
      </div>
    </div>
  );
}
