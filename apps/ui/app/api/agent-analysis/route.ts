import { NextRequest, NextResponse } from 'next/server';

// 模拟的告警数据分析结果
const mockAnalysisResults = [
    {
        step: "数据收集",
        description: "从 OpenSearch 检索相关告警数据",
        status: "complete",
        details: {
            query: "alert.severity:(critical OR high)",
            timeRange: "过去24小时",
            totalAlerts: 23,
            criticalAlerts: 8,
            highAlerts: 15
        }
    },
    {
        step: "威胁识别",
        description: "基于 MITRE ATT&CK 框架识别攻击技术",
        status: "complete",
        details: {
            techniques: ["T1055.012", "T1003.001", "T1021.001"],
            tactics: ["Defense Evasion", "Credential Access", "Lateral Movement"],
            confidence: 0.87
        }
    },
    {
        step: "行为分析",
        description: "分析攻击者的行为模式和攻击链",
        status: "complete",
        details: {
            attackChain: [
                "初始访问 → 权限提升 → 凭据转储 → 横向移动",
                "检测到进程注入行为",
                "发现异常网络连接"
            ],
            riskScore: 85,
            impactAssessment: "高风险 - 可能导致数据泄露"
        }
    },
    {
        step: "建议措施",
        description: "生成针对性的安全建议和响应措施",
        status: "complete",
        details: {
            immediateActions: [
                "立即隔离受影响主机",
                "重置相关用户凭据",
                "检查横向移动路径"
            ],
            preventiveMeasures: [
                "加强端点检测规则",
                "实施零信任网络架构",
                "定期进行渗透测试"
            ]
        }
    }
];

export async function POST(request: NextRequest) {
    try {
        const body = await request.json();
        const { query, alertData } = body;

        // 模拟处理延迟
        await new Promise(resolve => setTimeout(resolve, 1000));

        // 基于查询内容生成不同的分析结果
        let analysisResult = {
            query: query,
            timestamp: new Date().toISOString(),
            steps: mockAnalysisResults,
            summary: {
                totalSteps: mockAnalysisResults.length,
                completedSteps: mockAnalysisResults.filter(s => s.status === 'complete').length,
                overallRisk: "高风险",
                confidence: 0.87,
                recommendation: "建议立即采取防护措施，重点关注进程注入和凭据访问行为"
            },
            artifacts: [
                {
                    type: "analysis-report",
                    title: "威胁分析报告",
                    content: `# 威胁分析报告

## 概述
基于提供的告警数据，检测到高风险攻击活动。

## 关键发现
- **攻击技术**: 进程注入 (T1055.012)
- **目标**: 凭据访问和横向移动
- **风险等级**: 高风险 (85/100)

## 建议措施
1. 立即隔离受影响主机
2. 重置相关用户凭据
3. 检查横向移动路径

## 详细分析
攻击者使用了进程注入技术来规避检测，随后尝试进行凭据转储...`,
                    createdAt: new Date().toISOString()
                }
            ]
        };

        // 根据查询内容调整结果
        if (query && query.toLowerCase().includes('网络')) {
            (analysisResult.steps[1].details as any).techniques.push("T1040");
            analysisResult.summary.recommendation = "重点关注网络流量异常和横向移动行为";
        }

        if (query && query.toLowerCase().includes('恶意软件')) {
            (analysisResult.steps[1].details as any).techniques.push("T1055.001");
            analysisResult.summary.recommendation = "建议加强恶意软件检测和进程监控";
        }

        return NextResponse.json({
            success: true,
            data: analysisResult
        });

    } catch (error) {
        console.error('Agent analysis API error:', error);
        return NextResponse.json(
            { success: false, error: 'Internal server error' },
            { status: 500 }
        );
    }
}
