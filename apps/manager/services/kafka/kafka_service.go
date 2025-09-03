package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/sysarmor/sysarmor/apps/manager/services/prometheus"
)

var (
	ErrTopicExists    = errors.New("topic already exists")
	ErrTopicNotFound  = errors.New("topic not found")
	ErrBrokerNotFound = errors.New("broker not found")
)

// KafkaService Kafka 管理服务 - 单一数据源：Prometheus
type KafkaService struct {
	brokers []string
	config  *sarama.Config
	// Prometheus服务用于获取指标数据（单一数据源）
	promService *prometheus.PrometheusService
}

// NewKafkaService 创建 Kafka 服务
func NewKafkaService(brokers []string) *KafkaService {
	config := sarama.NewConfig()
	config.Version = sarama.V3_4_0_0  // 更新到更新的版本
	config.Admin.Timeout = 30 * time.Second  // 增加超时时间
	config.Metadata.Timeout = 30 * time.Second
	config.Metadata.RefreshFrequency = 10 * time.Minute
	config.Consumer.Return.Errors = true
	config.Net.DialTimeout = 30 * time.Second
	config.Net.ReadTimeout = 30 * time.Second
	config.Net.WriteTimeout = 30 * time.Second
	
	return &KafkaService{
		brokers:     brokers,
		config:      config,
		promService: prometheus.NewPrometheusService(),
	}
}

// ClusterInfo 集群信息
type ClusterInfo struct {
	ClusterID             string           `json:"cluster_id"`
	Name                  string           `json:"name"`
	Status                string           `json:"status"`
	BrokerCount           int              `json:"broker_count"`
	ActiveControllers     int              `json:"active_controllers"`
	TopicCount            int              `json:"topic_count"`
	OnlinePartitionCount  int              `json:"online_partition_count"`
	Version               string           `json:"version"`
	Features              []string         `json:"features"`
	LastError             *string          `json:"last_error"`
	CheckedAt             time.Time        `json:"checked_at"`
	PartitionStats        PartitionStats   `json:"partition_stats"`
	ReplicaStats          ReplicaStats     `json:"replica_stats"`
	HealthStatus          string           `json:"health_status"`
}

// PartitionStats 分区统计信息
type PartitionStats struct {
	TotalPartitions              int `json:"total_partitions"`
	OnlinePartitions             int `json:"online_partitions"`
	OfflinePartitions            int `json:"offline_partitions"`
	UnderReplicatedPartitions    int `json:"under_replicated_partitions"`
	PreferredReplicaImbalance    int `json:"preferred_replica_imbalance"`
}

// ReplicaStats 副本统计信息
type ReplicaStats struct {
	TotalReplicas     int `json:"total_replicas"`
	InSyncReplicas    int `json:"in_sync_replicas"`
	OutOfSyncReplicas int `json:"out_of_sync_replicas"`
	PreferredReplicas int `json:"preferred_replicas"`
}

// BrokerInfo Broker 信息
type BrokerInfo struct {
	ID                    int32            `json:"id"`
	Host                  string           `json:"host"`
	Port                  int32            `json:"port"`
	Rack                  *string          `json:"rack"`
	Controller            bool             `json:"controller"`
	Version               string           `json:"version"`
	JMXPort               int32            `json:"jmx_port"`
	Timestamp             time.Time        `json:"timestamp"`
	DiskUsage             DiskUsage        `json:"disk_usage"`
	SegmentCount          int              `json:"segment_count"`
	SegmentSize           string           `json:"segment_size"`
	PartitionsLeader      int              `json:"partitions_leader"`
	PartitionsSkew        float64          `json:"partitions_skew"`
	LeadersSkew           float64          `json:"leaders_skew"`
	InSyncPartitions      int              `json:"in_sync_partitions"`
	OutOfSyncPartitions   int              `json:"out_of_sync_partitions"`
	NetworkStats          NetworkStats     `json:"network_stats"`
	JVMStats              JVMStats         `json:"jvm_stats"`
}

// DiskUsage 磁盘使用情况
type DiskUsage struct {
	TotalBytes      int64   `json:"total_bytes"`
	UsedBytes       int64   `json:"used_bytes"`
	FreeBytes       int64   `json:"free_bytes"`
	UsagePercentage float64 `json:"usage_percentage"`
}

// NetworkStats 网络统计
type NetworkStats struct {
	BytesInPerSec     int64 `json:"bytes_in_per_sec"`
	BytesOutPerSec    int64 `json:"bytes_out_per_sec"`
	MessagesInPerSec  int64 `json:"messages_in_per_sec"`
}

// JVMStats JVM 统计
type JVMStats struct {
	HeapUsed string `json:"heap_used"`
	HeapMax  string `json:"heap_max"`
	GCCount  int64  `json:"gc_count"`
	Uptime   string `json:"uptime"`
}

