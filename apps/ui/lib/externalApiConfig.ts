// 外部API配置管理
export interface ThreatApiConfig {
    baseUrl: string;
    timeout: number;
    headers: Record<string, string>;
}

/**
 * 获取威胁API配置
 */
export function getThreatApiConfig(): ThreatApiConfig {
    // Next.js 会自动将 NEXT_PUBLIC_ 开头的环境变量注入到客户端
    const baseUrl = process.env.NEXT_PUBLIC_THREAT_API_BASE_URL || '';

    return {
        baseUrl,
        timeout: 10000, // 10秒超时
        headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        }
    };
}

/**
 * 检查威胁API是否启用
 */
export function isThreatApiEnabled(): boolean {
    // Next.js 会自动将 NEXT_PUBLIC_ 开头的环境变量注入到客户端
    const enabled = process.env.NEXT_PUBLIC_THREAT_API_ENABLED;

    // 默认禁用，除非明确设置为 true
    if (enabled === undefined || enabled === null) {
        return false;
    }

    return String(enabled).toLowerCase() === 'true';
}
