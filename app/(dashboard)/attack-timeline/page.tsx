"use client";

import { AttackTimelineGraph } from "@/components/attack-timeline-graph";

export default function AttackTimelinePage() {
  return (
    <div className="container mx-auto p-4">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-800">攻击溯源图</h1>
        <p className="text-gray-600 mt-2">
          基于威胁情报的攻击时间线可视化分析工具
        </p>
      </div>
      
      <div className="bg-white rounded-lg shadow-sm">
        <AttackTimelineGraph />
      </div>
    </div>
  );
}