package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// PrometheusService Prometheus 客户端服务
type PrometheusService struct {
	baseURL string
	client  *http.Client
}

// NewPrometheusService 创建 Prometheus 服务
func NewPrometheusService() *PrometheusService {
	baseURL := os.Getenv("PROMETHEUS_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9090"
	}

	timeout := 30 * time.Second
	if timeoutStr := os.Getenv("PROMETHEUS_TIMEOUT"); timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsedTimeout
		}
	}

	return &PrometheusService{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// PrometheusResponse Prometheus 查询响应
type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string   `json:"resultType"`
		Result     []Result `json:"result"`
	} `json:"data"`
}

// Result Prometheus 查询结果
type Result struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

// KafkaTopicMetrics Kafka Topic 指标
type KafkaTopicMetrics struct {
	TopicName         string  `json:"topic_name"`
	MessagesTotal     int64   `json:"messages_total"`
	BytesTotal        int64   `json:"bytes_total"`
	MessagesPerSec    float64 `json:"messages_per_sec"`
	BytesPerSec       float64 `json:"bytes_per_sec"`
	PartitionCount    int     `json:"partition_count"`
	LogSizeBytes      int64   `json:"log_size_bytes"`
	Internal          bool    `json:"internal"`
	LastUpdated       time.Time `json:"last_updated"`
}

// KafkaBrokerMetrics Kafka Broker 指标
type KafkaBrokerMetrics struct {
	BrokerID        string    `json:"broker_id"`
	ID              string    `json:"id"`
	BytesInPerSec   float64   `json:"bytes_in_per_sec"`
	BytesOutPerSec  float64   `json:"bytes_out_per_sec"`
	MessagesInPerSec float64  `json:"messages_in_per_sec"`
	LastUpdated     time.Time `json:"last_updated"`
}

// KafkaClusterMetrics Kafka 集群指标
type KafkaClusterMetrics struct {
	BrokerCount         int       `json:"broker_count"`
	TopicCount          int       `json:"topic_count"`
	PartitionCount      int       `json:"partition_count"`
	TotalBytesInPerSec  float64   `json:"total_bytes_in_per_sec"`
	TotalBytesOutPerSec float64   `json:"total_bytes_out_per_sec"`
	LastUpdated         time.Time `json:"last_updated"`
}

// Query 执行 Prometheus 查询
func (p *PrometheusService) Query(ctx context.Context, query string) (*PrometheusResponse, error) {
	// 构建查询 URL
	queryURL := fmt.Sprintf("%s/api/v1/query", p.baseURL)
	
	// 添加查询参数
	params := url.Values{}
	params.Add("query", query)
	
	fullURL := fmt.Sprintf("%s?%s", queryURL, params.Encode())
	
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 执行请求
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus query failed with status %d", resp.StatusCode)
	}
	
	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	// 解析响应
	var promResp PrometheusResponse
	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed with status: %s", promResp.Status)
	}
	
	return &promResp, nil
}