// TopicInfo Topic 信息（增强版，包含大小和消息数量）
type TopicInfo struct {
	Name                        string          `json:"name"`
	Internal                    bool            `json:"internal"`
	PartitionCount              int             `json:"partition_count"`
	ReplicationFactor           int             `json:"replication_factor"`
	Replicas                    int             `json:"replicas"`
	InSyncReplicas              int             `json:"in_sync_replicas"`
	UnderReplicatedPartitions   int             `json:"under_replicated_partitions"`
	CleanUpPolicy               string          `json:"clean_up_policy"`
	SegmentSize                 int64           `json:"segment_size"`
	SegmentCount                int             `json:"segment_count"`
	BytesInPerSec               *int64          `json:"bytes_in_per_sec"`
	BytesOutPerSec              *int64          `json:"bytes_out_per_sec"`
	// 新增增强字段
	TotalMessages               int64           `json:"total_messages"`        // 总消息数量
	TotalSizeBytes              int64           `json:"total_size_bytes"`      // 总大小（字节）
	TotalSizeFormatted          string          `json:"total_size_formatted"`  // 格式化的大小
	RetentionMs                 int64           `json:"retention_ms"`          // 保留时间（毫秒）
	RetentionFormatted          string          `json:"retention_formatted"`   // 格式化的保留时间
	OldestMessageTimestamp      *time.Time      `json:"oldest_message_timestamp"` // 最老消息时间戳
	NewestMessageTimestamp      *time.Time      `json:"newest_message_timestamp"` // 最新消息时间戳
	MessagesPerSecond           float64         `json:"messages_per_second"`   // 每秒消息数
	LastUpdated                 time.Time       `json:"last_updated"`          // 最后更新时间
	Partitions                  []PartitionInfo `json:"partitions,omitempty"`
}

// PartitionInfo 分区信息
type PartitionInfo struct {
	Partition int32         `json:"partition"`
	Leader    int32         `json:"leader"`
	Replicas  []ReplicaInfo `json:"replicas"`
	OffsetMax int64         `json:"offset_max"`
	OffsetMin int64         `json:"offset_min"`
}

// ReplicaInfo 副本信息
type ReplicaInfo struct {
	Broker int32 `json:"broker"`
	Leader bool  `json:"leader"`
	InSync bool  `json:"in_sync"`
}

// TopicsResponse Topics 响应
type TopicsResponse struct {
	PageCount int         `json:"page_count"`
	Topics    []TopicInfo `json:"topics"`
	Total     int         `json:"total"`
	Page      int         `json:"page"`
	Limit     int         `json:"limit"`
}

// MessageInfo 消息信息
type MessageInfo struct {
	Partition int32                  `json:"partition"`
	Offset    int64                  `json:"offset"`
	Key       *string                `json:"key"`
	Value     string                 `json:"value"`
	Headers   map[string]string      `json:"headers"`
	Timestamp time.Time              `json:"timestamp"`
	Size      int                    `json:"size"`
}

// MessagesResponse 消息响应
type MessagesResponse struct {
	Messages      []MessageInfo `json:"messages"`
	Total         int           `json:"total"`
	BytesConsumed int64         `json:"bytes_consumed"`
	ElapsedMs     int64         `json:"elapsed_ms"`
}

// ConsumerGroupInfo Consumer Group 信息
type ConsumerGroupInfo struct {
	GroupID     string                    `json:"group_id"`
	State       string                    `json:"state"`
	Members     []ConsumerGroupMember     `json:"members"`
	Coordinator int32                     `json:"coordinator"`
	Lag         int64                     `json:"lag"`
	Topics      []string                  `json:"topics"`
	Offsets     []ConsumerGroupOffset     `json:"offsets,omitempty"`
}

// ConsumerGroupMember Consumer Group 成员
type ConsumerGroupMember struct {
	MemberID     string   `json:"member_id"`
	ClientID     string   `json:"client_id"`
	ClientHost   string   `json:"client_host"`
	Assignment   []string `json:"assignment"`
}

// ConsumerGroupOffset Consumer Group 偏移量
type ConsumerGroupOffset struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	Lag       int64  `json:"lag"`
}

// CreateTopicRequest 创建 Topic 请求
type CreateTopicRequest struct {
	Name              string            `json:"name" binding:"required"`
	Partitions        int32             `json:"partitions"`
	ReplicationFactor int16             `json:"replication_factor"`
	Config            map[string]string `json:"config,omitempty"`
}

