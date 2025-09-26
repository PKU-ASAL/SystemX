"use client";

import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import dynamic from 'next/dynamic';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { RefreshCw, Maximize2, Minimize2, RotateCcw } from "lucide-react";
import { AttackTimelineCytoscape } from '@/lib/AttackTimelineCytoscape';
import { ThreatAPI } from '@/lib/threatApi';
import { ThreatGraphData } from '@/types/threat';

interface AttackTimelineGraphProps {
  threatId?: string;
  className?: string;
}

function AttackTimelineGraphClient({ threatId: initialThreatId = 'th-001', className }: AttackTimelineGraphProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const timelineRef = useRef<AttackTimelineCytoscape | null>(null);
  const [loading, setLoading] = useState(false); // 改为false，让容器先渲染
  const [graphLoading, setGraphLoading] = useState(false); // 新增状态专门管理图表加载
  const [error, setError] = useState<string | null>(null);
  const [graphData, setGraphData] = useState<ThreatGraphData | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [selectedThreatId, setSelectedThreatId] = useState(initialThreatId);

  // 威胁ID选择器状态
  const [threatOptions, setThreatOptions] = useState<Array<{ value: string; label: string }>>([]);
  const [loadingOptions, setLoadingOptions] = useState(true);

  // 测试useEffect是否工作
  useEffect(() => {
    console.log('✅ [MOUNT-TEST] 组件已挂载，useEffect正常工作');
  }, []);

  // 加载威胁ID列表
  useEffect(() => {
    const loadThreatOptions = async () => {
      try {
        console.log('🎯 [OPTIONS] 开始加载威胁ID选项');
        setLoadingOptions(true);
        const threatIds = await ThreatAPI.getThreatList();
        console.log('🎯 [OPTIONS] 威胁ID列表获取成功:', threatIds);
        const options = threatIds.map(id => ({
          value: id,
          label: `威胁 ${id.toUpperCase()}`
        }));
        setThreatOptions(options);
        console.log('🎯 [OPTIONS] 威胁选项设置完成');
      } catch (error) {
        console.error('❌ [OPTIONS] 加载威胁ID列表失败:', error);
        // 使用默认选项
        setThreatOptions([{ value: initialThreatId, label: `威胁 ${initialThreatId.toUpperCase()}` }]);
      } finally {
        setLoadingOptions(false);
        console.log('🎯 [OPTIONS] 威胁选项加载完成，loadingOptions设为false');
      }
    };

    loadThreatOptions();
  }, [initialThreatId]);


  // 图表初始化useEffect
  useEffect(() => {
    console.log('🔥 [SIMPLE-EFFECT] useEffect 触发，准备初始化图表', {
      selectedThreatId,
      hasContainer: !!containerRef.current,
      loadingOptions
    });

    // 如果威胁选项还在加载，先等待
    if (loadingOptions) {
      console.log('⏳ [SIMPLE-EFFECT] 威胁选项还在加载，等待...');
      return;
    }

    // 简单延迟确保DOM已渲染
    const initTimeout = setTimeout(async () => {
      console.log('🚀 [SIMPLE-EFFECT] 开始初始化图表');
      
      if (!containerRef.current) {
        console.error('❌ [SIMPLE-EFFECT] 容器ref不存在');
        return;
      }

      try {
        setGraphLoading(true);
        setError(null);

        // 创建容器ID
        const containerId = `attack-timeline-${Math.random().toString(36).substr(2, 9)}`;
        containerRef.current.id = containerId;
        
        console.log('📡 [SIMPLE-EFFECT] 正在获取威胁数据:', selectedThreatId);
        
        // 获取威胁数据
        console.log('📡 [API-CALL] 准备调用威胁API，URL:', `http://110.40.136.112:1334/api/alert/threat-graph?threat_id=${selectedThreatId}`);
        
        let data;
        try {
          data = await ThreatAPI.getThreatGraphData(selectedThreatId);
          console.log('📊 [API-SUCCESS] 威胁数据获取成功，数据结构:', {
            hasNodes: !!data?.nodes,
            hasEdges: !!data?.edges,
            hasHopSequence: !!data?.hop_sequence,
            dataKeys: Object.keys(data || {})
          });
        } catch (apiError) {
          console.error('❌ [API-ERROR] 威胁数据获取失败:', apiError);
          throw apiError;
        }

        if (!data) {
          throw new Error('威胁数据为空');
        }
        
        setGraphData(data);

        // 初始化Cytoscape组件
        console.log('📊 [CYTOSCAPE] 开始初始化Cytoscape组件');
        timelineRef.current = new AttackTimelineCytoscape(containerId);
        timelineRef.current.loadData(data, selectedThreatId);
        timelineRef.current.render();

        console.log('✅ [SIMPLE-EFFECT] 图表初始化完成');
      } catch (err) {
        console.error('❌ [SIMPLE-EFFECT] 初始化失败:', err);
        setError(err instanceof Error ? err.message : '初始化失败');
      } finally {
        setGraphLoading(false);
      }
    }, 500); // 增加到500ms延迟

    // 清理函数
    return () => {
      clearTimeout(initTimeout);
      if (timelineRef.current) {
        try {
          timelineRef.current.destroy();
        } catch (error) {
          console.warn('⚠️ [CLEANUP] 组件清理时出现警告:', error);
        } finally {
          timelineRef.current = null;
        }
      }
    };
  }, [selectedThreatId, loadingOptions]);


  // 刷新数据
  const handleRefresh = async () => {
    if (!timelineRef.current) return;

    try {
      setGraphLoading(true);
      ThreatAPI.clearCache();
      const data = await ThreatAPI.getThreatGraphData(selectedThreatId);
      setGraphData(data);
      timelineRef.current.loadData(data, selectedThreatId);
      timelineRef.current.render();
    } catch (err) {
      setError(err instanceof Error ? err.message : '刷新失败');
    } finally {
      setGraphLoading(false);
    }
  };

  // 重置视图
  const handleResetView = () => {
    if (timelineRef.current) {
      timelineRef.current.resetView();
    }
  };

  // 适应视图
  const handleFitView = () => {
    if (timelineRef.current) {
      timelineRef.current.fit();
    }
  };

  // 全屏切换
  const toggleFullscreen = () => {
    setIsFullscreen(!isFullscreen);
  };

  // 威胁ID切换处理
  const handleThreatIdChange = (newThreatId: string) => {
    if (newThreatId !== selectedThreatId) {
      setSelectedThreatId(newThreatId);
    }
  };

  // 获取统计信息
  const getStats = () => {
    if (!graphData) return { nodes: 0, edges: 0, phases: 0 };
    
    console.log('📊 [STATS] 计算统计信息，graphData:', graphData);
    
    // 检查数据结构
    const nodes = graphData.nodes || graphData.hop_sequence || [];
    const edges = graphData.edges || graphData.metadata?.originalEdges || [];
    
    return {
      nodes: nodes.length,
      edges: edges.length,
      phases: nodes.filter((n: any) => n.node_abstract === "1").length
    };
  };

  const stats = getStats();

  console.log('🎯 [RENDER] AttackTimelineGraph组件正在渲染！', { selectedThreatId, graphLoading, error });
  
  return (
    <Card className={`${className} ${isFullscreen ? 'fixed inset-0 z-50 rounded-none' : ''}`}>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex-1">
            <div className="flex items-center gap-4">
              <CardTitle className="flex items-center gap-2">
                攻击时间线溯源图
                <Badge variant="outline" className="text-xs">
                  威胁ID: {selectedThreatId}
                </Badge>
              </CardTitle>
              
              {/* 威胁ID选择器 */}
              <div className="flex items-center gap-2">
                <span className="text-sm text-gray-600">选择威胁:</span>
                <Select
                  value={selectedThreatId}
                  onValueChange={handleThreatIdChange}
                  disabled={loadingOptions || graphLoading}
                >
                  <SelectTrigger className="w-32 h-8 text-xs">
                    <SelectValue placeholder={loadingOptions ? "加载中..." : "选择威胁"} />
                  </SelectTrigger>
                  <SelectContent>
                    {threatOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value} className="text-xs">
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            
            {graphData && (
              <div className="flex gap-2 mt-2">
                <Badge variant="secondary" className="text-xs">
                  节点: {stats.nodes}
                </Badge>
                <Badge variant="secondary" className="text-xs">
                  边: {stats.edges}
                </Badge>
                <Badge variant="secondary" className="text-xs">
                  阶段: {stats.phases}
                </Badge>
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleRefresh}
              disabled={graphLoading}
              className="text-xs"
            >
              <RefreshCw className={`h-3.5 w-3.5 mr-1 ${graphLoading ? 'animate-spin' : ''}`} />
              刷新
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleResetView}
              disabled={graphLoading || !timelineRef.current}
              className="text-xs"
            >
              <RotateCcw className="h-3.5 w-3.5 mr-1" />
              重置
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={toggleFullscreen}
              className="text-xs"
            >
              {isFullscreen ? (
                <Minimize2 className="h-3.5 w-3.5 mr-1" />
              ) : (
                <Maximize2 className="h-3.5 w-3.5 mr-1" />
              )}
              {isFullscreen ? '退出全屏' : '全屏'}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0">
{/* 始终显示容器，但在加载时显示加载状态 */}
        <div 
          ref={containerRef}
          className={`bg-white border-t ${isFullscreen ? 'h-full' : 'h-96'} relative`}
          style={{ 
            width: '100%',
            minHeight: isFullscreen ? '100vh' : '600px'
          }}
          onLoad={() => console.log('📊 [CONTAINER] 容器已加载')}
        >
          {/* 加载状态覆盖层 */}
          {(graphLoading || loadingOptions) && (
            <div className="absolute inset-0 flex items-center justify-center bg-gray-50 z-10">
              <div className="text-center">
                <RefreshCw className="h-8 w-8 animate-spin mx-auto mb-2 text-blue-600" />
                <p className="text-sm text-gray-600">
                  {loadingOptions ? '正在加载威胁选项...' : '正在加载攻击时间线...'}
                </p>
              </div>
            </div>
          )}
          
          {/* 错误状态覆盖层 */}
          {error && (
            <div className="absolute inset-0 flex items-center justify-center bg-gray-50 z-10">
              <div className="text-center text-red-600">
                <p className="text-sm font-medium">加载失败</p>
                <p className="text-xs text-gray-500 mt-1">{error}</p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleRefresh}
                  className="mt-2"
                >
                  重试
                </Button>
              </div>
            </div>
          )}
          
          {/* 图表容器 */}
          <div style={{ width: '100%', height: '100%', position: 'relative' }}>
            {/* 这是Cytoscape图表的实际容器 */}
          </div>
        </div>
      </CardContent>
      
      {/* 操作提示 */}
      {!graphLoading && !error && (
        <div className="px-4 py-2 bg-gray-50 border-t text-xs text-gray-600">
          <div className="flex flex-wrap gap-4">
            <span>• 点击抽象节点展开/收缩阶段</span>
            <span>• 点击具体节点展开子节点</span>
            <span>• 双击节点查看详情</span>
            <span>• 鼠标悬停边查看技术信息</span>
            <span>• 双击边查看详细信息</span>
          </div>
        </div>
      )}
    </Card>
  );
}

// 使用动态导入防止SSR hydration问题
export const AttackTimelineGraph = dynamic(() => Promise.resolve(AttackTimelineGraphClient), {
  ssr: false,
  loading: () => (
    <div className="flex items-center justify-center h-96 bg-gray-50 border rounded-lg">
      <div className="text-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-2"></div>
        <p className="text-sm text-gray-600">正在加载攻击时间线图表...</p>
      </div>
    </div>
  )
});