// GetKafkaTopicMetrics 获取 Kafka Topic 指标
func (p *PrometheusService) GetKafkaTopicMetrics(ctx context.Context) (map[string]*KafkaTopicMetrics, error) {
	metrics := make(map[string]*KafkaTopicMetrics)
	
	// 1. 获取消息总数
	messagesResp, err := p.Query(ctx, "kafka_topic_messages_total")
	if err != nil {
		return nil, fmt.Errorf("failed to get messages total: %w", err)
	}
	
	for _, result := range messagesResp.Data.Result {
		topicName := result.Metric["topic"]
		if topicName == "" {
			continue
		}
		
		if _, exists := metrics[topicName]; !exists {
			metrics[topicName] = &KafkaTopicMetrics{
				TopicName:   topicName,
				Internal:    strings.HasPrefix(topicName, "__"),
				LastUpdated: time.Now(),
			}
		}
		
		if len(result.Value) >= 2 {
			if valueStr, ok := result.Value[1].(string); ok {
				if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
					metrics[topicName].MessagesTotal = value
				}
			}
		}
	}
	
	// 2. 获取字节总数
	bytesResp, err := p.Query(ctx, "kafka_topic_bytes_in_total")
	if err != nil {
		return nil, fmt.Errorf("failed to get bytes total: %w", err)
	}
	
	for _, result := range bytesResp.Data.Result {
		topicName := result.Metric["topic"]
		if topicName == "" {
			continue
		}
		
		if _, exists := metrics[topicName]; !exists {
			metrics[topicName] = &KafkaTopicMetrics{
				TopicName:   topicName,
				Internal:    strings.HasPrefix(topicName, "__"),
				LastUpdated: time.Now(),
			}
		}
		
		if len(result.Value) >= 2 {
			if valueStr, ok := result.Value[1].(string); ok {
				if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
					metrics[topicName].BytesTotal = value
				}
			}
		}
	}
	
	// 3. 获取消息速率
	messagesPerSecResp, err := p.Query(ctx, "kafka_topic_messages_per_sec")
	if err == nil {
		for _, result := range messagesPerSecResp.Data.Result {
			topicName := result.Metric["topic"]
			if topicName == "" {
				continue
			}
			
			if _, exists := metrics[topicName]; !exists {
				metrics[topicName] = &KafkaTopicMetrics{
					TopicName:   topicName,
					Internal:    strings.HasPrefix(topicName, "__"),
					LastUpdated: time.Now(),
				}
			}
			
			if len(result.Value) >= 2 {
				if valueStr, ok := result.Value[1].(string); ok {
					if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
						metrics[topicName].MessagesPerSec = value
					}
				}
			}
		}
	}
	
	// 4. 获取字节速率
	bytesPerSecResp, err := p.Query(ctx, "kafka_topic_bytes_in_per_sec")
	if err == nil {
		for _, result := range bytesPerSecResp.Data.Result {
			topicName := result.Metric["topic"]
			if topicName == "" {
				continue
			}
			
			if _, exists := metrics[topicName]; !exists {
				metrics[topicName] = &KafkaTopicMetrics{
					TopicName:   topicName,
					Internal:    strings.HasPrefix(topicName, "__"),
					LastUpdated: time.Now(),
				}
			}
			
			if len(result.Value) >= 2 {
				if valueStr, ok := result.Value[1].(string); ok {
					if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
						metrics[topicName].BytesPerSec = value
					}
				}
			}
		}
	}
	
	// 5. 获取分区日志大小 (聚合)
	logSizeResp, err := p.Query(ctx, "sum by (topic) (kafka_topic_partition_log_size_bytes)")
	if err == nil {
		for _, result := range logSizeResp.Data.Result {
			topicName := result.Metric["topic"]
			if topicName == "" {
				continue
			}
			
			if _, exists := metrics[topicName]; !exists {
				metrics[topicName] = &KafkaTopicMetrics{
					TopicName:   topicName,
					Internal:    strings.HasPrefix(topicName, "__"),
					LastUpdated: time.Now(),
				}
			}
			
			if len(result.Value) >= 2 {
				if valueStr, ok := result.Value[1].(string); ok {
					if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
						metrics[topicName].LogSizeBytes = value
					}
				}
			}
		}
	}
	
	// 6. 计算分区数量 (通过分区日志大小指标)
	partitionCountResp, err := p.Query(ctx, "count by (topic) (kafka_topic_partition_log_size_bytes)")
	if err == nil {
		for _, result := range partitionCountResp.Data.Result {
			topicName := result.Metric["topic"]
			if topicName == "" {
				continue
			}
			
			if _, exists := metrics[topicName]; !exists {
				metrics[topicName] = &KafkaTopicMetrics{
					TopicName:   topicName,
					Internal:    strings.HasPrefix(topicName, "__"),
					LastUpdated: time.Now(),
				}
			}
			
			if len(result.Value) >= 2 {
				if valueStr, ok := result.Value[1].(string); ok {
					if value, err := strconv.Atoi(valueStr); err == nil {
						metrics[topicName].PartitionCount = value
					}
				}
			}
		}
	}
	
	return metrics, nil
}

