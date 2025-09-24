"use client";

import React, { useState, useEffect } from "react";
import dynamic from 'next/dynamic';

import type {
  OnTimeChangeProps,
  OnRefreshProps,
  EuiSuperDatePickerProps,
} from '@elastic/eui';

interface DateRangePickerProps {
  from: Date;
  to: Date;
  onRangeChange: (from: Date, to: Date) => void;
  label?: string;
  className?: string;
  showRecentlyUsed?: boolean;
  isLoading?: boolean;
  onRefresh?: () => Promise<void>;
}

// 将Date对象转换为EUI格式的时间字符串
function dateToEuiFormat(date: Date): string {
  return date.toISOString();
}

// 将EUI格式的时间字符串转换为Date对象
function euiFormatToDate(dateString: string): Date {
  if (dateString.startsWith('now')) {
    const now = new Date();
    if (dateString === 'now') return now;
    const match = dateString.match(/now([+-])(\d+)([smhdwMy])/);
    if (match) {
      const [, operator, value, unit] = match;
      const num = parseInt(value, 10);
      const multiplier = operator === '-' ? -1 : 1;
      switch (unit) {
        case 's': return new Date(now.getTime() + multiplier * num * 1000);
        case 'm': return new Date(now.getTime() + multiplier * num * 60 * 1000);
        case 'h': return new Date(now.getTime() + multiplier * num * 60 * 60 * 1000);
        case 'd': return new Date(now.getTime() + multiplier * num * 24 * 60 * 60 * 1000);
        case 'w': return new Date(now.getTime() + multiplier * num * 7 * 24 * 60 * 60 * 1000);
        case 'M': return new Date(now.getFullYear(), now.getMonth() + multiplier * num, now.getDate());
        case 'y': return new Date(now.getFullYear() + multiplier * num, now.getMonth(), now.getDate());
        default: return now;
      }
    }
  }
  return new Date(dateString);
}

// 创建一个完全隔离的EUI组件包装器
const SafeEuiSuperDatePicker = dynamic(
  () => import('@elastic/eui').then(mod => {
    const { EuiSuperDatePicker } = mod;
    
    // 创建一个安全的包装器组件
    const SafeWrapper = (props: any) => {
      // 使用useEffect确保只在客户端渲染
      const [mounted, setMounted] = React.useState(false);
      
      React.useEffect(() => {
        setMounted(true);
      }, []);
      
      if (!mounted) {
        return (
          <div className="inline-flex items-center px-3 py-2 border border-gray-300 rounded-md bg-white shadow-sm">
            <div className="animate-pulse flex items-center space-x-2">
              <div className="h-4 w-4 bg-gray-300 rounded"></div>
              <div className="h-4 w-32 bg-gray-300 rounded"></div>
            </div>
          </div>
        );
      }
      
      // 严格过滤props，只传递必要的属性
      const safeProps = {
        isLoading: props.isLoading,
        start: props.start,
        end: props.end,
        onTimeChange: props.onTimeChange,
        onRefresh: props.onRefresh,
        recentlyUsedRanges: props.recentlyUsedRanges,
        showUpdateButton: props.showUpdateButton,
        isAutoRefreshOnly: props.isAutoRefreshOnly,
      };
      
      // 使用div包装器确保不会有Fragment问题
      return (
        <div style={{ display: 'contents' }}>
          <EuiSuperDatePicker {...safeProps} />
        </div>
      );
    };
    
    return { default: SafeWrapper };
  }),
  { 
    ssr: false,
    loading: () => (
      <div className="inline-flex items-center px-3 py-2 border border-gray-300 rounded-md bg-white shadow-sm">
        <div className="animate-pulse flex items-center space-x-2">
          <div className="h-4 w-4 bg-gray-300 rounded"></div>
          <div className="h-4 w-32 bg-gray-300 rounded"></div>
        </div>
      </div>
    )
  }
);

export function DateRangePicker({
  from,
  to,
  onRangeChange,
  className = "",
  showRecentlyUsed = true,
  isLoading = false,
  onRefresh,
}: DateRangePickerProps) {
  const [recentlyUsedRanges, setRecentlyUsedRanges] = useState<
    NonNullable<EuiSuperDatePickerProps['recentlyUsedRanges']>
  >([]);
  const [start, setStart] = useState('now-30m');
  const [end, setEnd] = useState('now');

  // 当外部时间范围变化时，同步状态
  useEffect(() => {
    setStart(dateToEuiFormat(from));
    setEnd(dateToEuiFormat(to));
  }, [from, to]);

  const onTimeChange = ({ start, end }: OnTimeChangeProps) => {
    // 更新最近使用的时间范围
    const updated = recentlyUsedRanges.filter(r => r.start !== start || r.end !== end);
    updated.unshift({ start, end });
    setRecentlyUsedRanges(updated.length > 10 ? updated.slice(0, 10) : updated);

    setStart(start);
    setEnd(end);

    const fromDate = euiFormatToDate(start);
    const toDate = euiFormatToDate(end);
    onRangeChange(fromDate, toDate);
  };

  const handleRefresh = async ({ start, end, refreshInterval }: OnRefreshProps) => {
    if (onRefresh) await onRefresh();
    console.log('Refresh:', { start, end, refreshInterval });
  };

  return (
    <div className={className}>
      <SafeEuiSuperDatePicker
        isLoading={isLoading}
        start={start}
        end={end}
        onTimeChange={onTimeChange}
        onRefresh={handleRefresh}
        recentlyUsedRanges={showRecentlyUsed ? recentlyUsedRanges : undefined}
        showUpdateButton={true}
        isAutoRefreshOnly={false}
      />
    </div>
  );
}
