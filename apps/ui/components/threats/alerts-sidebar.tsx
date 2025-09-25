"use client";

import { useState, useEffect } from "react";
import { Search, AlertTriangle, Clock, Filter, CheckCircle } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface Alert {
  id: string;
  severity: "critical" | "high" | "medium" | "low";
  title: string;
  description: string;
  timestamp: string;
  host: string;
  technique?: string;
  riskScore: number;
}

interface AlertsSidebarProps {
  onAlertSelect: (alert: Alert) => void;
  selectedAlertId?: string;
}

// 模拟告警数据
const mockAlerts: Alert[] = [
  {
    id: "alert-001",
    severity: "critical",
    title: "进程注入检测",
    description: "检测到可疑的进程注入行为，可能是恶意软件尝试规避检测",
    timestamp: "2025-09-25T20:30:00Z",
    host: "web-server-01",
    technique: "T1055.012",
    riskScore: 95
  },
  {
    id: "alert-002",
    severity: "high",
    title: "异常网络连接",
    description: "发现与已知恶意IP的异常网络连接",
    timestamp: "2025-09-25T20:25:00Z",
    host: "db-server-02",
    technique: "T1071.001",
    riskScore: 85
  },
  {
    id: "alert-003",
    severity: "high",
    title: "凭据转储尝试",
    description: "检测到尝试从LSASS内存中转储凭据的行为",
    timestamp: "2025-09-25T20:20:00Z",
    host: "app-server-03",
    technique: "T1003.001",
    riskScore: 88
  },
  {
    id: "alert-004",
    severity: "medium",
    title: "可疑文件执行",
    description: "执行了来自临时目录的可疑文件",
    timestamp: "2025-09-25T20:15:00Z",
    host: "web-server-01",
    technique: "T1204.002",
    riskScore: 65
  },
  {
    id: "alert-005",
    severity: "critical",
    title: "横向移动检测",
    description: "检测到使用RDP进行的横向移动尝试",
    timestamp: "2025-09-25T20:10:00Z",
    host: "admin-workstation",
    technique: "T1021.001",
    riskScore: 92
  }
];

export function AlertsSidebar({ onAlertSelect, selectedAlertId }: AlertsSidebarProps) {
  const [alerts, setAlerts] = useState<Alert[]>(mockAlerts);
  const [filteredAlerts, setFilteredAlerts] = useState<Alert[]>(mockAlerts);
  const [searchQuery, setSearchQuery] = useState("");
  const [severityFilter, setSeverityFilter] = useState<string>("all");

  // 过滤告警
  useEffect(() => {
    let filtered = alerts;

    // 按严重程度过滤
    if (severityFilter !== "all") {
      filtered = filtered.filter(alert => alert.severity === severityFilter);
    }

    // 按搜索查询过滤
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(alert =>
        alert.title.toLowerCase().includes(query) ||
        alert.description.toLowerCase().includes(query) ||
        alert.host.toLowerCase().includes(query) ||
        (alert.technique && alert.technique.toLowerCase().includes(query))
      );
    }

    setFilteredAlerts(filtered);
  }, [alerts, searchQuery, severityFilter]);

  const getSeverityBadge = (severity: string) => {
    const variants = {
      critical: "destructive",
      high: "destructive", 
      medium: "default",
      low: "secondary",
    } as const;
    
    const labels = {
      critical: "严重",
      high: "高危",
      medium: "中等", 
      low: "低危",
    } as const;

    return (
      <Badge variant={variants[severity as keyof typeof variants] || "outline"} className="text-xs">
        {labels[severity as keyof typeof labels] || severity}
      </Badge>
    );
  };

  const formatTimestamp = (timestamp: string) => {
    return new Date(timestamp).toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  return (
    <div className="w-80 border-r border-border bg-background flex flex-col h-full">
      {/* 搜索和筛选区域 */}
      <div className="px-4 lg:px-6 py-4 border-b border-border">
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="搜索告警..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10 h-9 text-sm"
            />
          </div>
          <Select value={severityFilter} onValueChange={setSeverityFilter}>
            <SelectTrigger className="h-9 text-sm w-24">
              <Filter className="h-4 w-4" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="critical">严重</SelectItem>
              <SelectItem value="high">高危</SelectItem>
              <SelectItem value="medium">中等</SelectItem>
              <SelectItem value="low">低危</SelectItem>
            </SelectContent>
          </Select>
          <Badge variant="secondary" className="text-xs h-9 px-3 flex items-center">
            {filteredAlerts.length}
          </Badge>
        </div>
      </div>

      {/* 告警列表 */}
      <div className="flex-1 overflow-auto p-2">
        {filteredAlerts.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <AlertTriangle className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">未找到匹配的告警</p>
          </div>
        ) : (
          <div className="space-y-2">
            {filteredAlerts.map((alert) => (
              <Card
                key={alert.id}
                className={`cursor-pointer transition-all hover:shadow-sm ${
                  selectedAlertId === alert.id 
                    ? "ring-2 ring-primary bg-primary/5" 
                    : "hover:bg-muted/50"
                }`}
                onClick={() => onAlertSelect(alert)}
              >
                <CardContent className="p-2">
                  <div className="flex items-start justify-between mb-1">
                    <div className="flex-1 min-w-0">
                      <CardTitle className="text-xs font-medium truncate">
                        {alert.title}
                      </CardTitle>
                      <div className="flex items-center gap-1 mt-0.5">
                        {getSeverityBadge(alert.severity)}
                        <span className="text-xs text-muted-foreground">
                          {alert.riskScore}
                        </span>
                      </div>
                    </div>
                    {selectedAlertId === alert.id && (
                      <CheckCircle className="h-3 w-3 text-primary flex-shrink-0" />
                    )}
                  </div>
                  <div className="flex items-center justify-between text-xs text-muted-foreground mt-1">
                    <div className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      <span>{formatTimestamp(alert.timestamp)}</span>
                    </div>
                    <span className="font-mono text-xs">{alert.host}</span>
                  </div>
                  {alert.technique && (
                    <div className="mt-1">
                      <Badge variant="outline" className="text-xs h-4 px-1">
                        {alert.technique}
                      </Badge>
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {/* 底部操作 */}
      <div className="p-4 border-t border-border">
        <Button 
          variant="outline" 
          size="sm" 
          className="w-full"
          onClick={() => {
            // 刷新告警数据
            setAlerts([...mockAlerts]);
          }}
        >
          <AlertTriangle className="h-4 w-4 mr-2" />
          刷新告警
        </Button>
      </div>
    </div>
  );
}
