"use client";

import React, { useState } from "react";
import { FileText, Download, RefreshCw, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ThreatAPI } from "@/lib/threatApi";

interface ThreatReportSectionProps {
  threatId: string;
  className?: string;
}

export function ThreatReportSection({ threatId, className }: ThreatReportSectionProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 生成PDF报告
  const handleGeneratePdf = async () => {
    try {
      setLoading(true);
      setError(null);

      console.log(`开始生成威胁报告 PDF: ${threatId}`);
      
      // 调用真实的PDF API
      const response = await ThreatAPI.getThreatReportPdf(threatId);
      
      // 将base64转换为Blob并下载
      const pdfBlob = new Blob([base64ToArrayBuffer(response.pdf_base64)], { 
        type: 'application/pdf' 
      });
      
      const url = URL.createObjectURL(pdfBlob);
      const a = document.createElement('a');
      a.href = url;
      a.download = response.filename || `threat-report-${threatId}.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      
      console.log(`✅ 威胁报告 ${threatId} 下载成功`);
      
    } catch (err) {
      console.error('生成PDF报告失败:', err);
      setError(err instanceof Error ? err.message : '生成失败');
    } finally {
      setLoading(false);
    }
  };

  // base64转ArrayBuffer (将来实现PDF功能时使用)
  const base64ToArrayBuffer = (base64: string): ArrayBuffer => {
    const binaryString = atob(base64);
    const bytes = new Uint8Array(binaryString.length);
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i);
    }
    return bytes.buffer;
  };

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText className="h-5 w-5 text-primary" />
            <CardTitle>威胁分析报告</CardTitle>
            <Badge variant="outline" className="text-xs">
              {threatId.toUpperCase()}
            </Badge>
          </div>
        </div>
      </CardHeader>
      
      <CardContent className="space-y-4">
        <div className="text-sm text-muted-foreground">
          <p>生成包含以下内容的详细威胁分析报告：</p>
          <ul className="list-disc list-inside mt-2 space-y-1">
            <li>攻击时间线详细分析</li>
            <li>威胁影响范围评估</li>
            <li>攻击技术与战术映射 (MITRE ATT&CK)</li>
            <li>防护建议和修复方案</li>
            <li>相关IOC指标汇总</li>
          </ul>
        </div>
        
        {error && (
          <div className="flex items-center gap-2 p-3 bg-red-50 border border-red-200 rounded-md">
            <AlertTriangle className="h-4 w-4 text-red-600" />
            <span className="text-sm text-red-600">{error}</span>
          </div>
        )}
        
        <div className="flex gap-2">
          <Button
            onClick={handleGeneratePdf}
            disabled={loading}
            className="flex items-center gap-2"
          >
            {loading ? (
              <RefreshCw className="h-4 w-4 animate-spin" />
            ) : (
              <Download className="h-4 w-4" />
            )}
            {loading ? '生成中...' : '生成PDF报告'}
          </Button>
          
          {/* 预留其他报告格式按钮 */}
          {/* <Button variant="outline" disabled>
            <FileText className="h-4 w-4 mr-2" />
            导出Word
          </Button> */}
        </div>
        
        <div className="text-xs text-muted-foreground border-t pt-3">
          <p>📝 报告将包含该威胁事件的完整分析数据和可视化图表</p>
          <p>🔒 所有敏感信息已按安全规范处理</p>
        </div>
      </CardContent>
    </Card>
  );
}