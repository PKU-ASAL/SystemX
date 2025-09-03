package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaClient Kafka 客户端
type KafkaClient struct {
	brokers []string
	reader  *kafka.Reader
}

// EventMessage Kafka 事件消息结构 (匹配 Vector 输出格式)
type EventMessage struct {
	Timestamp       time.Time `json:"timestamp"`
	CollectorID     string    `json:"collector_id"`
	EventType       string    `json:"event_type"`
	KafkaTopic      string    `json:"kafka_topic"`
	OriginalMessage string    `json:"original_message"`
	ProcessedAt     time.Time `json:"processed_at"`
	
	// Vector 字段
	AppName        string `json:"appname,omitempty"`
	Facility       string `json:"facility,omitempty"`
	FacilityLabel  string `json:"facility_label,omitempty"`
	Host           string `json:"host,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	LogLevel       string `json:"log_level,omitempty"`
	Message        string `json:"message,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeverityLabel  string `json:"severity_label,omitempty"`
	SourceHost     string `json:"source_host,omitempty"`
	SourceIP       string `json:"source_ip,omitempty"`
	SourceType     string `json:"source_type,omitempty"`
	
	// Kafka 元数据
	Partition int    `json:"partition,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	Key       string `json:"key,omitempty"`
}

// QueryParams 查询参数
type QueryParams struct {
	Topic       string    `json:"topic"`
	CollectorID string    `json:"collector_id,omitempty"`
	EventType   string    `json:"event_type,omitempty"`
	Limit       int       `json:"limit"`
	FromTime    time.Time `json:"from_time,omitempty"`
	ToTime      time.Time `json:"to_time,omitempty"`
	Latest      bool      `json:"latest"`
}

// QueryResult 查询结果
type QueryResult struct {
	Events      []EventMessage `json:"events"`
	Total       int            `json:"total"`
	Topic       string         `json:"topic"`
	QueriedAt   time.Time      `json:"queried_at"`
	QueryParams QueryParams    `json:"query_params"`
}

// NewKafkaClient 创建 Kafka 客户端
func NewKafkaClient(brokers []string) *KafkaClient {
	return &KafkaClient{
		brokers: brokers,
	}
}

// QueryEvents 查询事件
func (k *KafkaClient) QueryEvents(ctx context.Context, params QueryParams) (*QueryResult, error) {
	// 设置默认值
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Limit > 1000 {
		params.Limit = 1000 // 最大限制
	}

	// 使用消费者组模式读取所有分区
	readerConfig := kafka.ReaderConfig{
		Brokers:     k.brokers,
		Topic:       params.Topic,
		GroupID:     fmt.Sprintf("sysarmor-manager-query-%d", time.Now().UnixNano()),
		StartOffset: kafka.FirstOffset,
	}

	reader := kafka.NewReader(readerConfig)
	defer reader.Close()

	var events []EventMessage
	readCount := 0
	maxReads := params.Limit * 2 // 读取更多消息以便过滤

	for readCount < maxReads {
		// 设置读取超时
		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		message, err := reader.ReadMessage(readCtx)
		cancel()

		if err != nil {
			if err == context.DeadlineExceeded {
				break // 超时，停止读取
			}
			log.Printf("Error reading message: %v", err)
			break
		}

		// 解析消息
		var event EventMessage
		if err := json.Unmarshal(message.Value, &event); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		// 添加 Kafka 元数据
		event.Partition = message.Partition
		event.Offset = message.Offset
		event.Key = string(message.Key)

		// 应用过滤条件
		if k.matchesFilter(event, params) {
			events = append(events, event)
			if len(events) >= params.Limit {
				break
			}
		}

		readCount++
	}

	// 如果查询最新事件，需要反转顺序
	if params.Latest {
		k.reverseEvents(events)
	}

	result := &QueryResult{
		Events:      events,
		Total:       len(events),
		Topic:       params.Topic,
		QueriedAt:   time.Now(),
		QueryParams: params,
	}

	return result, nil
}

// matchesFilter 检查事件是否匹配过滤条件
func (k *KafkaClient) matchesFilter(event EventMessage, params QueryParams) bool {
	// CollectorID 过滤
	if params.CollectorID != "" && event.CollectorID != params.CollectorID {
		return false
	}

	// EventType 过滤
	if params.EventType != "" && event.EventType != params.EventType {
		return false
	}

	// 时间范围过滤
	if !params.FromTime.IsZero() && event.Timestamp.Before(params.FromTime) {
		return false
	}

	if !params.ToTime.IsZero() && event.Timestamp.After(params.ToTime) {
		return false
	}

	return true
}

// reverseEvents 反转事件顺序
func (k *KafkaClient) reverseEvents(events []EventMessage) {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
}

// GetTopicInfo 获取 Topic 信息
func (k *KafkaClient) GetTopicInfo(ctx context.Context, topic string) (*TopicInfo, error) {
	conn, err := kafka.DialContext(ctx, "tcp", k.brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions: %w", err)
	}

	info := &TopicInfo{
		Topic:      topic,
		Partitions: len(partitions),
		QueriedAt:  time.Now(),
	}

	// 获取每个分区的信息
	for _, partition := range partitions {
		partInfo := PartitionInfo{
			ID:       partition.ID,
			Leader:   partition.Leader.Host,
			Replicas: len(partition.Replicas),
		}

		// 获取分区的偏移量信息
		if reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   k.brokers,
			Topic:     topic,
			Partition: partition.ID,
		}); reader != nil {
			// 获取最早和最新的偏移量
			firstOffset, lastOffset := reader.Stats().Offset, reader.Stats().Lag
			partInfo.FirstOffset = firstOffset
			partInfo.LastOffset = lastOffset
			reader.Close()
		}

		info.PartitionInfo = append(info.PartitionInfo, partInfo)
	}

	return info, nil
}

// TopicInfo Topic 信息
type TopicInfo struct {
	Topic         string          `json:"topic"`
	Partitions    int             `json:"partitions"`
	PartitionInfo []PartitionInfo `json:"partition_info"`
	QueriedAt     time.Time       `json:"queried_at"`
}

// PartitionInfo 分区信息
type PartitionInfo struct {
	ID          int    `json:"id"`
	Leader      string `json:"leader"`
	Replicas    int    `json:"replicas"`
	FirstOffset int64  `json:"first_offset"`
	LastOffset  int64  `json:"last_offset"`
}

// ListTopics 列出所有 Topic
func (k *KafkaClient) ListTopics(ctx context.Context) ([]string, error) {
	conn, err := kafka.DialContext(ctx, "tcp", k.brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions: %w", err)
	}

	topicSet := make(map[string]bool)
	for _, partition := range partitions {
		topicSet[partition.Topic] = true
	}

	var topics []string
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	return topics, nil
}
