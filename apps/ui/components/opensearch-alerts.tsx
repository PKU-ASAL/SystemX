"use client";

import React, { useState, useEffect } from "react";
import { RefreshCw, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { apiClient, ApiError } from "@/lib/api";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import { EventsTable } from "@/components/opensearch/events-table";
import { TimelineChart } from "@/components/opensearch/timeline-chart";
import { searchSchema, searchFilters, indexOptions } from "@/components/opensearch/search-config";
import { SecurityEvent, SearchState, TimelineData } from "@/types/security-events";
import dynamic from 'next/dynamic';

// 动态导入EUI组件避免SSR问题
const EuiSearchBar = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiSearchBar })),
  { ssr: false }
);

const EuiSuperSelect = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiSuperSelect })),
  { ssr: false }
);

const EuiFlexGroup = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiFlexGroup })),
  { ssr: false }
);

const EuiFlexItem = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiFlexItem })),
  { ssr: false }
);

export function OpenSearchAlerts() {
  // 初始化时间范围为过去15天
  const now = new Date();
  const fifteenDaysAgo = new Date(now.getTime() - 15 * 24 * 60 * 60 * 1000);

  const [searchState, setSearchState] = useState<SearchState>({
    query: null, // 将由EUI SearchBar管理
    timeRange: {
      from: fifteenDaysAgo,
      to: now,
      label: "~ 15 days ago → now",
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
  const [searchError, setSearchError] = useState<any>(null);
  const [selectedIndex, setSelectedIndex] = useState('sysarmor-alerts*');

  // 处理Index选择变化
  const handleIndexChange = (value: unknown) => {
    const indexValue = value as string;
    setSelectedIndex(indexValue);
    setSearchState((prev) => ({
      ...prev,
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  // 处理EUI SearchBar查询变化
  const handleSearchBarChange = ({ query, error }: any) => {
    if (error) {
      setSearchError(error);
      console.error('Search error:', error);
    } else {
      setSearchError(null);
      setSearchState((prev) => ({
        ...prev,
        query: query,
        pagination: { ...prev.pagination, current: 1 },
      }));
    }
  };

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
        if (severity in severityCounts) {
          severityCounts[severity as keyof typeof severityCounts]++;
        } else {
          severityCounts.low++;
        }
      });

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

      // 构建查询参数
      let queryString = "*";
      if (searchState.query) {
        try {
          const { EuiSearchBar } = await import('@elastic/eui');
          queryString = EuiSearchBar.Query.toESQueryString(searchState.query) || "*";
        } catch (err) {
          console.warn('Failed to convert query:', err);
        }
      }

      // 添加时间范围过滤器
      const fromTime = searchState.timeRange.from.toISOString();
      const toTime = searchState.timeRange.to.toISOString();
      const timeFilter = `@timestamp:[${fromTime} TO ${toTime}]`;
      
      const finalQuery = queryString === "*" 
        ? `* AND ${timeFilter}`
        : `(${queryString}) AND ${timeFilter}`;

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
      console.log('API Response:', eventsResponse); // 调试信息
      
      const events = eventsResponse?.hits?.hits || eventsResponse?.data?.hits?.hits || [];
      
      // 修复total字段解析逻辑
      let total = 0;
      if (eventsResponse?.hits?.total?.value) {
        total = eventsResponse.hits.total.value;
      } else if (eventsResponse?.hits?.total) {
        total = typeof eventsResponse.hits.total === 'number' ? eventsResponse.hits.total : eventsResponse.hits.total.value || 0;
      } else if (eventsResponse?.data?.hits?.total?.value) {
        total = eventsResponse.data.hits.total.value;
      } else if (eventsResponse?.data?.hits?.total) {
        total = typeof eventsResponse.data.hits.total === 'number' ? eventsResponse.data.hits.total : eventsResponse.data.hits.total.value || 0;
      } else {
        // 如果无法从API响应中获取total，使用实际events数组长度
        total = events.length;
      }

      console.log('Parsed events:', events.length, 'Total:', total, 'Raw total field:', eventsResponse?.data?.hits?.total); // 调试信息

      setEvents(events);
      setSearchState((prev) => ({
        ...prev,
        pagination: {
          ...prev.pagination,
          total: total,
        },
      }));

      const timelineData = generateTimelineData(events);
      console.log('Generated timeline data:', timelineData); // 调试信息
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
    searchState.query,
    searchState.timeRange,
    searchState.pagination.current,
    searchState.sortField,
    searchState.sortDirection,
    selectedIndex,
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
    const severityMap: Record<string, { label: string; color: string }> = {
      critical: { label: "Critical", color: "bg-red-100 text-red-800 border-red-200" },
      high: { label: "High", color: "bg-orange-100 text-orange-800 border-orange-200" },
      medium: { label: "Medium", color: "bg-yellow-100 text-yellow-800 border-yellow-200" },
      low: { label: "Low", color: "bg-blue-100 text-blue-800 border-blue-200" },
      info: { label: "Info", color: "bg-gray-100 text-gray-800 border-gray-200" },
    };

    const info = severityMap[severity?.toLowerCase()] || {
      label: severity || "Unknown",
      color: "bg-gray-100 text-gray-800 border-gray-200",
    };

    return (
      <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${info.color}`}>
        {info.label}
      </span>
    );
  };

  const getRiskScoreBadge = (score: number) => {
    let color = "bg-gray-100 text-gray-800 border-gray-200";
    if (score >= 80) color = "bg-red-100 text-red-800 border-red-200";
    else if (score >= 60) color = "bg-orange-100 text-orange-800 border-orange-200";
    else if (score >= 40) color = "bg-yellow-100 text-yellow-800 border-yellow-200";
    else if (score >= 20) color = "bg-blue-100 text-blue-800 border-blue-200";

    return (
      <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${color}`}>
        {score}
      </span>
    );
  };

  return (
    <div className="flex flex-col h-full bg-white">
      {/* 顶部控制栏 - Index选择器、搜索框、时间过滤器 */}
      <div className="border-b border-gray-200 p-6">
        <EuiFlexGroup alignItems="center" gutterSize="m">
          {/* Index选择器 */}
          <EuiFlexItem grow={false} style={{ minWidth: '200px' }}>
            <div style={{ 
              fontSize: '14px',
              '--euiFontSizeS': '14px',
              '--euiFontSizeXS': '12px'
            } as React.CSSProperties}>
              <EuiSuperSelect
                options={indexOptions}
                valueOfSelected={selectedIndex}
                onChange={handleIndexChange}
                fullWidth
              />
            </div>
          </EuiFlexItem>

          {/* 搜索框 */}
          <EuiFlexItem>
            <div style={{ 
              fontSize: '14px',
              '--euiFontSizeS': '14px',
              '--euiFontSizeXS': '12px'
            } as React.CSSProperties}>
              <EuiSearchBar
                box={{
                  placeholder: 'Search events... (e.g., alert.severity:critical metadata.host:server-01)',
                  incremental: true,
                  schema: searchSchema,
                }}
                filters={searchFilters}
                onChange={handleSearchBarChange}
              />
            </div>
          </EuiFlexItem>

          {/* 时间范围选择器 */}
          <EuiFlexItem grow={false}>
            <div style={{ 
              fontSize: '14px',
              '--euiFontSizeS': '14px',
              '--euiFontSizeXS': '12px'
            } as React.CSSProperties}>
              <DateRangePicker
                from={searchState.timeRange.from}
                to={searchState.timeRange.to}
                onRangeChange={(from, to) => {
                  const label = `${from.toLocaleDateString()} → ${to.toLocaleDateString()}`;
                  setSearchState((prev) => ({
                    ...prev,
                    timeRange: { from, to, label },
                    pagination: { ...prev.pagination, current: 1 },
                  }));
                }}
                label={searchState.timeRange.label}
                onRefresh={fetchData}
                isLoading={loading}
              />
            </div>
          </EuiFlexItem>
        </EuiFlexGroup>
      </div>

      {/* 事件数量统计 */}
      <div className="bg-white px-6 py-2 border-b border-gray-200">
        <div className="text-center">
          <span className="text-sm text-gray-600">
            <span className="font-medium text-gray-900">
              {searchState.pagination.total.toLocaleString()}
            </span>{" "}
            events found
          </span>
        </div>
      </div>

      {/* 时间柱状图 */}
      <div className="bg-white px-6 py-4 border-b border-gray-200">
        <TimelineChart data={timelineData} />
      </div>

      {/* 事件表格 */}
      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <RefreshCw className="h-6 w-6 animate-spin mr-2 text-blue-600" />
            <span className="text-sm text-gray-600">Loading events...</span>
          </div>
        ) : events.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            <AlertTriangle className="h-8 w-8 mx-auto mb-3 text-gray-300" />
            <p className="text-sm font-medium text-gray-600">No events found</p>
            <p className="text-xs text-gray-500">Try adjusting your search query or time range</p>
          </div>
        ) : (
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
        )}
      </div>

      {/* 事件详情对话框 */}
      <Dialog open={detailDialogOpen} onOpenChange={setDetailDialogOpen}>
        <DialogContent className="sm:max-w-[900px] max-h-[80vh] overflow-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5" />
              Event Details
            </DialogTitle>
            <DialogDescription>
              Event ID: {selectedEvent?._id}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {selectedEvent && (
              <div className="space-y-6">
                <div className="grid grid-cols-2 gap-6">
                  <div>
                    <h4 className="font-semibold mb-3 text-gray-800">Basic Information</h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-gray-600">Time:</span>
                        <span className="font-mono">
                          {formatTimestamp(selectedEvent._source["@timestamp"])}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">Severity:</span>
                        {getSeverityBadge(selectedEvent._source.alert?.severity)}
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">Risk Score:</span>
                        {getRiskScoreBadge(selectedEvent._source.alert?.risk_score)}
                      </div>
                    </div>
                  </div>
                  <div>
                    <h4 className="font-semibold mb-3 text-gray-800">Host Information</h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-gray-600">Host Name:</span>
                        <span>{selectedEvent._source.metadata?.host || "N/A"}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">Source:</span>
                        <span className="font-mono">
                          {selectedEvent._source.metadata?.source}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
                <div>
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-semibold text-gray-800">Raw Event Data (JSON)</h4>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        if (selectedEvent) {
                          navigator.clipboard.writeText(JSON.stringify(selectedEvent._source, null, 2));
                          // 可以添加一个toast通知
                        }
                      }}
                      className="h-7 px-2 text-xs"
                    >
                      <svg className="h-3 w-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                      </svg>
                      Copy
                    </Button>
                  </div>
                  <div className="p-4 bg-gray-50 rounded-lg max-h-64 overflow-auto border">
                    <pre className="text-xs whitespace-pre-wrap text-gray-700">
                      {JSON.stringify(selectedEvent._source, null, 2)}
                    </pre>
                  </div>
                </div>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
