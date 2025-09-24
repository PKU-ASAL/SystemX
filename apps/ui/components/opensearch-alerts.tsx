"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { apiClient, ApiError } from "@/lib/api";
import {
  RefreshCw,
  Search,
  Eye,
  AlertTriangle,
  Shield,
  Activity,
  Clock,
  Filter,
  ChevronDown,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Database,
  Server,
  Flag,
  Plus,
  X,
  Calendar as CalendarIcon,
  ChevronRight,
} from "lucide-react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import dynamic from 'next/dynamic';

// 动态导入EUI组件避免SSR问题
const EuiSearchBar = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiSearchBar })),
  { ssr: false }
);

const EuiHealth = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiHealth })),
  { ssr: false }
);

const EuiSuperSelect = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiSuperSelect })),
  { ssr: false }
);

const EuiFormRow = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiFormRow })),
  { ssr: false }
);

interface SecurityEvent {
  _id: string;
  _source: {
    "@timestamp": string;
    alert: {
      id: string;
      type: string;
      category: string;
      severity: string;
      risk_score: number;
      confidence: number;
      rule: {
        id: string;
        name: string;
        description: string;
        title: string;
        mitigation: string;
        references: string[];
      };
      evidence: {
        event_type: string;
        process_name: string;
        process_cmdline: string;
        file_path: string;
        network_info: any;
      };
    };
    event: {
      raw: {
        event_id: string;
        timestamp: string;
        source: string;
        message: {
          "evt.num": number;
          "evt.time": number;
          "evt.type": string;
          "evt.category": string;
          "evt.dir": string;
          "evt.args": string;
          "proc.name": string;
          "proc.exe": string;
          "proc.cmdline": string;
          "proc.pid": number;
          "proc.ppid": number;
          "proc.uid": number;
          "proc.gid": number;
          "fd.name": string;
          "net.sockaddr": any;
          host: string;
          is_warn: boolean;
        };
      };
    };
    timing: {
      created_at: string;
      processed_at: string;
    };
    metadata: {
      collector_id: string;
      host: string;
      source: string;
      processor: string;
    };
  };
}

interface DynamicFilter {
  id: string;
  field: string;
  operator: string;
  value: string;
  label: string;
}

interface SearchState {
  query: string;
  dynamicFilters: DynamicFilter[];
  timeRange: {
    from: Date;
    to: Date;
    label: string;
  };
  pagination: {
    current: number;
    size: number;
    total: number;
  };
  sortField: string;
  sortDirection: "asc" | "desc";
  selectedEvents: string[];
}

interface TimelineData {
  time: string;
  count: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
}

interface FieldOption {
  value: string;
  label: string;
  type: string;
}

// 基于实际告警数据结构的可用字段
const AVAILABLE_FIELDS: FieldOption[] = [
  // 基础时间字段
  { value: "@timestamp", label: "Timestamp", type: "date" },
  
  // 告警字段
  { value: "alert.severity", label: "Severity", type: "keyword" },
  { value: "alert.risk_score", label: "Risk Score", type: "number" },
  { value: "alert.confidence", label: "Confidence", type: "number" },
  { value: "alert.type", label: "Alert Type", type: "keyword" },
  { value: "alert.category", label: "Alert Category", type: "keyword" },
  
  // 告警规则字段
  { value: "alert.rule.id", label: "Rule ID", type: "keyword" },
  { value: "alert.rule.name", label: "Rule Name", type: "text" },
  
  // 证据字段
  { value: "alert.evidence.event_type", label: "Event Type", type: "keyword" },
  { value: "alert.evidence.process_name", label: "Process Name", type: "keyword" },
  { value: "alert.evidence.process_cmdline", label: "Command Line", type: "text" },
  { value: "alert.evidence.file_path", label: "File Path", type: "keyword" },
  
  // 元数据字段
  { value: "metadata.collector_id", label: "Collector ID", type: "keyword" },
  { value: "metadata.host", label: "Host Name", type: "keyword" },
  { value: "metadata.source", label: "Source", type: "keyword" },
  { value: "metadata.processor", label: "Processor", type: "keyword" },
  
  // 原始事件字段
  { value: "event.raw.event_id", label: "Event ID", type: "keyword" },
  { value: "event.raw.source", label: "Raw Source", type: "keyword" },
  { value: "event.raw.message.proc.name", label: "Raw Process Name", type: "keyword" },
  { value: "event.raw.message.proc.pid", label: "Raw Process PID", type: "number" },
  { value: "event.raw.message.host", label: "Raw Host", type: "keyword" },
];

const OPERATORS = {
  keyword: [
    { value: "is", label: "is" },
    { value: "is_not", label: "is not" },
    { value: "contains", label: "contains" },
    { value: "starts_with", label: "starts with" },
  ],
  text: [
    { value: "contains", label: "contains" },
    { value: "is", label: "is" },
    { value: "is_not", label: "is not" },
  ],
  number: [
    { value: "equals", label: "equals" },
    { value: "greater_than", label: "greater than" },
    { value: "less_than", label: "less than" },
    { value: "between", label: "between" },
  ],
  ip: [
    { value: "is", label: "is" },
    { value: "is_not", label: "is not" },
    { value: "in_range", label: "in range" },
  ],
  date: [
    { value: "is", label: "is" },
    { value: "after", label: "after" },
    { value: "before", label: "before" },
    { value: "between", label: "between" },
  ],
};

