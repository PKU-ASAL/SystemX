"use client";

import React from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Eye,
  AlertTriangle,
  Shield,
  Clock,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Database,
  Server,
  Flag,
  ChevronRight,
} from "lucide-react";
import { SecurityEvent, SearchState } from "@/types/security-events";

interface EventsTableProps {
  events: SecurityEvent[];
  searchState: SearchState;
  expandedRows: Set<string>;
  onSelectEvent: (eventId: string) => void;
  onSelectAll: () => void;
  onSort: (field: string) => void;
  onToggleExpansion: (eventId: string) => void;
  onViewDetails: (event: SecurityEvent) => void;
}

export function EventsTable({
  events,
  searchState,
  expandedRows,
  onSelectEvent,
  onSelectAll,
  onSort,
  onToggleExpansion,
  onViewDetails,
}: EventsTableProps) {
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

  return (
    <div className="overflow-x-auto">
      <Table className="min-w-full">
        <TableHeader className="bg-gray-50/80 sticky top-0 border-b border-gray-200">
          <TableRow className="hover:bg-gray-50/80">
            <TableHead className="w-10 px-3 py-2 text-left">
              <Checkbox
                checked={searchState.selectedEvents.length === events.length}
                onCheckedChange={onSelectAll}
                className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
              />
            </TableHead>
            <TableHead className="w-6 px-1 py-2 text-left"></TableHead>
            <TableHead
              className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[140px]"
              onClick={() => onSort("@timestamp")}
            >
              <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                <Clock className="h-3.5 w-3.5" />
                Time
                {getSortIcon("@timestamp")}
              </div>
            </TableHead>
            <TableHead
              className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[90px]"
              onClick={() => onSort("severity")}
            >
              <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                <AlertTriangle className="h-3.5 w-3.5" />
                Severity
                {getSortIcon("severity")}
              </div>
            </TableHead>
            <TableHead
              className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[100px]"
              onClick={() => onSort("event_type")}
            >
              <div className="flex items-center gap-1.5 text-xs font-medium text-gray-700">
                <Shield className="h-3.5 w-3.5" />
                Event Type
                {getSortIcon("event_type")}
              </div>
            </TableHead>
            <TableHead
              className="px-3 py-2 text-left cursor-pointer hover:bg-gray-100/80 transition-colors min-w-[70px]"
              onClick={() => onSort("risk_score")}
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
                  searchState.selectedEvents.includes(event._id) ? "bg-blue-50/60" : ""
                }`}
              >
                <TableCell className="px-3 py-2">
                  <Checkbox
                    checked={searchState.selectedEvents.includes(event._id)}
                    onCheckedChange={() => onSelectEvent(event._id)}
                    className="h-3.5 w-3.5 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                  />
                </TableCell>
                <TableCell className="px-1 py-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onToggleExpansion(event._id)}
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
                  {getSeverityBadge(event._source.alert?.severity || "unknown")}
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
                    onClick={() => onViewDetails(event)}
                    className="h-6 w-6 p-0 hover:bg-gray-100/80"
                  >
                    <Eye className="h-3.5 w-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
              {/* 展开行 */}
              {expandedRows.has(event._id) && (
                <TableRow className="bg-gradient-to-r from-slate-50 to-blue-50/30 border-l-2 border-blue-400">
                  <TableCell colSpan={10} className="px-6 py-4">
                    <div className="bg-white rounded-lg border border-gray-200 shadow-sm overflow-hidden">
                      <Table className="text-xs">
                        <TableBody>
                          {/* 告警信息分组 */}
                          <TableRow className="bg-red-50/50">
                            <TableCell colSpan={2} className="px-4 py-2 font-semibold text-red-800 text-xs">
                              <div className="flex items-center gap-2">
                                <AlertTriangle className="h-3 w-3" />
                                告警信息
                              </div>
                            </TableCell>
                          </TableRow>
                          <TableRow className="hover:bg-gray-50/50">
                            <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">告警ID</TableCell>
                            <TableCell className="px-4 py-2 font-mono text-gray-900">{event._source.alert?.id || "N/A"}</TableCell>
                          </TableRow>
                          <TableRow className="hover:bg-gray-50/50">
                            <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">告警类型</TableCell>
                            <TableCell className="px-4 py-2 text-gray-900">{event._source.alert?.type || "N/A"}</TableCell>
                          </TableRow>
                          <TableRow className="hover:bg-gray-50/50">
                            <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">告警分类</TableCell>
                            <TableCell className="px-4 py-2 text-gray-900">{event._source.alert?.category || "N/A"}</TableCell>
                          </TableRow>
                          <TableRow className="hover:bg-gray-50/50">
                            <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">置信度</TableCell>
                            <TableCell className="px-4 py-2">
                              <Badge variant="secondary" className="text-xs">
                                {event._source.alert?.confidence ? `${(event._source.alert.confidence * 100).toFixed(1)}%` : "N/A"}
                              </Badge>
                            </TableCell>
                          </TableRow>

                          {/* 规则信息分组 */}
                          <TableRow className="bg-blue-50/50">
                            <TableCell colSpan={2} className="px-4 py-2 font-semibold text-blue-800 text-xs">
                              <div className="flex items-center gap-2">
                                <Shield className="h-3 w-3" />
                                规则信息
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
                              <TableCell className="px-4 py-2 w-32 text-gray-600 font-medium">规则描述</TableCell>
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
  );
}