// GetKafkaTopicMetricsByName 获取特定 Topic 的指标
func (p *PrometheusService) GetKafkaTopicMetricsByName(ctx context.Context, topicName string) (*KafkaTopicMetrics, error) {
	allMetrics, err := p.GetKafkaTopicMetrics(ctx)
	if err != nil {
		return nil, err
	}
	
	if metric, exists := allMetrics[topicName]; exists {
		return metric, nil
	}
	
	return nil, fmt.Errorf("topic %s not found in metrics", topicName)
}

// GetKafkaBrokerMetrics 获取 Kafka Broker 指标
func (p *PrometheusService) GetKafkaBrokerMetrics(ctx context.Context) (map[string]*KafkaBrokerMetrics, error) {
	brokers := make(map[string]*KafkaBrokerMetrics)
	
	// 尝试多种可能的Broker指标查询
	queries := []string{
		"kafka_server_broker_id",
		"kafka_broker_bytes_in_per_sec",
		"kafka_server_brokertopicmetrics_bytesinpersec",
		"up{job=~\".*kafka.*\"}",
	}
	
	var brokerInstances []string
	
	// 尝试从不同的指标中获取Broker实例列表
	for _, query := range queries {
		resp, err := p.Query(ctx, fmt.Sprintf("group by (instance) (%s)", query))
		if err == nil && len(resp.Data.Result) > 0 {
			for _, result := range resp.Data.Result {
				if instance := result.Metric["instance"]; instance != "" {
					brokerInstances = append(brokerInstances, instance)
				}
			}
			break // 找到数据就停止尝试其他查询
		}
	}
	
	// 如果没有找到任何Broker实例，创建一个默认的
	if len(brokerInstances) == 0 {
		brokerInstances = []string{"localhost:9092"}
	}
	
	// 为每个Broker实例创建指标
	for i, instance := range brokerInstances {
		brokers[instance] = &KafkaBrokerMetrics{
			BrokerID:    instance,
			ID:          fmt.Sprintf("%d", i),
			LastUpdated: time.Now(),
		}
	}
	
	// 尝试获取网络指标
	networkQueries := []string{
		"kafka_server_brokertopicmetrics_bytesinpersec",
		"kafka_broker_bytes_in_per_sec",
		"rate(kafka_server_brokertopicmetrics_bytesin_total[5m])",
	}
	
	for _, query := range networkQueries {
		networkResp, err := p.Query(ctx, query)
		if err == nil {
			for _, result := range networkResp.Data.Result {
				instance := result.Metric["instance"]
				if broker, exists := brokers[instance]; exists {
					if len(result.Value) >= 2 {
						if valueStr, ok := result.Value[1].(string); ok {
							if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
								broker.BytesInPerSec = value
							}
						}
					}
				}
			}
			break // 找到数据就停止尝试其他查询
		}
	}
	
	return brokers, nil
}

