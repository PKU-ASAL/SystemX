const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || '/api/v1';

// 自定义 API 错误类
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public endpoint: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface Collector {
  collector_id: string;
  hostname: string;
  ip_address: string;
  os_type?: string;
  os_version?: string;
  deployment_type?: string;
  status: string;
  last_seen?: string;
  worker_address?: string;
  kafka_topic?: string;
  created_at?: string;
  updated_at?: string;
  metadata?: {
    group?: string;
    environment?: string;
    owner?: string;
    tags?: string[];
    description?: string;
    purpose?: string;
    region?: string;
    datacenter?: string;
  };
}

export interface Event {
  id: string;
  collector_id: string;
  event_type: string;
  timestamp: string;
  data: any;
}

export interface HealthStatus {
  status: string;
  workers: WorkerStatus[];
  healthy_workers: number;
  total_workers: number;
}

export interface WorkerStatus {
  id: string;
  url: string;
  status: string;
  last_check: string;
  response_time: number;
}

// Kafka interfaces - 基于实际 API 返回结构
export interface KafkaBroker {
  id: number;
  host: string;
  port: number;
  rack?: string | null;
  controller: boolean;
  version: string;
  jmx_port?: number;
  timestamp: string;
  disk_usage: {
    total_bytes: number;
    used_bytes: number;
    free_bytes: number;
    usage_percentage: number;
  };
  segment_count: number;
  segment_size: string;
  partitions_leader: number;
  partitions_skew: number;
  leaders_skew: number;
  in_sync_partitions: number;
  out_of_sync_partitions: number;
  network_stats: {
    bytes_in_per_sec: number;
    bytes_out_per_sec: number;
    messages_in_per_sec: number;
  };
  jvm_stats: {
    heap_used: string;
    heap_max: string;
    gc_count: number;
    uptime: string;
  };
}

// Topics Overview API 返回的 Topic 项
export interface TopicOverviewItem {
  name: string;
  partition_count: number;
  replication_factor: number;
  out_of_sync_replicas: number;
  messages_total: number;
  size_bytes: number;
  size_formatted: string;
  messages_per_sec: number;
  bytes_per_sec: number;
  is_internal: boolean;
}

// Topics Overview API 返回的汇总信息
export interface TopicsSummary {
  total_topics: number;
  internal_topics: number;
  total_size_bytes: number;
}

// Topics Overview API 完整响应
export interface TopicsOverviewResponse {
  topics: TopicOverviewItem[];
  summary: TopicsSummary;
}

// Topic Detail API 返回的概览信息
export interface TopicOverviewInfo {
  name: string;
  partition_count: number;
  replication_factor: number;
  config: Record<string, string>;
}

// Topic Detail API 返回的指标信息
export interface TopicMetricsInfo {
  messages_total: number;
  messages_per_sec: number;
  bytes_per_sec: number;
}

// Topic Detail API 返回的分区详细信息
export interface PartitionDetailInfo {
  partition_id: number;
  leader: number;
  replicas: number[];
  in_sync_replicas: number[];
  size_bytes: number;
  size_formatted: string;
  offset_earliest: number;
  offset_latest: number;
  offset_lag: number;
}

// Topic Detail API 完整响应
export interface TopicDetailsResponse {
  overview: TopicOverviewInfo;
  partitions: PartitionDetailInfo[];
  metrics: TopicMetricsInfo;
}

// 兼容性接口 - 用于组件中的统一处理
export interface KafkaTopic {
  name: string;
  partition_count?: number;
  replication_factor?: number;
  messages_total?: number;
  size_bytes?: number;
  size_formatted?: string;
  messages_per_sec?: number;
  bytes_per_sec?: number;
  is_internal?: boolean;
  out_of_sync_replicas?: number;

  // 详情页面特有字段
  overview?: TopicOverviewInfo;
  partitions?: PartitionDetailInfo[];
  metrics?: TopicMetricsInfo;

  // 兼容旧字段
  internal?: boolean;
  messageCount?: number;
  size?: string;
}

export interface KafkaPartition {
  partition: number;
  leader: number;
  replicas: number[];
  isr: number[];
  size: number;
  offset_lag: number;
}

export interface KafkaPartitionDetail {
  partition: number;
  leader: number;
  replicas: KafkaReplica[];
  offset_max: number;
  offset_min: number;
}

