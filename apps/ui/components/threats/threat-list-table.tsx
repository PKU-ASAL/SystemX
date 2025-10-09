"use client";

import React, { useEffect, useState } from "react";
import { RefreshCw, AlertTriangle, GitBranch, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ThreatAPI } from "@/lib/threatApi";

interface ThreatListTableProps {
  onThreatSelect: (threatId: string) => void;
  className?: string;
}

interface ThreatInfo {
  id: string;
  label: string;
  status: 'active' | 'inactive' | 'loading';
  lastUpdated?: string;
}

export function ThreatListTable({ onThreatSelect, className }: ThreatListTableProps) {
  const [threatList, setThreatList] = useState<ThreatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // localStorage缓存配置
  const CACHE_KEY = 'threat-list-cache';
  const CACHE_DURATION = 5 * 60 * 1000; // 5分钟缓存有效期

  // 从localStorage获取缓存
  const getCachedData = (): ThreatInfo[] | null => {
    try {
      const cached = localStorage.getItem(CACHE_KEY);
      if (!cached) return null;
      
      const { data, timestamp } = JSON.parse(cached);
      const now = Date.now();
      
      // 检查缓存是否过期
      if (now - timestamp > CACHE_DURATION) {
        localStorage.removeItem(CACHE_KEY);
        return null;
      }
      
      return data;
    } catch (error) {
      console.error('读取缓存失败:', error);
      localStorage.removeItem(CACHE_KEY);
      return null;
    }
  };

  // 保存到localStorage - 智能合并而不是替换
  const setCachedData = (newData: ThreatInfo[]) => {
    try {
      // 获取现有缓存数据
      const existingCached = getCachedData();
      let mergedData = newData;
      
      if (existingCached && existingCached.length > 0) {
        // 合并新旧数据，去重
        const existingIds = new Set(existingCached.map(t => t.id));
        const newIds = new Set(newData.map(t => t.id));
        
        // 保留所有曾经发现过的威胁ID
        const allIds = new Set([...existingIds, ...newIds]);
        mergedData = Array.from(allIds).sort().map(id => ({
          id,
          label: `威胁 ${id.toUpperCase()}`,
          status: 'active' as const,
          lastUpdated: new Date().toISOString()
        }));
        
        console.log(`🔄 [CACHE] 合并威胁列表: 原有${existingIds.size}个 + 新发现${newIds.size}个 = 总计${allIds.size}个`);
      }
      
      const cacheData = {
        data: mergedData,
        timestamp: Date.now()
      };
      localStorage.setItem(CACHE_KEY, JSON.stringify(cacheData));
    } catch (error) {
      console.error('保存缓存失败:', error);
    }
  };

  // 清除缓存
  const clearCache = () => {
    localStorage.removeItem(CACHE_KEY);
  };

  // 加载威胁列表
  const loadThreatList = async (forceRefresh: boolean = false) => {
    try {
      // 如果不是强制刷新，先尝试从缓存获取
      if (!forceRefresh) {
        const cachedData = getCachedData();
        if (cachedData) {
          console.log('📦 使用缓存的威胁列表');
          setThreatList(cachedData);
          setLoading(false);
          return;
        }
      }
      
      setLoading(true);
      setError(null);
      
      const threatIds = await ThreatAPI.getThreatList();
      
      if (threatIds.length === 0) {
        setError("未找到可用的威胁数据");
        setThreatList([]);
        return;
      }
      
      const threats: ThreatInfo[] = threatIds.map(id => ({
        id,
        label: `威胁 ${id.toUpperCase()}`,
        status: 'active' as const,
        lastUpdated: new Date().toISOString()
      }));
      
      setThreatList(threats);
      // 保存到缓存
      setCachedData(threats);
    } catch (err) {
      console.error('加载威胁列表失败:', err);
      setError(err instanceof Error ? err.message : '加载失败');
      setThreatList([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadThreatList(false); // 初始加载不强制刷新
  }, []);

  // 刷新列表
  const handleRefresh = () => {
    clearCache(); // 清除缓存
    ThreatAPI.clearCache();
    loadThreatList(true); // 强制刷新
  };

  // 获取状态徽章
  const getStatusBadge = (status: ThreatInfo['status']) => {
    switch (status) {
      case 'active':
        return <Badge variant="default" className="bg-green-500">活跃</Badge>;
      case 'inactive':
        return <Badge variant="secondary">非活跃</Badge>;
      case 'loading':
        return <Badge variant="outline">加载中</Badge>;
      default:
        return <Badge variant="outline">未知</Badge>;
    }
  };

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <GitBranch className="h-5 w-5 text-primary" />
            <CardTitle>威胁事件列表</CardTitle>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={loading}
          >
            <RefreshCw className={`h-4 w-4 mr-1 ${loading ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        </div>
        {threatList.length > 0 && (
          <p className="text-sm text-muted-foreground mt-2">
            发现 {threatList.length} 个威胁事件，点击查看详细溯源图
          </p>
        )}
      </CardHeader>
      
      <CardContent className="p-0">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-center">
              <RefreshCw className="h-8 w-8 animate-spin mx-auto mb-2 text-blue-600" />
              <p className="text-sm text-gray-600">正在加载威胁列表...</p>
            </div>
          </div>
        ) : error ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-center text-red-600">
              <AlertTriangle className="h-8 w-8 mx-auto mb-2" />
              <p className="text-sm font-medium">加载失败</p>
              <p className="text-xs text-gray-500 mt-1">{error}</p>
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefresh}
                className="mt-4"
              >
                重试
              </Button>
            </div>
          </div>
        ) : threatList.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-center text-gray-500">
              <AlertTriangle className="h-8 w-8 mx-auto mb-2" />
              <p className="text-sm">暂无威胁数据</p>
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefresh}
                className="mt-4"
              >
                刷新
              </Button>
            </div>
          </div>
        ) : (
          <div className="border-t">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[150px]">威胁ID</TableHead>
                  <TableHead>威胁名称</TableHead>
                  <TableHead className="w-[120px]">状态</TableHead>
                  <TableHead className="w-[180px]">最后更新</TableHead>
                  <TableHead className="w-[100px] text-center">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {threatList.map((threat) => (
                  <TableRow
                    key={threat.id}
                    className="cursor-pointer hover:bg-muted/50 transition-colors"
                    onClick={() => onThreatSelect(threat.id)}
                  >
                    <TableCell className="font-mono font-medium">
                      {threat.id}
                    </TableCell>
                    <TableCell>{threat.label}</TableCell>
                    <TableCell>{getStatusBadge(threat.status)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {threat.lastUpdated ? new Date(threat.lastUpdated).toLocaleString('zh-CN') : '-'}
                    </TableCell>
                    <TableCell className="text-center">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation();
                          onThreatSelect(threat.id);
                        }}
                      >
                        查看
                        <ChevronRight className="h-4 w-4 ml-1" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}