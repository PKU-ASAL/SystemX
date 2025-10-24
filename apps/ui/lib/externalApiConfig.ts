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
    // 在客户端使用环境变量，需要通过 typeof window 检查
    const baseUrl = typeof window !== 'undefined'
        ? (window as any).ENV?.NEXT_PUBLIC_THREAT_API_BASE_URL || ''
        : process.env.NEXT_PUBLIC_THREAT_API_BASE_URL || '';

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
    // 在客户端使用环境变量，需要通过 typeof window 检查
    const enabled = typeof window !== 'undefined'
        ? (window as any).ENV?.NEXT_PUBLIC_THREAT_API_ENABLED
        : process.env.NEXT_PUBLIC_THREAT_API_ENABLED;

    // 默认禁用，除非明确设置为 true
    if (enabled === undefined || enabled === null) {
        return false;
    }

    return String(enabled).toLowerCase() === 'true';
}
