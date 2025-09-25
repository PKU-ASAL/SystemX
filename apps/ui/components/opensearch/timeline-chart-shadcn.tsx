"use client";

import React from "react";
import { Bar, BarChart, CartesianGrid, XAxis } from "recharts";
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { TimelineData } from "@/types/security-events";

interface TimelineChartProps {
  data: TimelineData[];
  totalEvents: number;
}

const chartConfig = {
  critical: {
    label: "严重",
    color: "#ef4444", // red-500
  },
  high: {
    label: "高危", 
    color: "#f97316", // orange-500
  },
  medium: {
    label: "中等",
    color: "#eab308", // yellow-500
  },
  low: {
    label: "低危",
    color: "#22c55e", // green-500
  },
} satisfies ChartConfig;

export function TimelineChartShadcn({ data, totalEvents }: TimelineChartProps) {
  return (
    <div className="space-y-4">
      {/* 事件数量统计 - 平铺显示 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="text-2xl font-bold text-foreground">
            {totalEvents.toLocaleString()}
          </div>
          <div className="text-sm text-muted-foreground">
            事件总数
          </div>
        </div>
        <div className="flex items-center gap-4 text-xs">
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-red-500"></div>
            <span>严重</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-orange-500"></div>
            <span>高危</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-yellow-500"></div>
            <span>中等</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-green-500"></div>
            <span>低危</span>
          </div>
        </div>
      </div>

      {/* 时间线图表 */}
      <ChartContainer config={chartConfig} className="h-32 w-full">
        <BarChart
          accessibilityLayer
          data={data}
          margin={{
            left: 12,
            right: 12,
            top: 12,
            bottom: 12,
          }}
        >
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey="time"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={32}
            tick={{ fontSize: 11 }}
          />
          <ChartTooltip
            content={
              <ChartTooltipContent
                className="w-[200px]"
                labelFormatter={(value) => `时间: ${value}`}
                formatter={(value, name) => [
                  value,
                  chartConfig[name as keyof typeof chartConfig]?.label || name
                ]}
              />
            }
          />
          <Bar
            dataKey="critical"
            stackId="severity"
            fill="var(--color-critical)"
            radius={[0, 0, 0, 0]}
          />
          <Bar
            dataKey="high"
            stackId="severity"
            fill="var(--color-high)"
            radius={[0, 0, 0, 0]}
          />
          <Bar
            dataKey="medium"
            stackId="severity"
            fill="var(--color-medium)"
            radius={[0, 0, 0, 0]}
          />
          <Bar
            dataKey="low"
            stackId="severity"
            fill="var(--color-low)"
            radius={[2, 2, 0, 0]}
          />
        </BarChart>
      </ChartContainer>
    </div>
  );
}