// TopicConfig Topic 配置信息
type TopicConfig struct {
	Name        string                 `json:"name"`
	Configs     map[string]ConfigEntry `json:"configs"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ConfigEntry 配置项
type ConfigEntry struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Default   bool   `json:"default"`
	ReadOnly  bool   `json:"read_only"`
	Sensitive bool   `json:"sensitive"`
	Source    string `json:"source"`
}

// BasicTopic 基础的Topic信息
type BasicTopic struct {
	Name           string `json:"name"`
	PartitionCount int    `json:"partition_count"`
	Internal       bool   `json:"internal"`
}

// BasicTopicsResponse 基础的Topics响应
type BasicTopicsResponse struct {
	Topics    []BasicTopic `json:"topics"`
	Total     int          `json:"total"`
	Page      int          `json:"page"`
	Limit     int          `json:"limit"`
	PageCount int          `json:"page_count"`
}

// EnhancedTopic 增强的Topic信息 (包含Prometheus指标)
type EnhancedTopic struct {
	Name              string  `json:"name"`
	PartitionCount    int     `json:"partition_count"`
	Internal          bool    `json:"internal"`
	MessagesTotal     int64   `json:"messages_total"`
	BytesTotal        int64   `json:"bytes_total"`
	BytesTotalFormatted string `json:"bytes_total_formatted"`
	MessagesPerSec    float64 `json:"messages_per_sec"`
	BytesPerSec       float64 `json:"bytes_per_sec"`
	LogSizeBytes      int64   `json:"log_size_bytes"`
	LastUpdated       string  `json:"last_updated"`
}

// EnhancedTopicsResponse 增强的Topics响应
type EnhancedTopicsResponse struct {
	Topics    []EnhancedTopic `json:"topics"`
	Total     int             `json:"total"`
	Page      int             `json:"page"`
	Limit     int             `json:"limit"`
	PageCount int             `json:"page_count"`
	DataSource string         `json:"data_source"` // "prometheus" 或 "kafka"
}

// TopicMetrics Topic 指标信息
type TopicMetrics struct {
	TopicName           string    `json:"topic_name"`
	TotalMessages       int64     `json:"total_messages"`
	TotalSizeBytes      int64     `json:"total_size_bytes"`
	MessagesPerSecond   float64   `json:"messages_per_second"`
	BytesPerSecond      float64   `json:"bytes_per_second"`
	PartitionCount      int       `json:"partition_count"`
	ReplicationFactor   int       `json:"replication_factor"`
	RetentionMs         int64     `json:"retention_ms"`
	LastUpdated         time.Time `json:"last_updated"`
}

// GetClusters 获取集群信息 - 单一数据源：Prometheus
func (s *KafkaService) GetClusters(ctx context.Context) ([]ClusterInfo, error) {
	// 从Prometheus获取集群指标
	clusterMetrics, err := s.promService.GetKafkaClusterMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster metrics from Prometheus: %w", err)
	}

	// 获取Topic指标来计算详细统计
	topicMetrics, err := s.promService.GetKafkaTopicMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metrics from Prometheus: %w", err)
	}

	// 计算分区和副本统计
	partitionStats := PartitionStats{
		TotalPartitions:              clusterMetrics.PartitionCount,
		OnlinePartitions:             clusterMetrics.PartitionCount, // 假设所有分区都在线
		OfflinePartitions:            0,
		UnderReplicatedPartitions:    0, // 需要从Prometheus获取更详细的指标
		PreferredReplicaImbalance:    0,
	}

	// 计算副本统计（基于分区数量和默认复制因子）
	totalReplicas := 0
	for _, metric := range topicMetrics {
		totalReplicas += metric.PartitionCount // 假设复制因子为1
	}

	replicaStats := ReplicaStats{
		TotalReplicas:     totalReplicas,
		InSyncReplicas:    totalReplicas, // 假设所有副本都同步
		OutOfSyncReplicas: 0,
		PreferredReplicas: totalReplicas,
	}

	cluster := ClusterInfo{
		Name:                 "kafka-cluster",
		Status:               "online",
		BrokerCount:          clusterMetrics.BrokerCount,
		TopicCount:           clusterMetrics.TopicCount,
		OnlinePartitionCount: clusterMetrics.PartitionCount,
		Version:              "3.4-IV0",
		Features:             []string{"TOPIC_DELETION"},
		LastError:            nil,
		CheckedAt:            time.Now(),
		PartitionStats:       partitionStats,
		ReplicaStats:         replicaStats,
		HealthStatus:         "healthy",
	}
	return []ClusterInfo{cluster}, nil
}


// GetBrokers 获取 Broker 信息 - 单一数据源：Prometheus
func (s *KafkaService) GetBrokers(ctx context.Context) ([]BrokerInfo, error) {
	// 从Prometheus获取Broker指标
	brokerMetrics, err := s.promService.GetKafkaBrokerMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get broker metrics from Prometheus: %w", err)
	}

	if len(brokerMetrics) == 0 {
		return nil, fmt.Errorf("no broker metrics found in Prometheus")
	}

	var brokerInfos []BrokerInfo
	for _, metric := range brokerMetrics {
		// 解析Broker ID
		brokerID, _ := strconv.ParseInt(metric.ID, 10, 32)
		
		brokerInfo := BrokerInfo{
			ID:                  int32(brokerID),
			Host:                metric.BrokerID, // instance地址
			Port:                9092,
			Rack:                nil,
			Controller:          false,
			Version:             "3.4-IV0",
			JMXPort:             9999,
			Timestamp:           time.Now(),
			DiskUsage:           DiskUsage{},
			SegmentCount:        0,
			SegmentSize:         "0 MB",
			PartitionsLeader:    0,
			PartitionsSkew:      0.0,
			LeadersSkew:         0.0,
			InSyncPartitions:    0,
			OutOfSyncPartitions: 0,
			NetworkStats: NetworkStats{
				BytesInPerSec: int64(metric.BytesInPerSec),
			},
			JVMStats: JVMStats{},
		}
		brokerInfos = append(brokerInfos, brokerInfo)
	}
	return brokerInfos, nil
}


// GetTopicsListEnhanced 获取增强的 Topics 列表 - 单一数据源：Prometheus
func (s *KafkaService) GetTopicsListEnhanced(ctx context.Context, page, limit int, search string) (*EnhancedTopicsResponse, error) {
	// 从 Prometheus 获取指标
	promMetrics, err := s.promService.GetKafkaTopicMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metrics from Prometheus: %w", err)
	}

	if len(promMetrics) == 0 {
		return &EnhancedTopicsResponse{
			Topics:     []EnhancedTopic{},
			Total:      0,
			Page:       page,
			Limit:      limit,
			PageCount:  0,
			DataSource: "prometheus",
		}, nil
	}

	// 使用 Prometheus 数据
	var enhancedTopics []EnhancedTopic
	
	// 过滤和搜索
	for topicName, metric := range promMetrics {
		if search != "" && !strings.Contains(strings.ToLower(topicName), strings.ToLower(search)) {
			continue
		}
		
		enhancedTopics = append(enhancedTopics, EnhancedTopic{
			Name:                metric.TopicName,
			PartitionCount:      metric.PartitionCount,
			Internal:            metric.Internal,
			MessagesTotal:       metric.MessagesTotal,
			BytesTotal:          metric.BytesTotal,
			BytesTotalFormatted: formatBytes(metric.BytesTotal),
			MessagesPerSec:      metric.MessagesPerSec,
			BytesPerSec:         metric.BytesPerSec,
			LogSizeBytes:        metric.LogSizeBytes,
			LastUpdated:         metric.LastUpdated.Format("2006-01-02 15:04:05"),
		})
	}
	
	total := len(enhancedTopics)
	pageCount := (total + limit - 1) / limit
	
	// 分页
	start := (page - 1) * limit
	end := start + limit
	if end > total {
		end = total
	}
	if start > total {
		start = total
	}
	
	pagedTopics := enhancedTopics[start:end]
	
	return &EnhancedTopicsResponse{
		Topics:     pagedTopics,
		Total:      total,
		Page:       page,
		Limit:      limit,
		PageCount:  pageCount,
		DataSource: "prometheus",
	}, nil
}


// GetTopicsList 获取 Topics 列表（简化版本）- 单一数据源：Prometheus
func (s *KafkaService) GetTopicsList(ctx context.Context, page, limit int, search string) (*BasicTopicsResponse, error) {
	// 从Prometheus获取Topic列表
	topics, err := s.promService.GetTopicListFromPrometheus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic list from Prometheus: %w", err)
	}

	// 使用Prometheus数据
	var filteredTopics []string
	for _, topicName := range topics {
		if search != "" && !strings.Contains(strings.ToLower(topicName), strings.ToLower(search)) {
			continue
		}
		filteredTopics = append(filteredTopics, topicName)
	}

	total := len(filteredTopics)
	pageCount := (total + limit - 1) / limit

	// 分页
	start := (page - 1) * limit
	end := start + limit
	if end > total {
		end = total
	}
	if start > total {
		start = total
	}

	var basicTopics []BasicTopic
	// 获取Topic指标来补充分区信息
	promMetrics, _ := s.promService.GetKafkaTopicMetrics(ctx)
	
	for i := start; i < end; i++ {
		topicName := filteredTopics[i]
		partitionCount := 0
		
		if metric, exists := promMetrics[topicName]; exists {
			partitionCount = metric.PartitionCount
		}

		basicTopics = append(basicTopics, BasicTopic{
			Name:           topicName,
			PartitionCount: partitionCount,
			Internal:       strings.HasPrefix(topicName, "__"),
		})
	}

	return &BasicTopicsResponse{
		Topics:    basicTopics,
		Total:     total,
		Page:      page,
		Limit:     limit,
		PageCount: pageCount,
	}, nil
}


// GetTopicDetails 获取 Topic 详细信息 - 单一数据源：Prometheus
func (s *KafkaService) GetTopicDetails(ctx context.Context, topicName string) (*TopicInfo, error) {
	// 从Prometheus获取指标数据
	promMetric, err := s.promService.GetKafkaTopicMetricsByName(ctx, topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metrics from Prometheus: %w", err)
	}

	// 构建TopicInfo，使用Prometheus数据
	topicInfo := &TopicInfo{
		Name:                      topicName,
		Internal:                  strings.HasPrefix(topicName, "__"),
		PartitionCount:            promMetric.PartitionCount,
		ReplicationFactor:         1, // 默认值，Prometheus通常不提供此信息
		Replicas:                  promMetric.PartitionCount, // 估算值
		InSyncReplicas:            promMetric.PartitionCount, // 估算值
		UnderReplicatedPartitions: 0, // 默认值
		CleanUpPolicy:             "DELETE",
		SegmentSize:               promMetric.BytesTotal,
		SegmentCount:              promMetric.PartitionCount,
		BytesInPerSec:             nil,
		BytesOutPerSec:            nil,
		TotalMessages:             promMetric.MessagesTotal,
		TotalSizeBytes:            promMetric.BytesTotal,
		TotalSizeFormatted:        formatBytes(promMetric.BytesTotal),
		RetentionMs:               604800000, // 7天默认
		RetentionFormatted:        "7d",
		MessagesPerSecond:         promMetric.MessagesPerSec,
		LastUpdated:               promMetric.LastUpdated,
		Partitions:                nil, // 不提供详细分区信息，因为只使用Prometheus
	}

	return topicInfo, nil
}


// GetTopicMessages 获取 Topic 消息 - 必须使用Sarama
func (s *KafkaService) GetTopicMessages(ctx context.Context, topicName string, limit int, partition int, offset string) (*MessagesResponse, error) {
	consumer, err := sarama.NewConsumer(s.brokers, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}
	defer consumer.Close()

	// 获取分区列表
	partitions, err := consumer.Partitions(topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}

	var targetPartitions []int32
	if partition >= 0 {
		// 检查分区是否存在
		found := false
		for _, p := range partitions {
			if p == int32(partition) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("partition %d not found", partition)
		}
		targetPartitions = []int32{int32(partition)}
	} else {
		targetPartitions = partitions
	}

	var messages []MessageInfo
	var bytesConsumed int64
	startTime := time.Now()

	// 如果请求最新消息，我们需要从每个分区的末尾向前读取
	if offset == "latest" || offset == "" {
		return s.getLatestMessages(consumer, topicName, targetPartitions, limit)
	}

	for _, p := range targetPartitions {
		if len(messages) >= limit {
			break
		}

		// 确定起始偏移量
		var startOffset int64
		switch offset {
		case "earliest":
			startOffset = sarama.OffsetOldest
		default:
			// 尝试解析为数字
			if parsedOffset, err := strconv.ParseInt(offset, 10, 64); err == nil {
				startOffset = parsedOffset
			} else {
				startOffset = sarama.OffsetNewest
			}
		}

		partitionConsumer, err := consumer.ConsumePartition(topicName, p, startOffset)
		if err != nil {
			continue
		}

		// 消费消息
		messageCount := 0
		maxMessages := limit - len(messages)

	consumeLoop:
		for messageCount < maxMessages {
			select {
			case msg := <-partitionConsumer.Messages():
				if msg == nil {
					break consumeLoop
				}

				headers := make(map[string]string)
				for _, header := range msg.Headers {
					headers[string(header.Key)] = string(header.Value)
				}

				var key *string
				if msg.Key != nil {
					keyStr := string(msg.Key)
					key = &keyStr
				}

				messages = append(messages, MessageInfo{
					Partition: msg.Partition,
					Offset:    msg.Offset,
					Key:       key,
					Value:     string(msg.Value),
					Headers:   headers,
					Timestamp: msg.Timestamp,
					Size:      len(msg.Value),
				})

				bytesConsumed += int64(len(msg.Value))
				messageCount++

			case <-time.After(1 * time.Second):
				// 超时退出
				break consumeLoop
			}
		}

		partitionConsumer.Close()
	}

	elapsedMs := time.Since(startTime).Milliseconds()

	return &MessagesResponse{
		Messages:      messages,
		Total:         len(messages),
		BytesConsumed: bytesConsumed,
		ElapsedMs:     elapsedMs,
	}, nil
}

// getLatestMessages 获取最新的消息
func (s *KafkaService) getLatestMessages(consumer sarama.Consumer, topicName string, partitions []int32, limit int) (*MessagesResponse, error) {
	var allMessages []MessageInfo
	var bytesConsumed int64
	startTime := time.Now()

	// 创建一个新的client来获取偏移量
	client, err := sarama.NewClient(s.brokers, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// 为每个分区收集消息
	for _, p := range partitions {
		if len(allMessages) >= limit {
			break
		}

		// 获取分区的最新偏移量和最早偏移量
		latestOffset, err := client.GetOffset(topicName, p, sarama.OffsetNewest)
		if err != nil {
			continue
		}

		earliestOffset, err := client.GetOffset(topicName, p, sarama.OffsetOldest)
		if err != nil {
			continue
		}

		// 如果分区为空，跳过
		if latestOffset <= earliestOffset {
			continue
		}

		// 计算要读取的消息数量
		messagesPerPartition := limit / len(partitions)
		if messagesPerPartition == 0 {
			messagesPerPartition = 1
		}

		// 从最新位置向前计算起始偏移量
		startOffset := latestOffset - int64(messagesPerPartition)
		if startOffset < earliestOffset {
			startOffset = earliestOffset
		}

		partitionConsumer, err := consumer.ConsumePartition(topicName, p, startOffset)
		if err != nil {
			continue
		}

		// 收集消息
		messageCount := 0
		timeout := time.After(3 * time.Second)

	partitionLoop:
		for messageCount < messagesPerPartition && len(allMessages) < limit {
			select {
			case msg := <-partitionConsumer.Messages():
				if msg == nil {
					break partitionLoop
				}

				headers := make(map[string]string)
				for _, header := range msg.Headers {
					headers[string(header.Key)] = string(header.Value)
				}

				var key *string
				if msg.Key != nil {
					keyStr := string(msg.Key)
					key = &keyStr
				}

				allMessages = append(allMessages, MessageInfo{
					Partition: msg.Partition,
					Offset:    msg.Offset,
					Key:       key,
					Value:     string(msg.Value),
					Headers:   headers,
					Timestamp: msg.Timestamp,
					Size:      len(msg.Value),
				})

				bytesConsumed += int64(len(msg.Value))
				messageCount++

			case <-timeout:
				break partitionLoop
			}
		}

		partitionConsumer.Close()
	}

	elapsedMs := time.Since(startTime).Milliseconds()

	return &MessagesResponse{
		Messages:      allMessages,
		Total:         len(allMessages),
		BytesConsumed: bytesConsumed,
		ElapsedMs:     elapsedMs,
	}, nil
}

// GetConsumerGroups 获取 Consumer Groups - 必须使用Sarama
func (s *KafkaService) GetConsumerGroups(ctx context.Context) ([]ConsumerGroupInfo, error) {
	// 注意：Sarama 的基础客户端不直接支持获取 Consumer Groups
	// 这里返回空列表，实际实现需要使用 Admin API 或其他方法
	return []ConsumerGroupInfo{}, nil
}

// GetConsumerGroupDetails 获取 Consumer Group 详细信息 - 必须使用Sarama
func (s *KafkaService) GetConsumerGroupDetails(ctx context.Context, groupName string) (*ConsumerGroupInfo, error) {
	// 注意：这里需要实现具体的 Consumer Group 查询逻辑
	return nil, fmt.Errorf("consumer group details not implemented yet")
}

// CreateTopic 创建 Topic - 必须使用Sarama
func (s *KafkaService) CreateTopic(ctx context.Context, req *CreateTopicRequest) error {
	admin, err := sarama.NewClusterAdmin(s.brokers, s.config)
	if err != nil {
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	// 设置默认值
	if req.Partitions <= 0 {
		req.Partitions = 3
	}
	if req.ReplicationFactor <= 0 {
		req.ReplicationFactor = 1
	}

	// 转换配置为 Sarama 需要的格式
	configEntries := make(map[string]*string)
	for k, v := range req.Config {
		configEntries[k] = &v
	}

	topicDetail := &sarama.TopicDetail{
		NumPartitions:     req.Partitions,
		ReplicationFactor: req.ReplicationFactor,
		ConfigEntries:     configEntries,
	}

	err = admin.CreateTopic(req.Name, topicDetail, false)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return ErrTopicExists
		}
		return fmt.Errorf("failed to create topic: %w", err)
	}

	return nil
}

// DeleteTopic 删除 Topic - 必须使用Sarama
func (s *KafkaService) DeleteTopic(ctx context.Context, topicName string, force bool) error {
	admin, err := sarama.NewClusterAdmin(s.brokers, s.config)
	if err != nil {
		if force {
			// 强制模式下，即使无法创建 admin client 也不返回错误
			return nil
		}
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	err = admin.DeleteTopic(topicName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			if force {
				// 强制模式下，topic 不存在也不算错误
				return nil
			}
			return ErrTopicNotFound
		}
		if force {
			// 强制模式下，其他错误也不返回
			return nil
		}
		return fmt.Errorf("failed to delete topic: %w", err)
	}

	return nil
}

// GetTopicConfig 获取 Topic 配置 - 必须使用Sarama
func (s *KafkaService) GetTopicConfig(ctx context.Context, topicName string) (*TopicConfig, error) {
	admin, err := sarama.NewClusterAdmin(s.brokers, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	// 检查 topic 是否存在
	client, err := sarama.NewClient(s.brokers, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	topics, err := client.Topics()
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %w", err)
	}

	found := false
	for _, topic := range topics {
		if topic == topicName {
			found = true
			break
		}
	}

	if !found {
		return nil, ErrTopicNotFound
	}

	// 获取 topic 配置
	configResource := sarama.ConfigResource{
		Type: sarama.TopicResource,
		Name: topicName,
	}

	configs, err := admin.DescribeConfig(configResource)
	if err != nil {
		return nil, fmt.Errorf("failed to describe topic config: %w", err)
	}

	configEntries := make(map[string]ConfigEntry)
	for _, entry := range configs {
		configEntries[entry.Name] = ConfigEntry{
			Name:      entry.Name,
			Value:     entry.Value,
			Default:   entry.Default,
			ReadOnly:  entry.ReadOnly,
			Sensitive: entry.Sensitive,
			Source:    string(entry.Source),
		}
	}

	return &TopicConfig{
		Name:      topicName,
		Configs:   configEntries,
		UpdatedAt: time.Now(),
	}, nil
}

// UpdateTopicConfig 更新 Topic 配置 - 必须使用Sarama
func (s *KafkaService) UpdateTopicConfig(ctx context.Context, topicName string, configs map[string]string) error {
	admin, err := sarama.NewClusterAdmin(s.brokers, s.config)
	if err != nil {
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	// 检查 topic 是否存在
	client, err := sarama.NewClient(s.brokers, s.config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	topics, err := client.Topics()
	if err != nil {
		return fmt.Errorf("failed to get topics: %w", err)
	}

	found := false
	for _, topic := range topics {
		if topic == topicName {
			found = true
			break
		}
	}

	if !found {
		return ErrTopicNotFound
	}

	// 构建配置更新请求
	configEntries := make(map[string]*string)
	for k, v := range configs {
		configEntries[k] = &v
	}

	err = admin.AlterConfig(sarama.TopicResource, topicName, configEntries, false)
	if err != nil {
		return fmt.Errorf("failed to update topic config: %w", err)
	}

	return nil
}

// GetTopicMetrics 获取 Topic 指标信息 - 单一数据源：Prometheus
func (s *KafkaService) GetTopicMetrics(ctx context.Context, topicName string) (*TopicMetrics, error) {
	// 从Prometheus获取指标
	promMetric, err := s.promService.GetKafkaTopicMetricsByName(ctx, topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metrics from Prometheus: %w", err)
	}

	return &TopicMetrics{
		TopicName:         promMetric.TopicName,
		TotalMessages:     promMetric.MessagesTotal,
		TotalSizeBytes:    promMetric.BytesTotal,
		MessagesPerSecond: promMetric.MessagesPerSec,
		BytesPerSecond:    promMetric.BytesPerSec,
		PartitionCount:    promMetric.PartitionCount,
		ReplicationFactor: 1, // 默认值，Prometheus通常不提供此信息
		RetentionMs:       604800000, // 7天默认
		LastUpdated:       promMetric.LastUpdated,
	}, nil
}

// GetTopicsOverview 获取所有 Topics 概览 - Phase 1 实现
func (s *KafkaService) GetTopicsOverview(ctx context.Context) (*TopicsOverviewResponse, error) {
	// 使用 Prometheus 获取 Topics 概览数据
	topicsData, err := s.promService.GetAllTopicsOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get topics overview from Prometheus: %w", err)
	}
	
	var topics []TopicOverviewItem
	var totalTopics, internalTopics int
	var totalSizeBytes int64
	
	for _, topic := range topicsData {
		topics = append(topics, TopicOverviewItem{
			Name:              topic.Name,
			PartitionCount:    topic.PartitionCount,
			ReplicationFactor: topic.ReplicationFactor,
			OutOfSyncReplicas: topic.OutOfSyncReplicas,
			MessagesTotal:     topic.MessagesTotal,
			SizeBytes:         topic.SizeBytes,
			SizeFormatted:     topic.SizeFormatted,
			MessagesPerSec:    topic.MessagesPerSec,
			BytesPerSec:       topic.BytesPerSec,
			IsInternal:        topic.IsInternal,
		})
		
		totalTopics++
		if topic.IsInternal {
			internalTopics++
		}
		totalSizeBytes += topic.SizeBytes
	}
	
	return &TopicsOverviewResponse{
		Topics: topics,
		Summary: TopicsSummary{
			TotalTopics:    totalTopics,
			InternalTopics: internalTopics,
			TotalSizeBytes: totalSizeBytes,
		},
	}, nil
}

// GetBrokersOverview 获取 Brokers 概览 - Phase 1 实现
func (s *KafkaService) GetBrokersOverview(ctx context.Context) (*BrokersOverviewResponse, error) {
	// 使用 Prometheus 获取 Broker 概览数据
	cluster, brokers, err := s.promService.GetBrokerOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get brokers overview from Prometheus: %w", err)
	}
	
	var brokerItems []BrokerOverviewItem
	for _, broker := range brokers {
		brokerItems = append(brokerItems, BrokerOverviewItem{
			ID:             broker.ID,
			Host:           broker.Host,
			Port:           broker.Port,
			IsController:   broker.IsController,
			PartitionCount: broker.PartitionCount,
			LeaderCount:    broker.LeaderCount,
			NetworkStats: NetworkStats{
				BytesInPerSec:  int64(broker.BytesInPerSec),
				BytesOutPerSec: int64(broker.BytesOutPerSec),
				MessagesInPerSec: int64(broker.MessagesPerSec),
			},
			JVMStats: JVMStats{
				HeapUsed: formatBytes(broker.JVMHeapUsed),
				HeapMax:  formatBytes(broker.JVMHeapMax),
			},
		})
	}
	
	return &BrokersOverviewResponse{
		Cluster: ClusterOverviewInfo{
			Version:                   cluster.Version,
			ControllerID:              cluster.ControllerID,
			BrokerCount:               cluster.BrokerCount,
			OnlineBrokers:             cluster.OnlineBrokers,
			TotalPartitions:           cluster.TotalPartitions,
			OnlinePartitions:          cluster.OnlinePartitions,
			UnderReplicatedPartitions: cluster.UnderReplicatedPartitions,
			PreferredReplicaImbalance: cluster.PreferredReplicaImbalance,
		},
		Brokers: brokerItems,
	}, nil
}

// GetTopicDetailsEnhanced 增强的 Topic 详情 - Phase 1 实现
func (s *KafkaService) GetTopicDetailsEnhanced(ctx context.Context, topicName string) (*TopicDetailsResponse, error) {
	// 1. 获取基础 Topic 信息 (使用现有方法)
	basicTopic, err := s.GetTopicDetails(ctx, topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic topic details: %w", err)
	}
	
	// 2. 获取分区大小信息
	partitionSizes, err := s.promService.GetTopicPartitionSizes(ctx, topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition sizes: %w", err)
	}
	
	// 3. 获取 Prometheus 指标来获取正确的 BytesPerSec
	promMetric, err := s.promService.GetKafkaTopicMetricsByName(ctx, topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metrics from Prometheus: %w", err)
	}
	
	// 4. 构建增强的分区信息
	var partitions []PartitionDetailInfo
	for partitionID, size := range partitionSizes {
		partitions = append(partitions, PartitionDetailInfo{
			PartitionID:    partitionID,
			Leader:         -1, // Phase 2 实现
			Replicas:       []int32{}, // Phase 2 实现
			InSyncReplicas: []int32{}, // Phase 2 实现
			SizeBytes:      size,
			SizeFormatted:  formatBytes(size),
			OffsetEarliest: -1, // Phase 2 实现
			OffsetLatest:   -1, // Phase 2 实现
			OffsetLag:      -1, // Phase 2 实现
		})
	}
	
	return &TopicDetailsResponse{
		Overview: TopicOverviewInfo{
			Name:              basicTopic.Name,
			PartitionCount:    basicTopic.PartitionCount,
			ReplicationFactor: basicTopic.ReplicationFactor,
			Config: map[string]string{
				"retention.ms": fmt.Sprintf("%d", basicTopic.RetentionMs),
				"cleanup.policy": basicTopic.CleanUpPolicy,
			},
		},
		Partitions: partitions,
		Metrics: TopicMetricsInfo{
			MessagesTotal:   basicTopic.TotalMessages,
			MessagesPerSec:  basicTopic.MessagesPerSecond,
			BytesPerSec:     promMetric.BytesPerSec, // 使用 Prometheus 的正确数据
		},
	}, nil
}

// 新增数据结构定义

// TopicsOverviewResponse Topics 概览响应
type TopicsOverviewResponse struct {
	Topics  []TopicOverviewItem `json:"topics"`
	Summary TopicsSummary       `json:"summary"`
}

// TopicOverviewItem Topic 概览项
type TopicOverviewItem struct {
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
}

// TopicsSummary Topics 汇总信息
type TopicsSummary struct {
	TotalTopics    int   `json:"total_topics"`
	InternalTopics int   `json:"internal_topics"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
}

