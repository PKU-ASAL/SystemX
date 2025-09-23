"use client";

import { usePathname } from 'next/navigation';
import { useMemo } from 'react';

export interface BreadcrumbItem {
    label: string;
    href?: string;
    isCurrentPage?: boolean;
}

// 路径映射配置
const pathMappings: Record<string, string> = {
    '/': '仪表板',
    '/health': '系统健康',
    '/kafka': 'Kafka管理',
    '/kafka/brokers': 'Broker管理',
    '/kafka/topics': 'Topic管理',
    '/kafka/consumers': 'Consumer管理',
    '/logs': '日志管理',
    '/logs/search': '日志搜索',
    '/logs/analysis': '日志分析',
    '/opensearch': 'OpenSearch',
    '/opensearch/alerts': '告警管理',
    '/terminal-create': '创建终端',
    '/terminal-list': '终端列表',
    '/workflows': '工作流管理',
    '/workflows/create': '创建工作流',
    '/workflows/templates': '工作流模板',
    '/settings': '系统设置',
    '/help': '帮助文档',
};

export function useBreadcrumb(): BreadcrumbItem[] {
    const pathname = usePathname();

    return useMemo(() => {
        // 移除开头的斜杠并分割路径
        const pathSegments = pathname.split('/').filter(Boolean);

        // 如果是根路径，直接返回仪表板
        if (pathSegments.length === 0) {
            return [{ label: '仪表板', href: '/', isCurrentPage: true }];
        }

        const breadcrumbs: BreadcrumbItem[] = [
            { label: '仪表板', href: '/' }
        ];

        // 构建面包屑路径
        let currentPath = '';
        pathSegments.forEach((segment, index) => {
            currentPath += `/${segment}`;
            const isLast = index === pathSegments.length - 1;

            // 获取显示名称
            const label = pathMappings[currentPath] ||
                segment.charAt(0).toUpperCase() + segment.slice(1);

            breadcrumbs.push({
                label,
                href: isLast ? undefined : currentPath,
                isCurrentPage: isLast
            });
        });

        return breadcrumbs;
    }, [pathname]);
}
