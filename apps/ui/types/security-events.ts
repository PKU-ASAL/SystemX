export interface SecurityEvent {
    _id: string;
    _source: {
        "@timestamp": string;
        alert: {
            id: string;
            type: string;
            category: string;
            severity: string;
            risk_score: number;
            confidence: number;
            rule: {
                id: string;
                name: string;
                description: string;
                title: string;
                mitigation: string;
                references: string[];
            };
            evidence: {
                event_type: string;
                process_name: string;
                process_cmdline: string;
                file_path: string;
                network_info: any;
            };
        };
        event: {
            raw: {
                event_id: string;
                timestamp: string;
                source: string;
                message: {
                    "evt.num": number;
                    "evt.time": number;
                    "evt.type": string;
                    "evt.category": string;
                    "evt.dir": string;
                    "evt.args": string;
                    "proc.name": string;
                    "proc.exe": string;
                    "proc.cmdline": string;
                    "proc.pid": number;
                    "proc.ppid": number;
                    "proc.uid": number;
                    "proc.gid": number;
                    "fd.name": string;
                    "net.sockaddr": any;
                    host: string;
                    is_warn: boolean;
                };
            };
        };
        timing: {
            created_at: string;
            processed_at: string;
        };
        metadata: {
            collector_id: string;
            host: string;
            source: string;
            processor: string;
        };
    };
}

export interface SearchState {
    query: any; // EUI Query object
    timeRange: {
        from: Date;
        to: Date;
        label: string;
    };
    pagination: {
        current: number;
        size: number;
        total: number;
    };
    sortField: string;
    sortDirection: "asc" | "desc";
    selectedEvents: string[];
}

export interface TimelineData {
    time: string;
    count: number;
    critical: number;
    high: number;
    medium: number;
    low: number;
}