// BrokersOverviewResponse Brokers 概览响应
type BrokersOverviewResponse struct {
	Cluster ClusterOverviewInfo   `json:"cluster"`
	Brokers []BrokerOverviewItem  `json:"brokers"`
}

// ClusterOverviewInfo 集群概览信息
type ClusterOverviewInfo struct {
	Version                   string `json:"version"`
	ControllerID              int32  `json:"controller_id"`
	BrokerCount               int    `json:"broker_count"`
	OnlineBrokers             int    `json:"online_brokers"`
	TotalPartitions           int    `json:"total_partitions"`
	OnlinePartitions          int    `json:"online_partitions"`
	UnderReplicatedPartitions int    `json:"under_replicated_partitions"`
	PreferredReplicaImbalance int    `json:"preferred_replica_imbalance"`
}

// BrokerOverviewItem Broker 概览项
type BrokerOverviewItem struct {
	ID             int32        `json:"id"`
	Host           string       `json:"host"`
	Port           int32        `json:"port"`
	IsController   bool         `json:"is_controller"`
	PartitionCount int          `json:"partition_count"`
	LeaderCount    int          `json:"leader_count"`
	NetworkStats   NetworkStats `json:"network_stats"`
	JVMStats       JVMStats     `json:"jvm_stats"`
}