// GetKafkaClusterMetrics 获取 Kafka 集群指标
func (p *PrometheusService) GetKafkaClusterMetrics(ctx context.Context) (*KafkaClusterMetrics, error) {
	metrics := &KafkaClusterMetrics{
		LastUpdated: time.Now(),
	}
	
	// 1. 获取Broker数量 - 使用实际存在的指标
	brokerCountResp, err := p.Query(ctx, "count(group by (instance) (kafka_broker_bytes_in_per_sec))")
	if err == nil && len(brokerCountResp.Data.Result) > 0 {
		if len(brokerCountResp.Data.Result[0].Value) >= 2 {
			if valueStr, ok := brokerCountResp.Data.Result[0].Value[1].(string); ok {
				if count, err := strconv.Atoi(valueStr); err == nil {
					metrics.BrokerCount = count
				}
			}
		}
	}
	
	// 2. 获取Topic数量
	topicCountResp, err := p.Query(ctx, "count(group by (topic) (kafka_topic_partition_log_size_bytes))")
	if err == nil && len(topicCountResp.Data.Result) > 0 {
		if len(topicCountResp.Data.Result[0].Value) >= 2 {
			if valueStr, ok := topicCountResp.Data.Result[0].Value[1].(string); ok {
				if count, err := strconv.Atoi(valueStr); err == nil {
					metrics.TopicCount = count
				}
			}
		}
	}
	
	// 3. 获取分区数量
	partitionCountResp, err := p.Query(ctx, "count(kafka_topic_partition_log_size_bytes)")
	if err == nil && len(partitionCountResp.Data.Result) > 0 {
		if len(partitionCountResp.Data.Result[0].Value) >= 2 {
			if valueStr, ok := partitionCountResp.Data.Result[0].Value[1].(string); ok {
				if count, err := strconv.Atoi(valueStr); err == nil {
					metrics.PartitionCount = count
				}
			}
		}
	}
	
	// 4. 获取集群总吞吐量
	totalBytesInResp, err := p.Query(ctx, "sum(kafka_server_brokertopicmetrics_bytesinpersec)")
	if err == nil && len(totalBytesInResp.Data.Result) > 0 {
		if len(totalBytesInResp.Data.Result[0].Value) >= 2 {
			if valueStr, ok := totalBytesInResp.Data.Result[0].Value[1].(string); ok {
				if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
					metrics.TotalBytesInPerSec = value
				}
			}
		}
	}
	
	return metrics, nil
}

// GetTopicListFromPrometheus 从Prometheus获取Topic列表
func (p *PrometheusService) GetTopicListFromPrometheus(ctx context.Context) ([]string, error) {
	resp, err := p.Query(ctx, "group by (topic) (kafka_topic_partition_log_size_bytes)")
	if err != nil {
		return nil, fmt.Errorf("failed to get topic list: %w", err)
	}
	
	var topics []string
	for _, result := range resp.Data.Result {
		if topicName := result.Metric["topic"]; topicName != "" {
			topics = append(topics, topicName)
		}
	}
	
	return topics, nil
}

// TopicOverview Topic 概览信息
type TopicOverview struct {
	Name              string  `json:"name"`
	PartitionCount    int     `json:"partition_count"`
	ReplicationFactor int     `json:"replication_factor"`
	OutOfSyncReplicas int     `json:"out_of_sync_replicas"`
	MessagesTotal     int64   `json:"messages_total"`
	SizeBytes         int64   `json:"size_bytes"`
	SizeFormatted     string  `json:"size_formatted"`
	MessagesPerSec    float64 `json:"messages_per_sec"`
	BytesPerSec       float64 `json:"bytes_per_sec"`
	IsInternal        bool    `json:"is_internal"`
	LastUpdated       time.Time `json:"last_updated"`
}

// BrokerOverview Broker 概览信息
type BrokerOverview struct {
	ID               int32   `json:"id"`
	Host             string  `json:"host"`
	Port             int32   `json:"port"`
	IsController     bool    `json:"is_controller"`
	PartitionCount   int     `json:"partition_count"`
	LeaderCount      int     `json:"leader_count"`
	BytesInPerSec    float64 `json:"bytes_in_per_sec"`
	BytesOutPerSec   float64 `json:"bytes_out_per_sec"`
	MessagesPerSec   float64 `json:"messages_per_sec"`
	JVMHeapUsed      int64   `json:"jvm_heap_used"`
	JVMHeapMax       int64   `json:"jvm_heap_max"`
	LastUpdated      time.Time `json:"last_updated"`
}

// ClusterOverview 集群概览信息
type ClusterOverview struct {
	Version                   string    `json:"version"`
	ControllerID              int32     `json:"controller_id"`
	BrokerCount               int       `json:"broker_count"`
	OnlineBrokers             int       `json:"online_brokers"`
	TotalPartitions           int       `json:"total_partitions"`
	OnlinePartitions          int       `json:"online_partitions"`
	UnderReplicatedPartitions int       `json:"under_replicated_partitions"`
	PreferredReplicaImbalance int       `json:"preferred_replica_imbalance"`
	LastUpdated               time.Time `json:"last_updated"`
}