export interface KafkaReplica {
  broker: number;
  leader: boolean;
  in_sync: boolean;
}

export interface KafkaConsumerGroup {
  group_id: string;
  state: string;
  protocol_type: string;
  protocol: string;
  members: KafkaConsumerMember[];
  coordinator: {
    id: number;
    host: string;
    port: number;
  };
  lag: number;
  topics: string[];
}

export interface KafkaConsumerMember {
  member_id: string;
  client_id: string;
  client_host: string;
  assignments: KafkaTopicPartition[];
}

export interface KafkaTopicPartition {
  topic: string;
  partition: number;
  offset: number;
  lag: number;
}

class ApiClient {
  private baseUrl: string;

  constructor() {
    this.baseUrl = API_BASE_URL;
  }

  private async request<T>(endpoint: string, options?: RequestInit): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;

    // 创建 AbortController 用于超时控制
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 10000); // 10秒超时

    try {
      const response = await fetch(url, {
        headers: {
          'Content-Type': 'application/json',
          ...options?.headers,
        },
        signal: controller.signal,
        ...options,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        let errorMessage = `HTTP error! status: ${response.status}`;
        try {
          const errorData = await response.json();
          if (errorData.message) {
            errorMessage = errorData.message;
          } else if (errorData.error) {
            errorMessage = errorData.error;
          }
        } catch {
          // 如果无法解析错误响应，使用默认错误消息
        }
        throw new ApiError(errorMessage, response.status, endpoint);
      }

      return await response.json();
    } catch (error) {
      clearTimeout(timeoutId);
      console.error(`API request failed: ${url}`, error);

      // 网络错误或连接问题
      if (error instanceof TypeError && error.message.includes('fetch')) {
        throw new ApiError('网络连接失败，请检查后端服务是否正常运行', 0, endpoint);
      }

      // 超时错误
      if (error instanceof Error && error.name === 'AbortError') {
        throw new ApiError('请求超时，请稍后重试', 408, endpoint);
      }

      // 如果已经是 ApiError，直接抛出
      if (error instanceof ApiError) {
        throw error;
      }

      // 其他未知错误
      throw new ApiError(error instanceof Error ? error.message : '未知错误', 500, endpoint);
    }
  }

  // Collector APIs
  async getCollectors(params?: {
    page?: number;
    limit?: number;
    status?: string;
    group?: string;
    environment?: string;
    owner?: string;
    tags?: string;
    sort?: string;
    order?: string;
  }): Promise<{ data: Collector[]; total: number; page: number; limit: number }> {
    const searchParams = new URLSearchParams();
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined) {
          searchParams.append(key, value.toString());
        }
      });
    }

    const endpoint = `/collectors${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    const response = await this.request<{ data: { collectors: Collector[]; total: number }; success: boolean }>(endpoint);

    return {
      data: response.data.collectors || [],
      total: response.data.total || 0,
      page: params?.page || 1,
      limit: params?.limit || 20
    };
  }

  async getCollector(id: string): Promise<Collector> {
    return this.request<Collector>(`/collectors/${id}`);
  }

  async registerCollector(data: {
    hostname: string;
    ip_address: string;
    os_type: string;
    os_version: string;
    deployment_type: string;
    metadata?: any;
  }): Promise<{ success: boolean; data: { collector_id: string; worker_url: string; script_download_url: string } }> {
    return this.request(`/collectors/register`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteCollector(id: string, options?: { force?: boolean }): Promise<{ success: boolean; message?: string }> {
    const searchParams = new URLSearchParams();
    if (options?.force) {
      searchParams.append('force', 'true');
    }

    const endpoint = `/collectors/${id}${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    return this.request(endpoint, {
      method: 'DELETE',
    });
  }

  // Event APIs
  async getEvents(params: {
    topic: string;
    collector_id?: string;
    event_type?: string;
    limit?: number;
    latest?: boolean;
    from_time?: string;
    to_time?: string;
  }): Promise<{ data: Event[]; total: number }> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        searchParams.append(key, value.toString());
      }
    });

    const response = await this.request<{ data: { events: Event[] | null; total: number }; success: boolean }>(`/events/query?${searchParams.toString()}`);
    return {
      data: response.data.events || [],
      total: response.data.total || 0
    };
  }

  async getCollectorEvents(collectorId: string, params?: {
    event_type?: string;
    limit?: number;
    latest?: boolean;
    from_time?: string;
    to_time?: string;
  }): Promise<{ data: Event[]; total: number }> {
    const searchParams = new URLSearchParams();
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined) {
          searchParams.append(key, value.toString());
        }
      });
    }

    const endpoint = `/events/collectors/${collectorId}${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    const response = await this.request<{ data: { events: Event[] | null; total: number }; success: boolean }>(endpoint);
    return {
      data: response.data.events || [],
      total: response.data.total || 0
    };
  }

  async getTopics(): Promise<{ data: string[] }> {
    const response = await this.request<{ data: { collector_topics: string[] | null; other_topics: string[]; total_topics: number }; success: boolean }>('/events/topics');
    const topics = [
      ...(response.data.collector_topics || []),
      ...(response.data.other_topics || [])
    ];
    return { data: topics };
  }

  // Health APIs
  async getHealth(): Promise<HealthStatus> {
    const response = await this.request<{ data: any; success: boolean }>('/health');
    const healthData = response.data;

    return {
      status: healthData.healthy ? 'healthy' : 'unhealthy',
      workers: healthData.workers?.map((worker: any) => ({
        id: worker.name,
        url: worker.url,
        status: worker.healthy ? 'healthy' : 'unhealthy',
        last_check: worker.checked_at,
        response_time: worker.response_time_ms || 0
      })) || [],
      healthy_workers: healthData.healthy_workers || 0,
      total_workers: healthData.total_workers || 0
    };
  }

  async getWorkerStatus(): Promise<{ data: WorkerStatus[] }> {
    const healthResponse = await this.getHealth();
    return {
      data: healthResponse.workers
    };
  }

  // Kafka APIs
  async getKafkaClusterInfo(): Promise<{ data: any }> {
    const response = await this.request<any>('/services/kafka/clusters');
    return response;
  }

  async getKafkaBrokers(): Promise<{ data: KafkaBroker[] }> {
    const response = await this.request<any>('/services/kafka/brokers');
    return {
      data: response.data || []
    };
  }

  // 使用新的 Topics Overview API
  async getKafkaTopics(params?: { page?: number; limit?: number; search?: string }): Promise<{ data: KafkaTopic[]; total: number }> {
    const response = await this.request<{ data: TopicsOverviewResponse; success: boolean }>('/services/kafka/topics');

    let topics = response.data.topics || [];

    // 应用搜索过滤
    if (params?.search) {
      const searchTerm = params.search.toLowerCase();
      topics = topics.filter(topic =>
        topic.name.toLowerCase().includes(searchTerm)
      );
    }

    // 转换为兼容格式
    const convertedTopics: KafkaTopic[] = topics.map(topic => ({
      name: topic.name,
      partition_count: topic.partition_count,
      replication_factor: topic.replication_factor,
      messages_total: topic.messages_total,
      size_bytes: topic.size_bytes,
      size_formatted: topic.size_formatted,
      messages_per_sec: topic.messages_per_sec,
      bytes_per_sec: topic.bytes_per_sec,
      is_internal: topic.is_internal,
      out_of_sync_replicas: topic.out_of_sync_replicas,
      // 兼容字段
      internal: topic.is_internal,
      messageCount: topic.messages_total,
      size: topic.size_formatted
    }));

    // 应用分页
    const total = convertedTopics.length;
    const page = params?.page || 1;
    const limit = params?.limit || 20;
    const startIndex = (page - 1) * limit;
    const endIndex = startIndex + limit;
    const paginatedTopics = convertedTopics.slice(startIndex, endIndex);

    return {
      data: paginatedTopics,
      total: total
    };
  }

  async createKafkaTopic(data: {
    name: string;
    partitions?: number;
    replication_factor?: number;
    config?: Record<string, string>;
  }): Promise<any> {
    return this.request('/kafka/topics', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteKafkaTopic(topicName: string): Promise<any> {
    return this.request(`/kafka/topics/${topicName}`, {
      method: 'DELETE',
    });
  }

  async getKafkaTopicDetail(topicName: string): Promise<{ data: KafkaTopic }> {
    const response = await this.request<{ data: TopicDetailsResponse; success: boolean }>(`/kafka/topics/${topicName}`);

    // 转换为兼容格式
    const topicData: KafkaTopic = {
      name: response.data.overview.name,
      partition_count: response.data.overview.partition_count,
      replication_factor: response.data.overview.replication_factor,
      messages_total: response.data.metrics.messages_total,
      messages_per_sec: response.data.metrics.messages_per_sec,
      bytes_per_sec: response.data.metrics.bytes_per_sec,

      // 详情页面特有字段
      overview: response.data.overview,
      partitions: response.data.partitions,
      metrics: response.data.metrics,

      // 兼容字段
      messageCount: response.data.metrics.messages_total,
    };

    return {
      data: topicData
    };
  }

  async getKafkaTopicMessages(topicName: string, params?: {
    limit?: number;
    partition?: number;
    offset?: string;
  }): Promise<{ data: any[] }> {
    const searchParams = new URLSearchParams();
    if (params?.limit) searchParams.append('limit', params.limit.toString());
    if (params?.partition !== undefined) searchParams.append('partition', params.partition.toString());
    if (params?.offset) searchParams.append('offset', params.offset);

    const endpoint = `/kafka/topics/${topicName}/messages${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    const response = await this.request<any>(endpoint);

    return {
      data: response.data?.messages || []
    };
  }

  async getKafkaTopicConfig(topicName: string): Promise<{ data: Record<string, any> }> {
    const response = await this.request<any>(`/kafka/topics/${topicName}/config`);
    return {
      data: response.data?.configs || {}
    };
  }

  async updateKafkaTopicConfig(topicName: string, config: Record<string, string>): Promise<any> {
    return this.request(`/kafka/topics/${topicName}/config`, {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  }


  // OpenSearch APIs
  async getOpenSearchClusterHealth(): Promise<any> {
    return this.request('/opensearch/cluster/health');
  }

  async getOpenSearchClusterStats(): Promise<any> {
    return this.request('/opensearch/cluster/stats');
  }

  async getOpenSearchIndices(): Promise<any> {
    return this.request('/opensearch/indices');
  }

  async searchSecurityEvents(params?: {
    index?: string;
    q?: string;
    size?: number;
    from?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    if (params?.index) searchParams.append('index', params.index);
    if (params?.q) searchParams.append('q', params.q);
    if (params?.size) searchParams.append('size', params.size.toString());
    if (params?.from) searchParams.append('from', params.from.toString());

    const endpoint = `/opensearch/events/search${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    return this.request(endpoint);
  }

  async getRecentSecurityEvents(params?: {
    index?: string;
    hours?: number;
    size?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    if (params?.index) searchParams.append('index', params.index);
    if (params?.hours) searchParams.append('hours', params.hours.toString());
    if (params?.size) searchParams.append('size', params.size.toString());

    const endpoint = `/opensearch/events/recent${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    return this.request(endpoint);
  }

  async getHighRiskEvents(params: {
    min_score: number;
    index?: string;
    size?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    searchParams.append('min_score', params.min_score.toString());
    if (params.index) searchParams.append('index', params.index);
    if (params.size) searchParams.append('size', params.size.toString());

    return this.request(`/opensearch/events/high-risk?${searchParams.toString()}`);
  }

  async getThreatEvents(params?: {
    index?: string;
    size?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    if (params?.index) searchParams.append('index', params.index);
    if (params?.size) searchParams.append('size', params.size.toString());

    const endpoint = `/opensearch/events/threats${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    return this.request(endpoint);
  }

  async getEventsBySource(params: {
    source: string;
    index?: string;
    size?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    searchParams.append('source', params.source);
    if (params.index) searchParams.append('index', params.index);
    if (params.size) searchParams.append('size', params.size.toString());

    return this.request(`/opensearch/events/by-source?${searchParams.toString()}`);
  }

  async getEventsByTimeRange(params: {
    from: string;
    to: string;
    index?: string;
    size?: number;
    page?: number;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    searchParams.append('from', params.from);
    searchParams.append('to', params.to);
    if (params.index) searchParams.append('index', params.index);
    if (params.size) searchParams.append('size', params.size.toString());
    if (params.page) searchParams.append('page', params.page.toString());

    return this.request(`/opensearch/events/time-range?${searchParams.toString()}`);
  }

  async getEventAggregations(params?: {
    index?: string;
  }): Promise<any> {
    const searchParams = new URLSearchParams();
    if (params?.index) searchParams.append('index', params.index);

    const endpoint = `/opensearch/events/aggregations${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    return this.request(endpoint);
  }
}

export const apiClient = new ApiClient();
