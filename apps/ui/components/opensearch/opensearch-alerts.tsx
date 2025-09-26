"use client";

import React, { useState, useEffect } from "react";
import { RefreshCw, AlertTriangle, Search, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { DateTimeRangePicker } from "@/components/ui/datetime-range-picker";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiClient, ApiError } from "@/lib/api";
import { EventsTable } from "@/components/opensearch/events-table";
import { TimelineChartShadcn } from "@/components/opensearch/timeline-chart-shadcn";
import { SecurityEvent, SearchState, TimelineData } from "@/types/security-events";

// 索引选项 - 显示实际的索引 pattern
const indexOptions = [
  { value: "sysarmor-alerts*", label: "sysarmor-alerts*" },
  { value: "sysarmor-alerts-audit", label: "sysarmor-alerts-audit" },
];

// 搜索建议
const searchSuggestions = [
  { value: "alert.severity:critical", label: "严重告警", description: "查找严重级别的告警" },
  { value: "alert.severity:high", label: "高危告警", description: "查找高危级别的告警" },
  { value: "metadata.host:*", label: "按主机搜索", description: "根据主机名搜索" },
  { value: "alert.risk_score:>=80", label: "高风险事件", description: "风险评分大于等于80" },
  { value: "event.type:network_connections", label: "网络连接事件", description: "网络连接相关事件" },
  { value: "@timestamp:[now-1h TO now]", label: "最近1小时", description: "最近1小时的事件" },
  { value: "@timestamp:[now-24h TO now]", label: "最近24小时", description: "最近24小时的事件" },
];