// GetAllTopicsOverview 获取所有 Topics 的概览信息
func (p *PrometheusService) GetAllTopicsOverview(ctx context.Context) (map[string]*TopicOverview, error) {
	topics := make(map[string]*TopicOverview)
	
	// 1. 获取所有 Topic 的分区数量
	partitionCountResp, err := p.Query(ctx, "count by (topic) (kafka_topic_partition_log_size_bytes)")
	if err != nil {
		return nil, fmt.Errorf("failed to get partition counts: %w", err)
	}
	
	for _, result := range partitionCountResp.Data.Result {
		topicName := result.Metric["topic"]
		if topicName == "" {
			continue
		}
		
		topics[topicName] = &TopicOverview{
			Name:              topicName,
			IsInternal:        strings.HasPrefix(topicName, "__"),
			ReplicationFactor: 1, // 默认值，后续可以通过其他方式获取
			LastUpdated:       time.Now(),
		}
		
		if len(result.Value) >= 2 {
			if valueStr, ok := result.Value[1].(string); ok {
				if count, err := strconv.Atoi(valueStr); err == nil {
					topics[topicName].PartitionCount = count
				}
			}
		}
	}
	
	// 2. 获取 Topic 总大小
	sizeResp, err := p.Query(ctx, "sum by (topic) (kafka_topic_partition_log_size_bytes)")
	if err == nil {
		for _, result := range sizeResp.Data.Result {
			topicName := result.Metric["topic"]
			if topic, exists := topics[topicName]; exists {
				if len(result.Value) >= 2 {
					if valueStr, ok := result.Value[1].(string); ok {
						if size, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
							topic.SizeBytes = size
							topic.SizeFormatted = formatBytes(size)
						}
					}
				}
			}
		}
	}
	
	// 3. 获取消息总数
	messagesResp, err := p.Query(ctx, "kafka_topic_messages_total")
	if err == nil {
		for _, result := range messagesResp.Data.Result {
			topicName := result.Metric["topic"]
			if topic, exists := topics[topicName]; exists {
				if len(result.Value) >= 2 {
					if valueStr, ok := result.Value[1].(string); ok {
						if messages, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
							topic.MessagesTotal = messages
						}
					}
				}
			}
		}
	}
	
	// 4. 获取消息速率
	messagesPerSecResp, err := p.Query(ctx, "kafka_topic_messages_per_sec")
	if err == nil {
		for _, result := range messagesPerSecResp.Data.Result {
			topicName := result.Metric["topic"]
			if topic, exists := topics[topicName]; exists {
				if len(result.Value) >= 2 {
					if valueStr, ok := result.Value[1].(string); ok {
						if rate, err := strconv.ParseFloat(valueStr, 64); err == nil {
							topic.MessagesPerSec = rate
						}
					}
				}
			}
		}
	}
	
	// 5. 获取字节速率
	bytesPerSecResp, err := p.Query(ctx, "kafka_topic_bytes_in_per_sec")
	if err == nil {
		for _, result := range bytesPerSecResp.Data.Result {
			topicName := result.Metric["topic"]
			if topic, exists := topics[topicName]; exists {
				if len(result.Value) >= 2 {
					if valueStr, ok := result.Value[1].(string); ok {
						if rate, err := strconv.ParseFloat(valueStr, 64); err == nil {
							topic.BytesPerSec = rate
						}
					}
				}
			}
		}
	}
	
	return topics, nil
}

