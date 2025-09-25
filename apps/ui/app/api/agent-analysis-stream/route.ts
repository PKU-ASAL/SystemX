import { NextRequest, NextResponse } from 'next/server';

// 模拟 LLM 流式响应的完整分析过程
export async function POST(request: NextRequest) {
    const { query, alertData } = await request.json();

    // 创建流式响应
    const encoder = new TextEncoder();
    const stream = new ReadableStream({
        async start(controller) {
            try {
                // 1. Thinking 阶段
                await delay(500);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'thinking',
                    content: '我需要分析这个安全事件，首先理解用户的查询意图，然后制定分析计划...'
                })}\n\n`));

                await delay(800);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'thinking',
                    content: '基于提供的告警数据，我需要：\n1. 分析攻击向量\n2. 识别 MITRE ATT&CK 技术\n3. 评估风险等级\n4. 提供缓解建议'
                })}\n\n`));

                // 2. Task 阶段 - 数据收集
                await delay(600);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'task',
                    task: {
                        id: 'task-1',
                        title: '数据收集与预处理',
                        status: 'active',
                        items: [
                            { text: '从 OpenSearch 检索相关告警数据', status: 'complete' },
                            { text: '解析告警字段和元数据', status: 'active' }
                        ]
                    }
                })}\n\n`));

                // 3. Tool 调用 - 搜索告警
                await delay(700);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'tool',
                    tool: {
                        name: 'search_alerts',
                        state: 'input-available',
                        input: {
                            query: alertData ? `host:${alertData.host}` : 'severity:(critical OR high)',
                            timeRange: '过去24小时',
                            maxResults: 50
                        }
                    }
                })}\n\n`));

                await delay(1000);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'tool',
                    tool: {
                        name: 'search_alerts',
                        state: 'output-available',
                        output: {
                            totalAlerts: 23,
                            criticalAlerts: 8,
                            highAlerts: 15,
                            relevantTechniques: ['T1055', 'T1071', 'T1548']
                        }
                    }
                })}\n\n`));

                // 4. 更新 Task 状态
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'task',
                    task: {
                        id: 'task-1',
                        title: '数据收集与预处理',
                        status: 'complete',
                        items: [
                            { text: '从 OpenSearch 检索相关告警数据', status: 'complete' },
                            { text: '解析告警字段和元数据', status: 'complete' }
                        ]
                    }
                })}\n\n`));

                // 5. 新的 Task - 威胁识别
                await delay(500);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'task',
                    task: {
                        id: 'task-2',
                        title: '威胁识别与分类',
                        status: 'active',
                        items: [
                            { text: '分析攻击技术和战术', status: 'active' },
                            { text: '映射到 MITRE ATT&CK 框架', status: 'pending' }
                        ]
                    }
                })}\n\n`));

                // 6. Tool 调用 - MITRE 分析
                await delay(800);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'tool',
                    tool: {
                        name: 'mitre_analysis',
                        state: 'input-available',
                        input: {
                            techniques: ['T1055', 'T1071', 'T1548'],
                            alertData: alertData || { severity: 'high', host: 'unknown' }
                        }
                    }
                })}\n\n`));

                await delay(1200);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'tool',
                    tool: {
                        name: 'mitre_analysis',
                        state: 'output-available',
                        output: {
                            attackChain: ['初始访问', '权限提升', '凭据访问', '横向移动'],
                            confidence: 0.87,
                            riskLevel: '高风险'
                        }
                    }
                })}\n\n`));

                // 7. Chain of Thought 推理过程
                await delay(600);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'chain_of_thought',
                    step: {
                        id: 'cot-1',
                        label: '攻击向量分析',
                        description: '基于告警数据分析可能的攻击路径',
                        status: 'active',
                        details: {
                            findings: ['检测到进程注入行为', '异常网络连接模式', '权限提升尝试'],
                            confidence: 0.85
                        }
                    }
                })}\n\n`));

                await delay(700);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'chain_of_thought',
                    step: {
                        id: 'cot-2',
                        label: '风险评估',
                        description: '评估攻击的潜在影响和风险等级',
                        status: 'complete',
                        details: {
                            riskScore: 87,
                            impactLevel: '高',
                            urgency: '立即处理'
                        }
                    }
                })}\n\n`));

                // 8. 最终分析结果
                await delay(500);
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'analysis_result',
                    result: {
                        query: query,
                        timestamp: new Date().toISOString(),
                        steps: [
                            {
                                step: '数据收集',
                                description: '从 OpenSearch 检索相关告警数据',
                                status: 'complete',
                                details: {
                                    query: alertData ? `host:${alertData.host}` : 'severity:(critical OR high)',
                                    timeRange: '过去24小时',
                                    totalAlerts: 23,
                                    criticalAlerts: 8,
                                    highAlerts: 15
                                }
                            },
                            {
                                step: '威胁识别',
                                description: '识别攻击技术和映射到 MITRE ATT&CK 框架',
                                status: 'complete',
                                details: {
                                    techniques: ['T1055', 'T1071', 'T1548'],
                                    attackChain: ['初始访问', '权限提升', '凭据访问', '横向移动']
                                }
                            },
                            {
                                step: '行为分析',
                                description: '分析攻击行为模式和异常指标',
                                status: 'complete',
                                details: {
                                    behaviorPatterns: ['进程注入', '网络异常', '权限滥用'],
                                    anomalyScore: 0.92
                                }
                            },
                            {
                                step: '建议措施',
                                description: '基于分析结果提供安全建议',
                                status: 'complete',
                                details: {
                                    recommendations: ['隔离受影响主机', '更新安全策略', '加强监控']
                                }
                            }
                        ],
                        summary: {
                            totalSteps: 4,
                            completedSteps: 4,
                            overallRisk: '高风险',
                            confidence: 0.87,
                            recommendation: '建议立即采取防护措施，重点关注进程注入和凭据访问行为'
                        },
                        artifacts: []
                    }
                })}\n\n`));

                // 9. 完成信号
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'done'
                })}\n\n`));

                controller.close();
            } catch (error) {
                controller.enqueue(encoder.encode(`data: ${JSON.stringify({
                    type: 'error',
                    error: '分析过程中出现错误'
                })}\n\n`));
                controller.close();
            }
        }
    });

    return new Response(stream, {
        headers: {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
        },
    });
}

function delay(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms));
}
