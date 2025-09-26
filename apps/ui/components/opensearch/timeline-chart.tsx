"use client";

import React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { TimelineData } from "@/types/security-events";

interface TimelineChartProps {
  data: TimelineData[];
}

export function TimelineChart({ data }: TimelineChartProps) {
  const CustomTooltip = ({ active, payload, label }: any) => {
    if (active && payload && payload.length) {
      const data = payload[0].payload;
      return (
        <div className="bg-white p-3 border border-gray-200 rounded-lg shadow-lg">
          <p className="font-medium text-gray-900">{`Time: ${label}`}</p>
          <p className="text-sm text-gray-600">{`Total: ${data.count}`}</p>
          <div className="mt-2 space-y-1">
            <p className="text-xs text-red-600">{`Critical: ${data.critical}`}</p>
            <p className="text-xs text-orange-600">{`High: ${data.high}`}</p>
            <p className="text-xs text-yellow-600">{`Medium: ${data.medium}`}</p>
            <p className="text-xs text-blue-600">{`Low: ${data.low}`}</p>
          </div>
        </div>
      );
    }
    return null;
  };

  return (
    <div className="h-32">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart
          data={data}
          margin={{ top: 10, right: 30, left: 20, bottom: 10 }}
        >
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis dataKey="time" tick={{ fontSize: 12 }} stroke="#666" />
          <YAxis tick={{ fontSize: 12 }} stroke="#666" />
          <Tooltip content={<CustomTooltip />} />
          <Bar
            dataKey="count"
            fill="#3b82f6"
            radius={[2, 2, 0, 0]}
            opacity={0.8}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