// TopicDetailsResponse Topic 详情响应
type TopicDetailsResponse struct {
	Overview   TopicOverviewInfo     `json:"overview"`
	Partitions []PartitionDetailInfo `json:"partitions"`
	Metrics    TopicMetricsInfo      `json:"metrics"`
}

// TopicOverviewInfo Topic 概览信息
type TopicOverviewInfo struct {
	Name              string            `json:"name"`
	PartitionCount    int               `json:"partition_count"`
	ReplicationFactor int               `json:"replication_factor"`
	Config            map[string]string `json:"config"`
}

// PartitionDetailInfo 分区详细信息
type PartitionDetailInfo struct {
	PartitionID    int32   `json:"partition_id"`
	Leader         int32   `json:"leader"`
	Replicas       []int32 `json:"replicas"`
	InSyncReplicas []int32 `json:"in_sync_replicas"`
	SizeBytes      int64   `json:"size_bytes"`
	SizeFormatted  string  `json:"size_formatted"`
	OffsetEarliest int64   `json:"offset_earliest"`
	OffsetLatest   int64   `json:"offset_latest"`
	OffsetLag      int64   `json:"offset_lag"`
}

// TopicMetricsInfo Topic 指标信息
type TopicMetricsInfo struct {
	MessagesTotal  int64   `json:"messages_total"`
	MessagesPerSec float64 `json:"messages_per_sec"`
	BytesPerSec    float64 `json:"bytes_per_sec"`
}

// formatBytes 格式化字节数
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