export function OpenSearchAlerts() {
  // 初始化时间范围为过去15天
  const now = new Date();
  const fifteenDaysAgo = new Date(now.getTime() - 15 * 24 * 60 * 60 * 1000);

  const [searchState, setSearchState] = useState<SearchState>({
    query: null,
    timeRange: {
      from: fifteenDaysAgo,
      to: now,
      label: "过去 15 天",
    },
    pagination: {
      current: 1,
      size: 25,
      total: 0,
    },
    sortField: "@timestamp",
    sortDirection: "desc",
    selectedEvents: [],
  });

  const [events, setEvents] = useState<SecurityEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedEvent, setSelectedEvent] = useState<SecurityEvent | null>(null);
  const [detailDialogOpen, setDetailDialogOpen] = useState(false);
  const [timelineData, setTimelineData] = useState<TimelineData[]>([]);
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  // 控件状态
  const [selectedIndex, setSelectedIndex] = useState('sysarmor-alerts*');
  const [searchQuery, setSearchQuery] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);

  // 从真实事件数据生成时间线数据
  const generateTimelineData = (events: SecurityEvent[]): TimelineData[] => {
    const data: TimelineData[] = [];
    const { from, to } = searchState.timeRange;
    const timeRangeMs = to.getTime() - from.getTime();

    let bucketCount: number;
    let bucketSizeMs: number;
    let timeFormat: Intl.DateTimeFormatOptions;

    if (timeRangeMs <= 2 * 60 * 60 * 1000) {
      bucketCount = Math.min(24, Math.ceil(timeRangeMs / (5 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { hour: "2-digit", minute: "2-digit" };
    } else if (timeRangeMs <= 24 * 60 * 60 * 1000) {
      bucketCount = Math.min(24, Math.ceil(timeRangeMs / (60 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { hour: "2-digit", minute: "2-digit" };
    } else if (timeRangeMs <= 7 * 24 * 60 * 60 * 1000) {
      bucketCount = Math.min(28, Math.ceil(timeRangeMs / (6 * 60 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { month: "2-digit", day: "2-digit", hour: "2-digit" };
    } else {
      bucketCount = Math.min(30, Math.ceil(timeRangeMs / (24 * 60 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { month: "2-digit", day: "2-digit" };
    }

    for (let i = 0; i < bucketCount; i++) {
      const bucketStart = new Date(from.getTime() + i * bucketSizeMs);
      const bucketEnd = new Date(from.getTime() + (i + 1) * bucketSizeMs);
      const timeStr = bucketStart.toLocaleString("zh-CN", timeFormat);

      const eventsInBucket = events.filter((event) => {
        const eventTime = new Date(event._source["@timestamp"]);
        return eventTime >= bucketStart && eventTime < bucketEnd;
      });

      const severityCounts = { critical: 0, high: 0, medium: 0, low: 0 };
      eventsInBucket.forEach((event) => {
        const severity = event._source.alert?.severity?.toLowerCase() || "low";
        console.log('Event severity:', event._source.alert?.severity, 'normalized:', severity); // 调试信息
        if (severity in severityCounts) {
          severityCounts[severity as keyof typeof severityCounts]++;
        } else {
          console.log('Unknown severity, defaulting to low:', severity); // 调试信息
          severityCounts.low++;
        }
      });
      
      console.log('Severity counts for bucket:', severityCounts); // 调试信息

      data.push({
        time: timeStr,
        count: eventsInBucket.length,
        critical: severityCounts.critical,
        high: severityCounts.high,
        medium: severityCounts.medium,
        low: severityCounts.low,
      });
    }

    return data;
  };

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      // 构建查询字符串
      let queryParts: string[] = [];
      
      // 添加搜索查询
      if (searchQuery.trim()) {
        queryParts.push(searchQuery.trim());
      }
      
      // 添加时间范围过滤
      const fromTime = searchState.timeRange.from.toISOString();
      const toTime = searchState.timeRange.to.toISOString();
      queryParts.push(`@timestamp:[${fromTime} TO ${toTime}]`);
      
      const finalQuery = queryParts.length > 0 ? queryParts.join(' AND ') : '*';

      const params: any = {
        index: selectedIndex,
        size: searchState.pagination.size,
        from: (searchState.pagination.current - 1) * searchState.pagination.size,
        q: finalQuery,
      };

      if (searchState.sortField && searchState.sortDirection) {
        params.sort = `${searchState.sortField}:${searchState.sortDirection}`;
      }

      const eventsResponse = await apiClient.searchSecurityEvents(params);
      const events = eventsResponse?.hits?.hits || eventsResponse?.data?.hits?.hits || [];
      
      // 解析 total 字段
      let total = 0;
      if (eventsResponse?.hits?.total?.value) {
        total = eventsResponse.hits.total.value;
      } else if (eventsResponse?.data?.hits?.total?.value) {
        total = eventsResponse.data.hits.total.value;
      } else {
        total = events.length;
      }

      setEvents(events);
      setSearchState((prev) => ({
        ...prev,
        pagination: {
          ...prev.pagination,
          total: total,
        },
      }));

      const timelineData = generateTimelineData(events);
      setTimelineData(timelineData);

    } catch (error) {
      console.error("❌ OpenSearch API 调用失败:", error);
      if (error instanceof ApiError) {
        setError(`OpenSearch 连接失败: ${error.message}`);
      } else {
        setError("无法连接到 OpenSearch 服务，请检查后端服务状态");
      }
      setEvents([]);
      setTimelineData([]);
      setSearchState((prev) => ({
        ...prev,
        pagination: { ...prev.pagination, total: 0 },
      }));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [
    searchQuery,
    selectedIndex,
    searchState.timeRange,
    searchState.pagination.current,
    searchState.sortField,
    searchState.sortDirection,
  ]);

  const handleSort = (field: string) => {
    setSearchState((prev) => ({
      ...prev,
      sortField: field,
      sortDirection:
        prev.sortField === field && prev.sortDirection === "desc" ? "asc" : "desc",
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  const handleSelectEvent = (eventId: string) => {
    setSearchState((prev) => ({
      ...prev,
      selectedEvents: prev.selectedEvents.includes(eventId)
        ? prev.selectedEvents.filter((id) => id !== eventId)
        : [...prev.selectedEvents, eventId],
    }));
  };

  const handleSelectAll = () => {
    const allEventIds = events.map((e) => e._id);
    setSearchState((prev) => ({
      ...prev,
      selectedEvents:
        prev.selectedEvents.length === allEventIds.length ? [] : allEventIds,
    }));
  };

  const toggleRowExpansion = (eventId: string) => {
    setExpandedRows((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(eventId)) {
        newSet.delete(eventId);
      } else {
        newSet.add(eventId);
      }
      return newSet;
    });
  };

  const handleDateRangeChange = (from: Date, to: Date) => {
    const label = `${from.toLocaleDateString()} → ${to.toLocaleDateString()}`;
    setSearchState((prev) => ({
      ...prev,
      timeRange: { from, to, label },
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  const formatTimestamp = (timestamp: string) => {
    try {
      return new Date(timestamp).toLocaleString("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
    } catch {
      return timestamp;
    }
  };

  const getSeverityBadge = (severity: string) => {
    const severityMap: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
      critical: { label: "严重", variant: "destructive" },
      high: { label: "高危", variant: "destructive" },
      medium: { label: "中等", variant: "default" },
      low: { label: "低危", variant: "secondary" },
      info: { label: "信息", variant: "outline" },
    };

    const info = severityMap[severity?.toLowerCase()] || {
      label: severity || "未知",
      variant: "outline" as const,
    };

    return <Badge variant={info.variant}>{info.label}</Badge>;
  };

  const getRiskScoreBadge = (score: number) => {
    let variant: "default" | "secondary" | "destructive" | "outline" = "outline";
    if (score >= 80) variant = "destructive";
    else if (score >= 60) variant = "destructive";
    else if (score >= 40) variant = "default";
    else if (score >= 20) variant = "secondary";

    return <Badge variant={variant}>{score}</Badge>;
  };

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 页面标题 - 统一布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-primary" />
              攻击告警
            </h1>
            <p className="text-sm text-muted-foreground mt-1">
              实时安全事件监控和威胁告警分析系统
            </p>
          </div>
        </div>
      </div>

      {/* 控制面板 - 单行布局 */}
      <div className="border-b border-border px-4 lg:px-6 py-3">
        <div className="flex items-center">
          {/* 数据源选择器 */}
          <div className="w-44 flex-shrink-0">
            <Select value={selectedIndex} onValueChange={setSelectedIndex}>
              <SelectTrigger className="text-xs w-full">
                <SelectValue placeholder="选择数据源" />
              </SelectTrigger>
              <SelectContent>
                {indexOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* 搜索框 */}
          <div className="flex-1 mx-3">
            <Popover open={searchOpen} onOpenChange={setSearchOpen}>
              <PopoverTrigger asChild>
                <div className="relative">
                  <Input
                    placeholder="输入搜索查询... (例如: alert.severity:critical)"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pr-10"
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0"
                    onClick={() => setSearchOpen(true)}
                  >
                    <Search className="h-4 w-4" />
                  </Button>
                </div>
              </PopoverTrigger>
              <PopoverContent className="w-[400px] p-0" align="start">
                <Command>
                  <CommandInput placeholder="搜索建议..." />
                  <CommandList>
                    <CommandEmpty>没有找到相关建议</CommandEmpty>
                    <CommandGroup heading="搜索建议">
                      {searchSuggestions.map((suggestion) => (
                        <CommandItem
                          key={suggestion.value}
                          onSelect={() => {
                            setSearchQuery(suggestion.value);
                            setSearchOpen(false);
                          }}
                        >
                          <div className="flex flex-col">
                            <span className="font-medium">{suggestion.label}</span>
                            <span className="text-xs text-muted-foreground">
                              {suggestion.description}
                            </span>
                          </div>
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
          </div>

          {/* 时间范围选择器 - 使用新的日期时间组件 */}
          <div className="w-96 flex-shrink-0">
            <DateTimeRangePicker
              from={searchState.timeRange.from}
              to={searchState.timeRange.to}
              onRangeChange={handleDateRangeChange}
            />
          </div>
        </div>
      </div>

      {/* 时间线图表 - 平铺布局 */}
      {timelineData.length > 0 && (
        <div className="bg-background px-4 lg:px-6 py-4 border-b border-border">
          <TimelineChartShadcn 
            data={timelineData} 
            totalEvents={searchState.pagination.total}
          />
        </div>
      )}

      {/* 事件表格 */}
      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <RefreshCw className="h-6 w-6 animate-spin mr-2 text-primary" />
            <span className="text-sm text-muted-foreground">加载事件数据...</span>
          </div>
        ) : events.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <AlertTriangle className="h-8 w-8 mx-auto mb-3 text-muted-foreground/50" />
            <p className="text-sm font-medium text-foreground">未找到事件</p>
            <p className="text-xs text-muted-foreground">
              尝试调整搜索条件或时间范围
            </p>
          </div>
        ) : (
          <div className="flex flex-col h-full">
            <div className="flex-1 overflow-auto">
              <EventsTable
                events={events}
                searchState={searchState}
                expandedRows={expandedRows}
                onSelectEvent={handleSelectEvent}
                onSelectAll={handleSelectAll}
                onSort={handleSort}
                onToggleExpansion={toggleRowExpansion}
                onViewDetails={(event) => {
                  setSelectedEvent(event);
                  setDetailDialogOpen(true);
                }}
              />
            </div>
            
            {/* 翻页组件 */}
            {searchState.pagination.total > 0 && (
              <div className="border-t border-border px-4 lg:px-6 py-3 bg-background">
                <div className="flex items-center justify-between">
                  <div className="text-sm text-muted-foreground">
                    显示 {((searchState.pagination.current - 1) * searchState.pagination.size) + 1} - {Math.min(searchState.pagination.current * searchState.pagination.size, searchState.pagination.total)} 条，共 {searchState.pagination.total} 条记录
                  </div>
                  
                  <div className="flex items-center gap-2">
                    {/* 每页显示数量选择器 */}
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-muted-foreground">每页显示</span>
                      <Select
                        value={searchState.pagination.size.toString()}
                        onValueChange={(value) => {
                          setSearchState((prev) => ({
                            ...prev,
                            pagination: {
                              ...prev.pagination,
                              size: parseInt(value),
                              current: 1,
                            },
                          }));
                        }}
                      >
                        <SelectTrigger className="w-20 h-8 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="10">10</SelectItem>
                          <SelectItem value="25">25</SelectItem>
                          <SelectItem value="50">50</SelectItem>
                          <SelectItem value="100">100</SelectItem>
                        </SelectContent>
                      </Select>
                      <span className="text-sm text-muted-foreground">条</span>
                    </div>

                    {/* 翻页按钮 */}
                    <div className="flex items-center gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setSearchState((prev) => ({
                            ...prev,
                            pagination: {
                              ...prev.pagination,
                              current: Math.max(1, prev.pagination.current - 1),
                            },
                          }));
                        }}
                        disabled={searchState.pagination.current <= 1}
                        className="h-8 w-8 p-0"
                      >
                        <ChevronLeft className="h-4 w-4" />
                      </Button>
                      
                      <div className="flex items-center gap-1">
                        {(() => {
                          const totalPages = Math.ceil(searchState.pagination.total / searchState.pagination.size);
                          const currentPage = searchState.pagination.current;
                          const pages = [];
                          
                          // 显示页码逻辑
                          let startPage = Math.max(1, currentPage - 2);
                          let endPage = Math.min(totalPages, currentPage + 2);
                          
                          if (endPage - startPage < 4) {
                            if (startPage === 1) {
                              endPage = Math.min(totalPages, startPage + 4);
                            } else {
                              startPage = Math.max(1, endPage - 4);
                            }
                          }
                          
                          for (let i = startPage; i <= endPage; i++) {
                            pages.push(
                              <Button
                                key={i}
                                variant={i === currentPage ? "default" : "outline"}
                                size="sm"
                                onClick={() => {
                                  setSearchState((prev) => ({
                                    ...prev,
                                    pagination: {
                                      ...prev.pagination,
                                      current: i,
                                    },
                                  }));
                                }}
                                className="h-8 w-8 p-0 text-xs"
                              >
                                {i}
                              </Button>
                            );
                          }
                          
                          return pages;
                        })()}
                      </div>
                      
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          const totalPages = Math.ceil(searchState.pagination.total / searchState.pagination.size);
                          setSearchState((prev) => ({
                            ...prev,
                            pagination: {
                              ...prev.pagination,
                              current: Math.min(totalPages, prev.pagination.current + 1),
                            },
                          }));
                        }}
                        disabled={searchState.pagination.current >= Math.ceil(searchState.pagination.total / searchState.pagination.size)}
                        className="h-8 w-8 p-0"
                      >
                        <ChevronRight className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* 事件详情对话框 */}
      <Dialog open={detailDialogOpen} onOpenChange={setDetailDialogOpen}>
        <DialogContent className="sm:max-w-[900px] max-h-[80vh] overflow-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5" />
              事件详情
            </DialogTitle>
            <DialogDescription>
              事件 ID: {selectedEvent?._id}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {selectedEvent && (
              <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div>
                    <h4 className="font-semibold mb-3 text-foreground">基本信息</h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">时间:</span>
                        <span className="font-mono text-foreground">
                          {formatTimestamp(selectedEvent._source["@timestamp"])}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">严重程度:</span>
                        {getSeverityBadge(selectedEvent._source.alert?.severity)}
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">风险评分:</span>
                        {getRiskScoreBadge(selectedEvent._source.alert?.risk_score)}
                      </div>
                    </div>
                  </div>
                  <div>
                    <h4 className="font-semibold mb-3 text-foreground">主机信息</h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">主机名:</span>
                        <span className="text-foreground">{selectedEvent._source.metadata?.host || "N/A"}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">数据源:</span>
                        <span className="font-mono text-foreground">
                          {selectedEvent._source.metadata?.source}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
                <div>
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-semibold text-foreground">原始事件数据 (JSON)</h4>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        if (selectedEvent) {
                          navigator.clipboard.writeText(JSON.stringify(selectedEvent._source, null, 2));
                        }
                      }}
                      className="h-7 px-2 text-xs"
                    >
                      <svg className="h-3 w-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
                      复制
                    </Button>
                  </div>
                  <div className="p-4 bg-muted rounded-lg max-h-64 overflow-auto border border-border">
                    <pre className="text-xs whitespace-pre-wrap text-muted-foreground">
                      {JSON.stringify(selectedEvent._source, null, 2)}
                    </pre>
                  </div>
                </div>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* 错误提示 */}
      {error && (
        <div className="fixed bottom-4 right-4 max-w-md">
          <Card className="border-destructive">
            <CardContent className="p-4">
              <div className="flex items-center">
                <AlertTriangle className="h-5 w-5 text-destructive mr-2" />
                <span className="text-sm text-destructive">{error}</span>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
