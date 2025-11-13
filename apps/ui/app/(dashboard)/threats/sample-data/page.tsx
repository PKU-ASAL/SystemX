"use client";

import { useState } from "react";
import { AttackTimelineGraph } from "@/components/threats/attack-timeline-graph";
import { Button } from "@/components/ui/button";
import { Database, FileJson } from "lucide-react";

const SAMPLE_DATA = [
  {
    id: "sample-1",
    name: "Sample Data 1 (Basic 3 Timestamp)",
    description: "output2endpoint_basic_3_timestamp_1.json",
    file: "/sample-data-1.json"
  },
  {
    id: "sample-2",
    name: "Sample Data 2 (Basic 2 Timestamp)",
    description: "output2endpoint_basic_2_timestamp_node_info.json",
    file: "/sample-data-2.json"
  }
];

export default function SampleDataPage() {
  const [selectedSample, setSelectedSample] = useState<typeof SAMPLE_DATA[0] | null>(null);

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {selectedSample && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSelectedSample(null)}
                className="text-muted-foreground hover:text-foreground"
              >
                ← 返回列表
              </Button>
            )}
            <div>
              <h1 className="text-lg font-semibold flex items-center gap-2">
                <Database className="h-4 w-4 text-primary" />
                {selectedSample ? selectedSample.name : 'Sample Data - 测试数据'}
              </h1>
              <p className="text-sm text-muted-foreground mt-1">
                {selectedSample
                  ? selectedSample.description
                  : '选择样本数据查看攻击溯源图'
                }
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="flex-1 overflow-auto">
        {!selectedSample ? (
          // 显示样本数据列表
          <div className="p-4 lg:p-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {SAMPLE_DATA.map((sample) => (
                <div
                  key={sample.id}
                  className="border border-border rounded-lg p-6 hover:border-primary hover:shadow-md transition-all cursor-pointer"
                  onClick={() => setSelectedSample(sample)}
                >
                  <div className="flex items-start gap-3">
                    <FileJson className="h-6 w-6 text-primary flex-shrink-0 mt-1" />
                    <div>
                      <h3 className="font-semibold text-foreground mb-2">
                        {sample.name}
                      </h3>
                      <p className="text-sm text-muted-foreground mb-3">
                        {sample.description}
                      </p>
                      <Button size="sm" variant="outline">
                        查看数据
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          // 显示选中样本的攻击溯源图
          <div className="h-full">
            <AttackTimelineGraph
              threatId={selectedSample.id}
              sampleDataUrl={selectedSample.file}
            />
          </div>
        )}
      </div>
    </div>
  );
}
