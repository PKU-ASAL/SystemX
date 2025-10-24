"use client";

import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { ChevronDown, Calendar as CalendarIcon, Clock } from "lucide-react";
import { cn } from "@/lib/utils";

interface DateTimeRangePickerProps {
  from: Date;
  to: Date;
  onRangeChange: (from: Date, to: Date) => void;
  className?: string;
}

// 相对日期预设
const RELATIVE_PRESETS = [
  { name: 'last1h', label: '最近1小时' },
  { name: 'last24h', label: '最近24小时' },
  { name: 'last7d', label: '最近7天' },
  { name: 'last30d', label: '最近30天' },
  { name: 'last3m', label: '最近3个月' },
  { name: 'last6m', label: '最近6个月' },
  { name: 'last1y', label: '最近1年' },
];

const getRelativeRange = (presetName: string): { from: Date; to: Date } => {
  const now = new Date();
  const from = new Date();
  
  switch (presetName) {
    case 'last1h':
      from.setHours(from.getHours() - 1);
      break;
    case 'last24h':
      from.setDate(from.getDate() - 1);
      break;
    case 'last7d':
      from.setDate(from.getDate() - 7);
      break;
    case 'last30d':
      from.setDate(from.getDate() - 30);
      break;
    case 'last3m':
      from.setMonth(from.getMonth() - 3);
      break;
    case 'last6m':
      from.setMonth(from.getMonth() - 6);
      break;
    case 'last1y':
      from.setFullYear(from.getFullYear() - 1);
      break;
    default:
      from.setDate(from.getDate() - 7);
  }
  
  return { from, to: now };
};

const formatDateTime = (date: Date): string => {
  // 使用固定格式避免水合错误
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');
  
  return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
};

const formatDateOnly = (date: Date): string => {
  return date.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
};

const formatTimeOnly = (date: Date): string => {
  return date.toTimeString().slice(0, 8);
};

export function DateTimeRangePicker({
  from,
  to,
  onRangeChange,
  className,
}: DateTimeRangePickerProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [fromOpen, setFromOpen] = useState(false);
  const [toOpen, setToOpen] = useState(false);

  const handleDateChange = (date: Date | undefined, isFrom: boolean) => {
    if (!date) return;
    
    if (isFrom) {
      // 保持原有时间，只更新日期
      const newDate = new Date(date);
      newDate.setHours(from.getHours());
      newDate.setMinutes(from.getMinutes());
      newDate.setSeconds(from.getSeconds());
      onRangeChange(newDate, to);
    } else {
      // 保持原有时间，只更新日期
      const newDate = new Date(date);
      newDate.setHours(to.getHours());
      newDate.setMinutes(to.getMinutes());
      newDate.setSeconds(to.getSeconds());
      onRangeChange(from, newDate);
    }
  };

  const handleTimeChange = (timeString: string, isFrom: boolean) => {
    const [hours, minutes, seconds] = timeString.split(':').map(Number);
    
    if (isFrom) {
      const newDate = new Date(from);
      newDate.setHours(hours || 0);
      newDate.setMinutes(minutes || 0);
      newDate.setSeconds(seconds || 0);
      onRangeChange(newDate, to);
    } else {
      const newDate = new Date(to);
      newDate.setHours(hours || 0);
      newDate.setMinutes(minutes || 0);
      newDate.setSeconds(seconds || 0);
      onRangeChange(from, newDate);
    }
  };

  return (
    <div className={cn(className)}>
      <Popover open={isOpen} onOpenChange={setIsOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            className="w-full justify-between font-normal text-sm"
          >
            <div className="flex items-center justify-between w-full pr-2">
              <div className="flex-1 text-left">{formatDateTime(from)}</div>
              <span className="text-muted-foreground px-2">至</span>
              <div className="flex-1 text-right">{formatDateTime(to)}</div>
            </div>
            <ChevronDown className="h-4 w-4 flex-shrink-0" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-3" align="start">
          <div className="flex gap-3">
            {/* 左侧：相对日期预设 */}
            <div className="w-28">
              <div className="text-xs font-medium mb-2">快速选择</div>
              <div className="space-y-1">
                {RELATIVE_PRESETS.map((preset) => (
                  <Button
                    key={preset.name}
                    variant="ghost"
                    size="sm"
                    className="w-full justify-start text-xs h-7 px-2"
                    onClick={() => {
                      const range = getRelativeRange(preset.name);
                      onRangeChange(range.from, range.to);
                      setIsOpen(false);
                    }}
                  >
                    <Clock className="h-3 w-3 mr-1" />
                    {preset.label}
                  </Button>
                ))}
              </div>
            </div>

            {/* 右侧：自定义日期时间 */}
            <div className="w-72 space-y-3">
              <div className="text-xs font-medium">自定义时间</div>
              
              {/* 开始时间 */}
              <div className="space-y-1">
                <div className="text-xs text-muted-foreground">开始</div>
                <div className="flex gap-2">
                  <Popover open={fromOpen} onOpenChange={setFromOpen}>
                    <PopoverTrigger asChild>
                      <Button
                        variant="outline"
                        className="w-36 justify-between font-normal text-xs h-8"
                      >
                        {formatDateOnly(from)}
                        <CalendarIcon className="h-3 w-3" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0" align="start">
                      <Calendar
                        mode="single"
                        selected={from}
                        onSelect={(date) => {
                          handleDateChange(date, true);
                          setFromOpen(false);
                        }}
                        initialFocus
                      />
                    </PopoverContent>
                  </Popover>
                  <Input
                    type="time"
                    value={formatTimeOnly(from)}
                    onChange={(e) => handleTimeChange(e.target.value, true)}
                    className="w-32 text-xs h-8"
                    step="1"
                  />
                </div>
              </div>

              {/* 结束时间 */}
              <div className="space-y-1">
                <div className="text-xs text-muted-foreground">结束</div>
                <div className="flex gap-2">
                  <Popover open={toOpen} onOpenChange={setToOpen}>
                    <PopoverTrigger asChild>
                      <Button
                        variant="outline"
                        className="w-36 justify-between font-normal text-xs h-8"
                      >
                        {formatDateOnly(to)}
                        <CalendarIcon className="h-3 w-3" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0" align="start">
                      <Calendar
                        mode="single"
                        selected={to}
                        onSelect={(date) => {
                          handleDateChange(date, false);
                          setToOpen(false);
                        }}
                        initialFocus
                      />
                    </PopoverContent>
                  </Popover>
                  <Input
                    type="time"
                    value={formatTimeOnly(to)}
                    onChange={(e) => handleTimeChange(e.target.value, false)}
                    className="w-32 text-xs h-8"
                    step="1"
                  />
                </div>
              </div>

              {/* 操作按钮 */}
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 px-3 text-xs"
                  onClick={() => setIsOpen(false)}
                >
                  取消
                </Button>
                <Button
                  size="sm"
                  className="h-7 px-3 text-xs"
                  onClick={() => setIsOpen(false)}
                >
                  确定
                </Button>
              </div>
            </div>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
