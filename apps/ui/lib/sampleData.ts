/**
 * Dashboard 样例数据
 * 用于展示和演示用途，通过环境变量 NEXT_PUBLIC_LOAD_SAMPLE_DATA 控制
 */

// 告警严重程度分布
export const sampleAlertSeverity = {
    critical: 12,
    high: 28,
    medium: 45,
    low: 67,
    total: 152,
    timeRange: "24h",
    updatedAt: new Date().toISOString(),
};

// 告警趋势数据 - 过去7天
export const sampleAlertTrends = {
    timeline: [
        {
            timestamp: new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 2,
            high: 5,
            medium: 8,
            low: 12,
            total: 27,
        },
        {
            timestamp: new Date(Date.now() - 5 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 1,
            high: 4,
            medium: 6,
            low: 10,
            total: 21,
        },
        {
            timestamp: new Date(Date.now() - 4 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 3,
            high: 6,
            medium: 9,
            low: 15,
            total: 33,
        },
        {
            timestamp: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 2,
            high: 3,
            medium: 7,
            low: 11,
            total: 23,
        },
        {
            timestamp: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 1,
            high: 5,
            medium: 8,
            low: 13,
            total: 27,
        },
        {
            timestamp: new Date(Date.now() - 1 * 24 * 60 * 60 * 1000).toISOString(),
            critical: 2,
            high: 4,
            medium: 6,
            low: 9,
            total: 21,
        },
        {
            timestamp: new Date().toISOString(),
            critical: 1,
            high: 1,
            medium: 1,
            low: 7,
            total: 10,
        },
    ],
    statistics: {
        total_alerts: 152,
        avg_per_interval: 21.7,
        interval: "1d",
        time_range: "7d",
    },
};

// Collector 概览数据
export const sampleCollectorsOverview = {
    summary: {
        total: 156,
        active: 142,
        inactive: 8,
        offline: 4,
        error: 2,
    },
    performance: {
        healthyPercentage: 91.0,
        avgResponseTime: 45,
        recentlyActive: 138,
    },
};

// 系统性能数据
export const sampleSystemPerformance = {
    kafka: {
        cluster_status: "online",
        brokers: 3,
        topics: 12,
        messages_per_sec: 1247,
        disk_usage: 42,
    },
    opensearch: {
        cluster_status: "green",
        indices_count: 24,
        docs_count: 1245678,
        storage_size: "12.4 GB",
        query_performance: {
            avg_response_time: 23,
            queries_per_sec: 156,
            slow_queries: 2,
        },
    },
    flink: {
        cluster_status: "healthy",
        jobs_running: 4,
        memory_usage: 68,
        processing_rate: 2340,
    },
};

// 最近告警终端数据
export const sampleRecentAlertHosts = [
    {
        hostname: "web-server-prod-01",
        ip_address: "10.0.1.15",
        alert_count: 18,
        last_alert_time: new Date(Date.now() - 15 * 60 * 1000).toISOString(),
        severity: "high",
    },
    {
        hostname: "db-master-01",
        ip_address: "10.0.2.20",
        alert_count: 12,
        last_alert_time: new Date(Date.now() - 28 * 60 * 1000).toISOString(),
        severity: "high",
    },
    {
        hostname: "app-server-prod-03",
        ip_address: "10.0.1.33",
        alert_count: 9,
        last_alert_time: new Date(Date.now() - 42 * 60 * 1000).toISOString(),
        severity: "medium",
    },
    {
        hostname: "cache-redis-02",
        ip_address: "10.0.3.12",
        alert_count: 7,
        last_alert_time: new Date(Date.now() - 65 * 60 * 1000).toISOString(),
        severity: "medium",
    },
    {
        hostname: "mq-broker-01",
        ip_address: "10.0.4.25",
        alert_count: 5,
        last_alert_time: new Date(Date.now() - 88 * 60 * 1000).toISOString(),
        severity: "low",
    },
    {
        hostname: "web-server-prod-02",
        ip_address: "10.0.1.16",
        alert_count: 4,
        last_alert_time: new Date(Date.now() - 102 * 60 * 1000).toISOString(),
        severity: "low",
    },
    {
        hostname: "app-server-prod-05",
        ip_address: "10.0.1.35",
        alert_count: 3,
        last_alert_time: new Date(Date.now() - 125 * 60 * 1000).toISOString(),
        severity: "low",
    },
    {
        hostname: "log-collector-03",
        ip_address: "10.0.5.18",
        alert_count: 2,
        last_alert_time: new Date(Date.now() - 148 * 60 * 1000).toISOString(),
        severity: "low",
    },
];

// 生成不同时间范围的告警趋势数据
export const generateSampleTimelineData = (range: string) => {
    const now = new Date();
    const data = [];
    let days = 7;

    switch (range) {
        case "7d":
            days = 7;
            break;
        case "30d":
            days = 30;
            break;
        case "3m":
            days = 90;
            break;
    }

    for (let i = days - 1; i >= 0; i--) {
        const date = new Date(now);
        date.setDate(date.getDate() - i);

        // 生成有规律的模拟数据，模拟真实场景的告警分布
        // 使用一些随机性但保持合理的比例
        const dayOfWeek = date.getDay();
        const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;

        // 工作日告警较多，周末较少
        const baseMultiplier = isWeekend ? 0.6 : 1.0;

        const critical = Math.floor((Math.random() * 3 + 1) * baseMultiplier);
        const high = Math.floor((Math.random() * 6 + 2) * baseMultiplier);
        const medium = Math.floor((Math.random() * 10 + 4) * baseMultiplier);
        const low = Math.floor((Math.random() * 15 + 8) * baseMultiplier);

        data.push({
            time: date.toLocaleDateString('zh-CN', {
                month: '2-digit',
                day: '2-digit'
            }),
            count: critical + high + medium + low,
            critical,
            high,
            medium,
            low,
        });
    }

    return data;
};

// 检查是否应该使用样例数据
export const shouldLoadSampleData = (): boolean => {
    const envValue = process.env.NEXT_PUBLIC_LOAD_SAMPLE_DATA;
    return envValue === 'true' || envValue === '1';
};