export function OpenSearchAlerts() {
  // 初始化时间范围为过去15天
  const now = new Date();
  const fifteenDaysAgo = new Date(now.getTime() - 15 * 24 * 60 * 60 * 1000);

  const [searchState, setSearchState] = useState<SearchState>({
    query: "",
    dynamicFilters: [],
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
  const [selectedEvent, setSelectedEvent] = useState<SecurityEvent | null>(
    null
  );
  const [detailDialogOpen, setDetailDialogOpen] = useState(false);
  const [aggregations, setAggregations] = useState<any>({});
  const [timelineData, setTimelineData] = useState<TimelineData[]>([]);
  const [showAddFilter, setShowAddFilter] = useState(false);
  const [showTimeRangePicker, setShowTimeRangePicker] = useState(false);
  const [activeTab, setActiveTab] = useState<"absolute" | "relative" | "now">(
    "absolute"
  );
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  
  // 临时时间范围状态，用于在用户确认前暂存
  const [tempTimeRange, setTempTimeRange] = useState({
    from: searchState.timeRange.from,
    to: searchState.timeRange.to,
  });
  const [availableFields, setAvailableFields] =
    useState<FieldOption[]>(AVAILABLE_FIELDS);
  const [error, setError] = useState<string | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<
    "connected" | "disconnected" | "connecting"
  >("connecting");

  // 新增过滤器的状态
  const [newFilter, setNewFilter] = useState({
    field: "",
    operator: "",
    value: "",
  });

  // 防抖相关
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const [debouncedQuery, setDebouncedQuery] = useState(searchState.query);
  
  // EUI SearchBar 相关状态
  const [searchError, setSearchError] = useState<any>(null);
  const [euiQuery, setEuiQuery] = useState<any>(null);
  
  // Index选择器状态
  const [selectedIndex, setSelectedIndex] = useState('sysarmor-alerts*');

  // Index选择器选项配置
  const indexOptions = [
    {
      value: 'sysarmor-alerts*',
      inputDisplay: (
        <EuiHealth color="success" style={{ lineHeight: 'inherit' }}>
          sysarmor-alerts*
        </EuiHealth>
      ),
      'data-test-subj': 'option-alerts',
    },
  ];

  // 严重程度过滤器选项
  const severityFilterOptions = [
    { value: 'critical', view: <EuiHealth color="danger">Critical</EuiHealth> },
    { value: 'high', view: <EuiHealth color="warning">High</EuiHealth> },
    { value: 'medium', view: <EuiHealth color="primary">Medium</EuiHealth> },
    { value: 'low', view: <EuiHealth color="success">Low</EuiHealth> },
  ];

  // 事件类型过滤器选项
  const eventTypeFilterOptions = [
    { value: 'file_access', view: 'File Access' },
    { value: 'process_execution', view: 'Process Execution' },
    { value: 'network_connection', view: 'Network Connection' },
    { value: 'system_call', view: 'System Call' },
  ];

  // EUI SearchBar 过滤器配置
  const searchFilters = [
    {
      type: 'field_value_selection' as const,
      field: 'severity',
      name: 'Severity',
      multiSelect: 'or' as const,
      options: severityFilterOptions,
    },
    {
      type: 'field_value_selection' as const,
      field: 'event_type',
      name: 'Event Type',
      multiSelect: 'or' as const,
      options: eventTypeFilterOptions,
    },
  ];

  // EUI SearchBar Schema配置
  const searchSchema = {
    strict: true,
    fields: {
      severity: {
        type: 'string',
        validate: (value: string) => {
          const validSeverities = ['critical', 'high', 'medium', 'low'];
          if (!validSeverities.includes(value.toLowerCase())) {
            throw new Error(`Invalid severity. Valid values: ${validSeverities.join(', ')}`);
          }
        },
      },
      risk_score: {
        type: 'number',
      },
      event_type: {
        type: 'string',
      },
      host: {
        type: 'string',
      },
      process_name: {
        type: 'string',
      },
      source: {
        type: 'string',
      },
      '@timestamp': {
        type: 'date',
      },
    },
  };

  // 处理Index选择变化
  const handleIndexChange = (value: unknown) => {
    const indexValue = value as string;
    setSelectedIndex(indexValue);
    // 重新搜索数据
    setSearchState((prev) => ({
      ...prev,
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  // 从EUI查询中提取过滤器
  const extractFiltersFromEuiQuery = (query: any) => {
    const filters: DynamicFilter[] = [];
    
    if (!query) return filters;
    
    try {
      // 提取字段过滤器
      const clauses = query.ast?.clauses || [];
      clauses.forEach((clause: any, index: number) => {
        if (clause.type === 'field') {
          const field = clause.field;
          const value = clause.value;
          const operator = clause.match === 'must_not' ? 'is_not' : 'is';
          
          filters.push({
            id: `eui-filter-${index}`,
            field: field,
            operator: operator,
            value: value,
            label: `${field} ${operator} ${value}`,
          });
        }
      });
    } catch (err) {
      console.warn('Failed to extract filters from EUI query:', err);
    }
    
    return filters;
  };

  // 处理EUI SearchBar查询变化
  const handleSearchBarChange = ({ query, error }: any) => {
    if (error) {
      setSearchError(error);
      console.error('Search error:', error);
    } else {
      setSearchError(null);
      setEuiQuery(query);
      
      // 提取EUI查询中的过滤器并添加到左侧sidebar
      const euiFilters = extractFiltersFromEuiQuery(query);
      
      // 将EUI查询转换为Elasticsearch查询字符串
      try {
        // 动态导入EuiSearchBar来访问Query方法
        import('@elastic/eui').then(({ EuiSearchBar }) => {
          const queryString = EuiSearchBar.Query.toESQueryString(query);
          
          setSearchState((prev) => ({
            ...prev,
            query: queryString || "",
            // 将EUI过滤器合并到现有过滤器中，避免重复
            dynamicFilters: [
              ...prev.dynamicFilters.filter(f => !f.id.startsWith('eui-filter-')),
              ...euiFilters
            ],
            pagination: { ...prev.pagination, current: 1 },
          }));
        });
      } catch (err) {
        // 如果转换失败，使用查询文本
        const queryText = query?.text || "";
        setSearchState((prev) => ({
          ...prev,
          query: queryText,
          dynamicFilters: [
            ...prev.dynamicFilters.filter(f => !f.id.startsWith('eui-filter-')),
            ...euiFilters
          ],
          pagination: { ...prev.pagination, current: 1 },
        }));
      }
    }
  };

  // 从真实事件数据生成时间线数据，根据时间范围动态调整
  const generateTimelineData = (events: SecurityEvent[]): TimelineData[] => {
    const data: TimelineData[] = [];
    const { from, to } = searchState.timeRange;
    const timeRangeMs = to.getTime() - from.getTime();

    // 根据时间范围决定时间桶的大小和数量
    let bucketCount: number;
    let bucketSizeMs: number;
    let timeFormat: Intl.DateTimeFormatOptions;

    if (timeRangeMs <= 2 * 60 * 60 * 1000) {
      // 2小时内，按5分钟分桶
      bucketCount = Math.min(24, Math.ceil(timeRangeMs / (5 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { hour: "2-digit", minute: "2-digit" };
    } else if (timeRangeMs <= 24 * 60 * 60 * 1000) {
      // 24小时内，按小时分桶
      bucketCount = Math.min(24, Math.ceil(timeRangeMs / (60 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { hour: "2-digit", minute: "2-digit" };
    } else if (timeRangeMs <= 7 * 24 * 60 * 60 * 1000) {
      // 7天内，按6小时分桶
      bucketCount = Math.min(28, Math.ceil(timeRangeMs / (6 * 60 * 60 * 1000)));
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { month: "2-digit", day: "2-digit", hour: "2-digit" };
    } else {
      // 超过7天，按天分桶
      bucketCount = Math.min(
        30,
        Math.ceil(timeRangeMs / (24 * 60 * 60 * 1000))
      );
      bucketSizeMs = timeRangeMs / bucketCount;
      timeFormat = { month: "2-digit", day: "2-digit" };
    }

    // 生成时间桶
    for (let i = 0; i < bucketCount; i++) {
      const bucketStart = new Date(from.getTime() + i * bucketSizeMs);
      const bucketEnd = new Date(from.getTime() + (i + 1) * bucketSizeMs);

      const timeStr = bucketStart.toLocaleString("zh-CN", timeFormat);

      // 统计该时间段内的事件
      const eventsInBucket = events.filter((event) => {
        const eventTime = new Date(event._source["@timestamp"]);
        return eventTime >= bucketStart && eventTime < bucketEnd;
      });

      // 按严重程度分类统计
      const severityCounts = {
        critical: 0,
        high: 0,
        medium: 0,
        low: 0,
      };

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

  const buildQueryFromFilters = () => {
    let query = debouncedQuery?.trim() || "*";

    // 验证查询语法，避免空查询导致错误
    if (query && query !== "*") {
      // 检查查询是否以冒号结尾但没有值（如 "host:"）
      if (query.match(/\w+:\s*$/)) {
        query = "*"; // 重置为通配符查询
      }
      // 检查是否有不完整的查询语法
      if (query.includes(":") && !query.match(/\w+:\s*\S+/)) {
        query = "*";
      }
    }

    const queryParts = [];

    // 如果有有效的搜索查询，添加到查询部分
    if (query && query !== "*") {
      queryParts.push(`(${query})`);
    }

    // 添加动态过滤器
    searchState.dynamicFilters.forEach((filter) => {
      let filterQuery = "";
      switch (filter.operator) {
        case "is":
          filterQuery = `${filter.field}:"${filter.value}"`;
          break;
        case "is_not":
          filterQuery = `NOT ${filter.field}:"${filter.value}"`;
          break;
        case "contains":
          filterQuery = `${filter.field}:*${filter.value}*`;
          break;
        case "starts_with":
          filterQuery = `${filter.field}:${filter.value}*`;
          break;
        case "equals":
          filterQuery = `${filter.field}:${filter.value}`;
          break;
        case "greater_than":
          filterQuery = `${filter.field}:>${filter.value}`;
          break;
        case "less_than":
          filterQuery = `${filter.field}:<${filter.value}`;
          break;
        default:
          filterQuery = `${filter.field}:"${filter.value}"`;
      }
      queryParts.push(`(${filterQuery})`);
    });

    // 添加时间范围过滤器
    const fromTime = searchState.timeRange.from.toISOString();
    const toTime = searchState.timeRange.to.toISOString();
    queryParts.push(`@timestamp:[${fromTime} TO ${toTime}]`);

    // 如果没有其他查询条件，使用通配符
    if (queryParts.length === 1) {
      return `* AND ${queryParts[0]}`;
    }

    return queryParts.join(" AND ");
  };

  // 动态检测事件数据中存在的字段 - 适配新的告警数据结构
  const detectAvailableFields = (events: SecurityEvent[]) => {
    if (events.length === 0) {
      setAvailableFields(AVAILABLE_FIELDS);
      return;
    }

    const detectedFields = new Set<string>();

    // 分析前几个事件的字段结构
    const sampleEvents = events.slice(0, Math.min(10, events.length));

    sampleEvents.forEach((event) => {
      const source = event._source;

      // 基础时间字段
      if (source["@timestamp"]) detectedFields.add("@timestamp");
      
      // 告警字段
      if (source.alert?.severity) detectedFields.add("alert.severity");
      if (source.alert?.risk_score !== undefined) detectedFields.add("alert.risk_score");
      if (source.alert?.confidence !== undefined) detectedFields.add("alert.confidence");
      if (source.alert?.type) detectedFields.add("alert.type");
      if (source.alert?.category) detectedFields.add("alert.category");
      
      // 告警规则字段
      if (source.alert?.rule?.id) detectedFields.add("alert.rule.id");
      if (source.alert?.rule?.name) detectedFields.add("alert.rule.name");
      
      // 证据字段
      if (source.alert?.evidence?.event_type) detectedFields.add("alert.evidence.event_type");
      if (source.alert?.evidence?.process_name) detectedFields.add("alert.evidence.process_name");
      if (source.alert?.evidence?.process_cmdline) detectedFields.add("alert.evidence.process_cmdline");
      if (source.alert?.evidence?.file_path) detectedFields.add("alert.evidence.file_path");
      
      // 元数据字段
      if (source.metadata?.collector_id) detectedFields.add("metadata.collector_id");
      if (source.metadata?.host) detectedFields.add("metadata.host");
      if (source.metadata?.source) detectedFields.add("metadata.source");
      if (source.metadata?.processor) detectedFields.add("metadata.processor");
      
      // 原始事件字段
      if (source.event?.raw?.event_id) detectedFields.add("event.raw.event_id");
      if (source.event?.raw?.source) detectedFields.add("event.raw.source");
      if (source.event?.raw?.message?.["proc.name"]) detectedFields.add("event.raw.message.proc.name");
      if (source.event?.raw?.message?.["proc.pid"]) detectedFields.add("event.raw.message.proc.pid");
      if (source.event?.raw?.message?.host) detectedFields.add("event.raw.message.host");
    });

    // 过滤出实际存在的字段
    const filteredFields = AVAILABLE_FIELDS.filter((field) =>
      detectedFields.has(field.value)
    );

    // 如果没有检测到任何字段，保持原有字段列表
    if (filteredFields.length > 0) {
      setAvailableFields(filteredFields);
    } else {
      setAvailableFields(AVAILABLE_FIELDS);
    }
  };

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      setConnectionStatus("connecting");

      const params: any = {
        index: "sysarmor-alerts-test",
        size: searchState.pagination.size,
        from:
          (searchState.pagination.current - 1) * searchState.pagination.size,
        q: buildQueryFromFilters(),
      };

      if (searchState.sortField && searchState.sortDirection) {
        params.sort = `${searchState.sortField}:${searchState.sortDirection}`;
      }

      // 分别处理事件搜索和聚合，允许部分失败
      let eventsResponse: any = null;
      let aggregationsResponse: any = null;
      let eventsError = false;
      let aggregationsError = false;

      try {
        eventsResponse = await apiClient.searchSecurityEvents(params);
      } catch (eventsErr) {
        console.error("Failed to fetch events:", eventsErr);
        eventsError = true;
      }

      try {
        aggregationsResponse = await apiClient.getEventAggregations({
          index: "sysarmor-alerts-test",
        });
      } catch (aggregationsErr) {
        console.warn("Failed to fetch aggregations:", aggregationsErr);
        aggregationsError = true;
      }

      if (eventsError && aggregationsError) {
        throw new Error("Both events and aggregations failed to load");
      }

      // 处理事件数据
      const events = eventsResponse
        ? eventsResponse.hits?.hits || eventsResponse.data?.hits?.hits || []
        : [];
      const total = eventsResponse
        ? eventsResponse.hits?.total?.value ||
          eventsResponse.hits?.total ||
          eventsResponse.data?.hits?.total?.value ||
          eventsResponse.data?.hits?.total ||
          0
        : 0;

      setEvents(events);
      setSearchState((prev) => ({
        ...prev,
        pagination: {
          ...prev.pagination,
          total: typeof total === "number" ? total : total.value || 0,
        },
      }));

      // 动态检测可用字段
      detectAvailableFields(events);

      // 生成时间线数据
      setTimelineData(generateTimelineData(events));

      // 处理聚合数据
      const aggregationsData = aggregationsResponse
        ? aggregationsResponse.data || aggregationsResponse
        : {};
      setAggregations(aggregationsData);

      setConnectionStatus("connected");

      // 如果部分服务失败，显示警告
      if (eventsError || aggregationsError) {
        setError("部分数据加载失败，显示的信息可能不完整");
      }
    } catch (error) {
      console.error("❌ OpenSearch API 调用失败:", error);
      setConnectionStatus("disconnected");

      if (error instanceof ApiError) {
        setError(`OpenSearch 连接失败: ${error.message}`);
      } else {
        setError("无法连接到 OpenSearch 服务，请检查后端服务状态");
      }

      setEvents([]);
      setAggregations({});
      setTimelineData(generateTimelineData([]));
      setSearchState((prev) => ({
        ...prev,
        pagination: { ...prev.pagination, total: 0 },
      }));
      // 重置为默认字段列表
      setAvailableFields(AVAILABLE_FIELDS);
    } finally {
      setLoading(false);
    }
  };

  // 防抖搜索效果
  useEffect(() => {
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }

    searchTimeoutRef.current = setTimeout(() => {
      setDebouncedQuery(searchState.query);
    }, 500); // 500ms 防抖延迟

    return () => {
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    };
  }, [searchState.query]);

  useEffect(() => {
    fetchData();
  }, [
    debouncedQuery,
    searchState.dynamicFilters,
    searchState.timeRange,
    searchState.pagination.current,
    searchState.sortField,
    searchState.sortDirection,
  ]);

  const handleSearch = (query: string) => {
    setSearchState((prev) => ({
      ...prev,
      query,
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  const handleSort = (field: string) => {
    setSearchState((prev) => ({
      ...prev,
      sortField: field,
      sortDirection:
        prev.sortField === field && prev.sortDirection === "desc"
          ? "asc"
          : "desc",
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  const handleAddFilter = () => {
    if (!newFilter.field || !newFilter.operator || !newFilter.value) return;

    const fieldOption = AVAILABLE_FIELDS.find(
      (f) => f.value === newFilter.field
    );
    const operatorOption = OPERATORS[
      fieldOption?.type as keyof typeof OPERATORS
    ]?.find((o) => o.value === newFilter.operator);

    const filter: DynamicFilter = {
      id: Date.now().toString(),
      field: newFilter.field,
      operator: newFilter.operator,
      value: newFilter.value,
      label: `${fieldOption?.label} ${operatorOption?.label} ${newFilter.value}`,
    };

    setSearchState((prev) => ({
      ...prev,
      dynamicFilters: [...prev.dynamicFilters, filter],
      pagination: { ...prev.pagination, current: 1 },
    }));

    setNewFilter({ field: "", operator: "", value: "" });
    setShowAddFilter(false);
  };

  const handleRemoveFilter = (filterId: string) => {
    setSearchState((prev) => ({
      ...prev,
      dynamicFilters: prev.dynamicFilters.filter((f) => f.id !== filterId),
      pagination: { ...prev.pagination, current: 1 },
    }));
  };

  const handleTimeRangeChange = (from: Date, to: Date) => {
    const label = `${from.toLocaleDateString()} → ${to.toLocaleDateString()}`;
    setSearchState((prev) => ({
      ...prev,
      timeRange: { from, to, label },
      pagination: { ...prev.pagination, current: 1 },
    }));
    setShowTimeRangePicker(false);
  };

  // 应用临时时间范围
  const applyTempTimeRange = () => {
    handleTimeRangeChange(tempTimeRange.from, tempTimeRange.to);
  };

  // 重置临时时间范围
  const resetTempTimeRange = () => {
    setTempTimeRange({
      from: searchState.timeRange.from,
      to: searchState.timeRange.to,
    });
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

  const getSeverityBadge = (severity: string) => {
    const severityMap: Record<string, { label: string; color: string }> = {
      critical: {
        label: "Critical",
        color: "bg-red-100 text-red-800 border-red-200",
      },
      high: {
        label: "High",
        color: "bg-orange-100 text-orange-800 border-orange-200",
      },
      medium: {
        label: "Medium",
        color: "bg-yellow-100 text-yellow-800 border-yellow-200",
      },
      low: { label: "Low", color: "bg-blue-100 text-blue-800 border-blue-200" },
      info: {
        label: "Info",
        color: "bg-gray-100 text-gray-800 border-gray-200",
      },
    };

    const info = severityMap[severity?.toLowerCase()] || {
      label: severity || "Unknown",
      color: "bg-gray-100 text-gray-800 border-gray-200",
    };

    return (
      <span
        className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${info.color}`}
      >
        {info.label}
      </span>
    );
  };

  const getRiskScoreBadge = (score: number) => {
    let color = "bg-gray-100 text-gray-800 border-gray-200";
    if (score >= 80) color = "bg-red-100 text-red-800 border-red-200";
    else if (score >= 60)
      color = "bg-orange-100 text-orange-800 border-orange-200";
    else if (score >= 40)
      color = "bg-yellow-100 text-yellow-800 border-yellow-200";
    else if (score >= 20) color = "bg-blue-100 text-blue-800 border-blue-200";

    return (
      <span
        className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium border ${color}`}
      >
        {score}
      </span>
    );
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

  const getSortIcon = (field: string) => {
    if (searchState.sortField !== field) {
      return <ArrowUpDown className="h-3.5 w-3.5 text-gray-400" />;
    }
    return searchState.sortDirection === "asc" ? (
      <ArrowUp className="h-3.5 w-3.5 text-blue-600" />
    ) : (
      <ArrowDown className="h-3.5 w-3.5 text-blue-600" />
    );
  };

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

  const selectedFieldType = AVAILABLE_FIELDS.find(
    (f) => f.value === newFilter.field
  )?.type;
  const availableOperators = selectedFieldType
    ? OPERATORS[selectedFieldType as keyof typeof OPERATORS] || []
    : [];

  return (
    <div className="@container/main flex flex-1 flex-col overflow-hidden">
      <div className="flex h-full bg-gray-50">
        {/* 左侧过滤器面板 */}
        <div className="w-80 bg-white border-r border-gray-200 flex flex-col">
          {/* Index 选择器 - EUI SuperSelect版 */}
          <div className="px-4 py-4 border-b border-gray-200 h-[72px] flex items-center">
            <div className="w-full">
              <EuiSuperSelect
                options={indexOptions}
                valueOfSelected={selectedIndex}
                onChange={handleIndexChange}
                fullWidth
              />
            </div>
          </div>

          {/* Filters 标题和添加按钮 */}
          <div className="px-4 py-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-gray-900 flex items-center gap-2">
              <Filter className="h-4 w-4" />
              Filters
            </h2>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs text-blue-600 hover:text-blue-700 hover:bg-blue-50"
              onClick={() => setShowAddFilter(true)}
            >
              <Plus className="h-3 w-3 mr-1" />
              Add filter
            </Button>
          </div>

          {/* 快速过滤器按钮 */}
          <div className="px-4 pb-3 space-y-2">
            <div className="text-xs font-medium text-gray-700 mb-2">Quick Filters</div>
            <div className="flex flex-wrap gap-1">
              {/* Severity快速过滤器 */}
              {['critical', 'high', 'medium', 'low'].map((severity) => {
                const isActive = searchState.dynamicFilters.some(f => 
                  f.field === 'severity' && f.value === severity
                );
                const colorMap = {
                  critical: 'bg-red-50 text-red-700 border-red-200 hover:bg-red-100',
                  high: 'bg-orange-50 text-orange-700 border-orange-200 hover:bg-orange-100',
                  medium: 'bg-yellow-50 text-yellow-700 border-yellow-200 hover:bg-yellow-100',
                  low: 'bg-green-50 text-green-700 border-green-200 hover:bg-green-100',
                };
                
                return (
                  <button
                    key={severity}
                    className={`px-2 py-1 text-xs rounded border transition-colors ${
                      isActive 
                        ? `${colorMap[severity as keyof typeof colorMap]} ring-1 ring-current` 
                        : `${colorMap[severity as keyof typeof colorMap]}`
                    }`}
                    onClick={() => {
                      if (isActive) {
                        // 移除过滤器
                        setSearchState(prev => ({
                          ...prev,
                          dynamicFilters: prev.dynamicFilters.filter(f => 
                            !(f.field === 'severity' && f.value === severity)
                          ),
                          pagination: { ...prev.pagination, current: 1 },
                        }));
                      } else {
                        // 添加过滤器
                        const newFilter: DynamicFilter = {
                          id: `quick-severity-${severity}`,
                          field: 'severity',
                          operator: 'is',
                          value: severity,
                          label: `Severity is ${severity}`,
                        };
                        setSearchState(prev => ({
                          ...prev,
                          dynamicFilters: [...prev.dynamicFilters, newFilter],
                          pagination: { ...prev.pagination, current: 1 },
                        }));
                      }
                    }}
                  >
                    {severity}
                  </button>
                );
              })}
            </div>
          </div>

          {/* 动态过滤器列表 */}
          <div className="flex-1 overflow-y-auto p-4">
            {searchState.dynamicFilters.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <Filter className="h-8 w-8 mx-auto mb-2 text-gray-300" />
                <p className="text-sm">No filters applied</p>
                <p className="text-xs text-gray-400">
                  Click "Add filter" to get started
                </p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {searchState.dynamicFilters.map((filter) => {
                  const fieldOption = AVAILABLE_FIELDS.find(
                    (f) => f.value === filter.field
                  );
                  const operatorOption = OPERATORS[
                    fieldOption?.type as keyof typeof OPERATORS
                  ]?.find((o) => o.value === filter.operator);

                  // 区分EUI生成的过滤器和手动添加的过滤器
                  const isEuiFilter = filter.id.startsWith('eui-filter-');
                  const filterBgColor = isEuiFilter 
                    ? "bg-green-50/80 border-green-200/80 hover:bg-green-100/80" 
                    : "bg-blue-50/80 border-blue-200/80 hover:bg-blue-100/80";
                  const filterTextColor = isEuiFilter ? "text-green-800" : "text-blue-800";
                  const filterValueColor = isEuiFilter ? "text-green-900" : "text-blue-900";
                  const filterButtonColor = isEuiFilter 
                    ? "text-green-500 hover:text-green-700 hover:bg-green-200/80" 
                    : "text-blue-500 hover:text-blue-700 hover:bg-blue-200/80";

                  return (
                    <div
                      key={filter.id}
                      className={`group flex items-center gap-2 p-2 rounded text-xs transition-colors ${filterBgColor}`}
                    >
                      <div className="flex-1 min-w-0">
                        <div className={`flex items-center gap-1 ${filterTextColor}`}>
                          {isEuiFilter && (
                            <Search className="h-3 w-3 flex-shrink-0" />
                          )}
                          <span className="font-medium truncate">
                            {fieldOption?.label || filter.field}
                          </span>
                          <span className={`${isEuiFilter ? 'text-green-600' : 'text-blue-600'} flex-shrink-0`}>
                            {operatorOption?.label || filter.operator}
                          </span>
                        </div>
                        <div className={`font-mono truncate mt-0.5 ${filterValueColor}`}>
                          "{filter.value}"
                        </div>
                        {isEuiFilter && (
                          <div className="text-xs text-green-600 mt-0.5">
                            From search query
                          </div>
                        )}
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className={`h-5 w-5 p-0 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0 ${filterButtonColor}`}
                        onClick={() => handleRemoveFilter(filter.id)}
                      >
                        <X className="h-3 w-3" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* 右侧主内容区域 */}
        <div className="flex-1 bg-white flex flex-col min-w-0">
          {/* 搜索框和时间筛选框 */}
          <div className="bg-white border-b border-gray-200 px-6 py-4 relative h-[72px] flex items-center">
            <div className="flex items-center gap-3 w-full min-w-0">
              {/* EUI SearchBar */}
              <div className="flex-1 min-w-0">
                <div style={{ fontSize: '14px' }}>
                  <EuiSearchBar
                    box={{
                      placeholder: 'Search events... (e.g., severity:critical host:server-01)',
                      incremental: true,
                      schema: searchSchema,
                    }}
                    onChange={handleSearchBarChange}
                  />
                </div>
              </div>

              {/* 时间范围选择器 */}
              <div className="flex-shrink-0" style={{ fontSize: '14px' }}>
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
            </div>
          </div>

          {/* 记录命中数量显示 */}
          <div className="bg-white px-6 py-1 text-center">
            <span className="text-sm text-gray-600">
              <span className="font-medium text-gray-900">
                {searchState.pagination.total.toLocaleString()}
              </span>{" "}
              hits
            </span>
          </div>

          {/* 时间柱状图 */}
          <div className="bg-white px-6 py-2">
            <div className="h-32">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart
                  data={timelineData}
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
          </div>

          {/* 表格内容 */}
          <div className="flex-1 overflow-auto border-t border-gray-200">
            {loading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-6 w-6 animate-spin mr-2 text-blue-600" />
                <span className="text-sm text-gray-600">Loading events...</span>
              </div>
            ) : events.length === 0 ? (
              <div className="text-center py-8 text-gray-500">
                <Search className="h-8 w-8 mx-auto mb-3 text-gray-300" />
                <p className="text-sm font-medium text-gray-600">
                  No events found
                </p>
                <p className="text-xs text-gray-500">
                  Try adjusting your search query or filters
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-full">
                  <TableHeader className="bg-gray-50/80 sticky top-0 border-b border-gray-200">
                    <TableRow className="hover:bg-gray-50/80">
                      <TableHead className="w-10 px-3 py-2 text-left">
                        <Checkbox
                          checked={
                            searchState.selectedEvents.length === events.length
                          }
                          onCheckedChange={handleSelectAll}
                          className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                        />
                      </TableHead>
                      <TableHead className="w-6 px-1 py-2 text-left">
                        {/* 展开列 */}
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[140px]"
                        onClick={() => handleSort("@timestamp")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Clock className="h-3.5 w-3.5" />
                          Time
                          {getSortIcon("@timestamp")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[90px]"
                        onClick={() => handleSort("severity")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <AlertTriangle className="h-3.5 w-3.5" />
                          Severity
                          {getSortIcon("severity")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
                        onClick={() => handleSort("event_type")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Shield className="h-3.5 w-3.5" />
                          Event Type
                          {getSortIcon("event_type")}
                        </div>
                      </TableHead>
                      <TableHead
                        className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[70px]"
                        onClick={() => handleSort("risk_score")}
                      >
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Flag className="h-3.5 w-3.5" />
                          Risk Score
                          {getSortIcon("risk_score")}
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[80px] hidden md:table-cell">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Database className="h-3.5 w-3.5" />
                          Source
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[80px] hidden lg:table-cell">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          <Server className="h-3.5 w-3.5" />
                          Host
                        </div>
                      </TableHead>
                      <TableHead className="px-3 py-2 text-left min-w-[160px] hidden xl:table-cell">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          Message
                        </div>
                      </TableHead>
                      <TableHead className="w-10 px-3 py-2 text-left">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                          Actions
                        </div>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {events.map((event) => (
                      <React.Fragment key={event._id}>
                        <TableRow
                          className={`border-b border-gray-100/60 hover:bg-gray-50/50 transition-colors ${
                            searchState.selectedEvents.includes(event._id)
                              ? "bg-blue-50/60"
                              : ""
                          }`}
                        >
                          <TableCell className="px-3 py-2">
                            <Checkbox
                              checked={searchState.selectedEvents.includes(
                                event._id
                              )}
                              onCheckedChange={() =>
                                handleSelectEvent(event._id)
                              }
                              className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                            />
                          </TableCell>
                          <TableCell className="px-1 py-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => toggleRowExpansion(event._id)}
                              className="h-5 w-5 p-0 hover:bg-gray-100/80"
                            >
                              <ChevronRight
                                className={`h-3 w-3 transition-transform ${
                                  expandedRows.has(event._id) ? "rotate-90" : ""
                                }`}
                              />
                            </Button>
                          </TableCell>
                          <TableCell className="px-3 py-2 font-mono text-xs text-gray-600">
                            {formatTimestamp(event._source["@timestamp"])}
                          </TableCell>
                          <TableCell className="px-3 py-2">
                            {getSeverityBadge(
                              event._source.alert?.severity || "unknown"
                            )}
                          </TableCell>
                          <TableCell className="px-3 py-2">
                            <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border bg-gray-100/80 text-gray-700 border-gray-200/80">
                              {event._source.alert?.evidence?.event_type || "Unknown"}
                            </span>
                          </TableCell>
                          <TableCell className="px-3 py-2">
                            {getRiskScoreBadge(event._source.alert?.risk_score || 0)}
                          </TableCell>
                          <TableCell className="px-3 py-2 font-mono text-xs text-gray-600 hidden md:table-cell">
                            <div
                              className="truncate max-w-[80px]"
                              title={event._source.metadata?.source || "Unknown"}
                            >
                              {event._source.metadata?.source || "Unknown"}
                            </div>
                          </TableCell>
                          <TableCell className="px-3 py-2 text-xs hidden lg:table-cell">
                            <div
                              className="truncate max-w-[80px]"
                              title={
                                event._source.metadata?.host ||
                                event._source.metadata?.collector_id ||
                                "N/A"
                              }
                            >
                              {event._source.metadata?.host ||
                                event._source.metadata?.collector_id ||
                                "N/A"}
                            </div>
                          </TableCell>
                          <TableCell className="px-3 py-2 hidden xl:table-cell">
                            <div
                              className="truncate text-xs text-gray-600 max-w-[160px]"
                              title={event._source.alert?.rule?.title}
                            >
                              {event._source.alert?.rule?.title}
                            </div>
                          </TableCell>
                          <TableCell className="px-3 py-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => {
                                setSelectedEvent(event);
                                setDetailDialogOpen(true);
                              }}
                              className="h-6 w-6 p-0 hover:bg-gray-100/80"
                            >
                              <Eye className="h-3.5 w-3.5" />
                            </Button>
                          </TableCell>
                        </TableRow>
                        {/* 展开行 - 子表格版本 */}
                        {expandedRows.has(event._id) && (
                          <TableRow className="bg-gradient-to-r from-slate-50 to-blue-50/30 border-l-2 border-blue-400">
                            <TableCell colSpan={10} className="px-6 py-4">
                              <div className="bg-white rounded-lg border border-gray-200 shadow-sm overflow-hidden">
                                <Table className="text-xs">
                                  <TableBody>
                                    {/* 系统信息分组 */}
                                    <TableRow className="bg-purple-50/50">
                                      <TableCell colSpan={2} className="px-4 py-2 font-semibold text-purple-800 text-xs">
                                        <div className="flex items-center gap-2">
                                          <Server className="h-3 w-3" />
                                          系统信息
                                        </div>
                                      </TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">主机名</TableCell>
                                      <TableCell className="px-4 py-2 font-mono text-gray-900">{event._source.metadata?.host || "N/A"}</TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">Collector ID</TableCell>
                                      <TableCell className="px-4 py-2 font-mono text-gray-900 break-all">{event._source.metadata?.collector_id || "N/A"}</TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">处理器</TableCell>
                                      <TableCell className="px-4 py-2 font-mono text-gray-900">{event._source.metadata?.processor || "N/A"}</TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">置信度</TableCell>
                                      <TableCell className="px-4 py-2">
                                        <Badge variant="secondary" className="text-xs">
                                          {event._source.alert?.confidence ? `${(event._source.alert.confidence * 100).toFixed(1)}%` : "N/A"}
                                        </Badge>
                                      </TableCell>
                                    </TableRow>

                                    {/* 告警规则分组 */}
                                    <TableRow className="bg-blue-50/50">
                                      <TableCell colSpan={2} className="px-4 py-2 font-semibold text-blue-800 text-xs">
                                        <div className="flex items-center gap-2">
                                          <Shield className="h-3 w-3" />
                                          告警规则
                                        </div>
                                      </TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">规则名称</TableCell>
                                      <TableCell className="px-4 py-2 text-gray-900">{event._source.alert?.rule?.name || "N/A"}</TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">规则ID</TableCell>
                                      <TableCell className="px-4 py-2 font-mono text-gray-900">{event._source.alert?.rule?.id || "N/A"}</TableCell>
                                    </TableRow>
                                    {event._source.alert?.rule?.description && (
                                      <TableRow className="hover:bg-gray-50/50">
                                        <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">描述</TableCell>
                                        <TableCell className="px-4 py-2 text-gray-700 leading-relaxed">{event._source.alert.rule.description}</TableCell>
                                      </TableRow>
                                    )}

                                    {/* 证据信息分组 */}
                                    <TableRow className="bg-orange-50/50">
                                      <TableCell colSpan={2} className="px-4 py-2 font-semibold text-orange-800 text-xs">
                                        <div className="flex items-center gap-2">
                                          <AlertTriangle className="h-3 w-3" />
                                          证据信息
                                        </div>
                                      </TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">事件类型</TableCell>
                                      <TableCell className="px-4 py-2">
                                        <Badge variant="outline" className="text-xs">
                                          {event._source.alert?.evidence?.event_type || "N/A"}
                                        </Badge>
                                      </TableCell>
                                    </TableRow>
                                    <TableRow className="hover:bg-gray-50/50">
                                      <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">进程名称</TableCell>
                                      <TableCell className="px-4 py-2 font-mono text-gray-900">{event._source.alert?.evidence?.process_name || "N/A"}</TableCell>
                                    </TableRow>
                                    {event._source.alert?.evidence?.file_path && (
                                      <TableRow className="hover:bg-gray-50/50">
                                        <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">文件路径</TableCell>
                                        <TableCell className="px-4 py-2 font-mono text-gray-900 break-all">{event._source.alert.evidence.file_path}</TableCell>
                                      </TableRow>
                                    )}
                                    {event._source.alert?.evidence?.process_cmdline && (
                                      <TableRow className="hover:bg-gray-50/50">
                                        <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium align-top">命令行</TableCell>
                                        <TableCell className="px-4 py-2">
                                          <div className="p-2 bg-gray-50 rounded border font-mono text-xs break-all leading-relaxed max-h-20 overflow-y-auto">
                                            {event._source.alert.evidence.process_cmdline}
                                          </div>
                                        </TableCell>
                                      </TableRow>
                                    )}
                                  </TableBody>
                                </Table>
                              </div>
                            </TableCell>
                          </TableRow>
                        )}
                      </React.Fragment>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </div>
      </div>


      {/* 添加过滤器对话框 */}
      <Dialog open={showAddFilter} onOpenChange={setShowAddFilter}>
        <DialogContent className="sm:max-w-[420px] p-0">
          <DialogHeader className="px-4 py-3 border-b border-gray-200/80">
            <DialogTitle className="text-sm font-medium text-gray-900 flex items-center gap-2">
              <Filter className="h-4 w-4 text-blue-600" />
              Add Filter
            </DialogTitle>
            <DialogDescription className="text-xs text-gray-600 mt-1">
              Create a new filter to refine your search results
            </DialogDescription>
          </DialogHeader>

          <div className="px-4 py-3 space-y-3">
            {/* Field Selection */}
            <div className="space-y-1.5">
              <Label
                htmlFor="field"
                className="text-xs font-medium text-gray-700"
              >
                Field
              </Label>
              <Select
                value={newFilter.field}
                onValueChange={(value) =>
                  setNewFilter((prev) => ({
                    ...prev,
                    field: value,
                    operator: "",
                    value: "",
                  }))
                }
              >
                <SelectTrigger className="h-8 text-xs w-full">
                  <SelectValue placeholder="Select a field" />
                </SelectTrigger>
                <SelectContent>
                  {availableFields.map((field) => (
                    <SelectItem
                      key={field.value}
                      value={field.value}
                      className="text-xs"
                    >
                      <div className="flex items-center justify-between w-full">
                        <span>{field.label}</span>
                        <span className="text-xs text-gray-500 ml-2">
                          ({field.type})
                        </span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Operator Selection */}
            {newFilter.field && (
              <div className="space-y-1.5">
                <Label
                  htmlFor="operator"
                  className="text-xs font-medium text-gray-700"
                >
                  Operator
                </Label>
                <Select
                  value={newFilter.operator}
                  onValueChange={(value) =>
                    setNewFilter((prev) => ({ ...prev, operator: value }))
                  }
                >
                  <SelectTrigger className="h-8 text-xs w-full">
                    <SelectValue placeholder="Select an operator" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableOperators.map((operator) => (
                      <SelectItem
                        key={operator.value}
                        value={operator.value}
                        className="text-xs"
                      >
                        {operator.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

            {/* Value Input */}
            {newFilter.operator && (
              <div className="space-y-1.5">
                <Label
                  htmlFor="value"
                  className="text-xs font-medium text-gray-700"
                >
                  Value
                </Label>
                <Input
                  id="value"
                  placeholder="Enter filter value"
                  value={newFilter.value}
                  onChange={(e) =>
                    setNewFilter((prev) => ({ ...prev, value: e.target.value }))
                  }
                  className="h-8 text-xs w-full"
                />
              </div>
            )}

            {/* Preview */}
            {newFilter.field && newFilter.operator && newFilter.value && (
              <div className="mt-3 p-2 bg-blue-50/80 border border-blue-200/80 rounded-md">
                <div className="text-xs text-gray-600 mb-1">Preview:</div>
                <div className="text-xs font-mono text-blue-800">
                  {
                    AVAILABLE_FIELDS.find((f) => f.value === newFilter.field)
                      ?.label
                  }{" "}
                  {
                    OPERATORS[
                      AVAILABLE_FIELDS.find((f) => f.value === newFilter.field)
                        ?.type as keyof typeof OPERATORS
                    ]?.find((o) => o.value === newFilter.operator)?.label
                  }{" "}
                  "{newFilter.value}"
                </div>
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex items-center justify-end gap-2 px-4 py-3 border-t border-gray-200/80 bg-gray-50/50">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowAddFilter(false)}
              className="h-7 px-3 text-xs"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleAddFilter}
              disabled={
                !newFilter.field || !newFilter.operator || !newFilter.value
              }
              className="h-7 px-3 text-xs bg-blue-600 hover:bg-blue-700"
            >
              Add Filter
            </Button>
          </div>
        </DialogContent>
      </Dialog>

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
                    <h4 className="font-semibold mb-3 text-gray-800">
                      Basic Information
                    </h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-gray-600">Time:</span>
                        <span className="font-mono">
                          {formatTimestamp(selectedEvent._source["@timestamp"])}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">Type:</span>
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800 border border-gray-200">
                          {selectedEvent._source.alert?.evidence?.event_type}
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
                      <div className="flex justify-between">
                        <span className="text-gray-600">Source:</span>
                        <span className="font-mono">
                          {selectedEvent._source.metadata?.source}
                        </span>
                      </div>
                    </div>
                  </div>
                  <div>
                    <h4 className="font-semibold mb-3 text-gray-800">
                      Host Information
                    </h4>
                    <div className="space-y-2 text-sm">
                      <div className="flex justify-between">
                        <span className="text-gray-600">Host Name:</span>
                        <span>{selectedEvent._source.metadata?.host || "N/A"}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">IP Address:</span>
                        <span className="font-mono">
                          {selectedEvent._source.event?.raw?.message?.host || "N/A"}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-600">Collector ID:</span>
                        <span className="font-mono">
                          {selectedEvent._source.metadata?.collector_id || "N/A"}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div>
                  <h4 className="font-semibold mb-3 text-gray-800">
                    Alert Rule
                  </h4>
                  <div className="p-4 bg-gray-50 rounded-lg text-sm border">
                    <div className="space-y-2">
                      <div><strong>Title:</strong> {selectedEvent._source.alert?.rule?.title}</div>
                      <div><strong>Description:</strong> {selectedEvent._source.alert?.rule?.description}</div>
                      <div><strong>Mitigation:</strong> {selectedEvent._source.alert?.rule?.mitigation}</div>
                    </div>
                  </div>
                </div>

                <div>
                  <h4 className="font-semibold mb-3 text-gray-800">
                    Raw Event Data (JSON)
                  </h4>
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