// GetTopicPartitionSizes 获取特定 Topic 的分区大小信息
func (p *PrometheusService) GetTopicPartitionSizes(ctx context.Context, topicName string) (map[int32]int64, error) {
	query := fmt.Sprintf(`kafka_topic_partition_log_size_bytes{topic="%s"}`, topicName)
	resp, err := p.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition sizes: %w", err)
	}
	
	partitionSizes := make(map[int32]int64)
	for _, result := range resp.Data.Result {
		partitionStr := result.Metric["partition"]
		if partitionStr == "" {
			continue
		}
		
		partition, err := strconv.ParseInt(partitionStr, 10, 32)
		if err != nil {
			continue
		}
		
		if len(result.Value) >= 2 {
			if valueStr, ok := result.Value[1].(string); ok {
				if size, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
					partitionSizes[int32(partition)] = size
				}
			}
		}
	}
	
	return partitionSizes, nil
}

// GetBrokerOverview 获取 Broker 概览信息
func (p *PrometheusService) GetBrokerOverview(ctx context.Context) (*ClusterOverview, []*BrokerOverview, error) {
	// 集群概览
	cluster := &ClusterOverview{
		Version:     "3.4.0", // 默认值
		LastUpdated: time.Now(),
	}
	
	// 获取分区总数
	partitionCountResp, err := p.Query(ctx, "count(kafka_topic_partition_log_size_bytes)")
	if err == nil && len(partitionCountResp.Data.Result) > 0 {
		if len(partitionCountResp.Data.Result[0].Value) >= 2 {
			if valueStr, ok := partitionCountResp.Data.Result[0].Value[1].(string); ok {
				if count, err := strconv.Atoi(valueStr); err == nil {
					cluster.TotalPartitions = count
					cluster.OnlinePartitions = count // 假设所有分区都在线
				}
			}
		}
	}
	
	// 获取 Broker 信息
	var brokers []*BrokerOverview
	
	// 尝试从不同的指标获取 Broker 实例
	brokerQueries := []string{
		"kafka_broker_bytes_in_per_sec",
		"kafka_server_brokertopicmetrics_bytesinpersec",
		"up{job=\"kafka\"}",
	}
	
	var brokerInstances []string
	for _, query := range brokerQueries {
		resp, err := p.Query(ctx, fmt.Sprintf("group by (instance) (%s)", query))
		if err == nil && len(resp.Data.Result) > 0 {
			for _, result := range resp.Data.Result {
				if instance := result.Metric["instance"]; instance != "" {
					brokerInstances = append(brokerInstances, instance)
				}
			}
			break
		}
	}
	
	// 为每个 Broker 创建概览信息
	for i, instance := range brokerInstances {
		broker := &BrokerOverview{
			ID:          int32(i + 1),
			Host:        strings.Split(instance, ":")[0],
			Port:        9092,
			LastUpdated: time.Now(),
		}
		
		// 获取网络指标
		networkResp, err := p.Query(ctx, fmt.Sprintf("kafka_broker_bytes_in_per_sec{instance=\"%s\"}", instance))
		if err == nil && len(networkResp.Data.Result) > 0 {
			if len(networkResp.Data.Result[0].Value) >= 2 {
				if valueStr, ok := networkResp.Data.Result[0].Value[1].(string); ok {
					if rate, err := strconv.ParseFloat(valueStr, 64); err == nil {
						broker.BytesInPerSec = rate
					}
				}
			}
		}
		
		// 获取 JVM 指标
		jvmResp, err := p.Query(ctx, fmt.Sprintf("kafka_jvm_memory_heap_used_bytes{instance=\"%s\"}", instance))
		if err == nil && len(jvmResp.Data.Result) > 0 {
			if len(jvmResp.Data.Result[0].Value) >= 2 {
				if valueStr, ok := jvmResp.Data.Result[0].Value[1].(string); ok {
					if heap, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
						broker.JVMHeapUsed = heap
					}
				}
			}
		}
		
		brokers = append(brokers, broker)
	}
	
	cluster.BrokerCount = len(brokers)
	cluster.OnlineBrokers = len(brokers)
	
	return cluster, brokers, nil
}

// formatBytes 格式化字节数 (移动到这里避免重复)
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// TestConnection 测试 Prometheus 连接
func (p *PrometheusService) TestConnection(ctx context.Context) error {
	_, err := p.Query(ctx, "up")
	return err
}
