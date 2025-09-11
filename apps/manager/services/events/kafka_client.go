package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"
)

// KafkaClient Kafka 客户端 (使用sarama库，与kafka_service保持一致)
type KafkaClient struct {
	brokers []string
	config  *sarama.Config
}

// EventMessage Kafka 事件消息结构 (匹配 Vector 输出格式)
type EventMessage struct {
	Timestamp       time.Time `json:"timestamp"`
	CollectorID     string    `json:"collector_id"`
	EventType       string    `json:"event_type"`
	ProcessedAt     time.Time `json:"processed_at"`
	
	// 主要消息内容
	Message         string    `json:"message"`           // Vector发送的主要消息内容
	Host            string    `json:"host,omitempty"`
	Source          string    `json:"source,omitempty"`
	Severity        string    `json:"severity,omitempty"`
	
	// Vector处理字段
	DataSource      string    `json:"data_source,omitempty"`
	EventCategory   string    `json:"event_category,omitempty"`
	PartitionKey    string    `json:"partition_key,omitempty"`
	TargetTopic     string    `json:"target_topic,omitempty"`
	SourceType      string    `json:"source_type,omitempty"`
	Port            int       `json:"port,omitempty"`
	
	// 原始数据字段（兼容性）
	Tags            []string  `json:"tags,omitempty"`
	
	// Kafka 元数据
	Partition       int       `json:"partition,omitempty"`
	Offset          int64     `json:"offset,omitempty"`
	Key             string    `json:"key,omitempty"`
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

// NewKafkaClient 创建 Kafka 客户端 (使用sarama)
func NewKafkaClient(brokers []string) *KafkaClient {
	config := sarama.NewConfig()
	config.Version = sarama.V3_4_0_0
	config.Consumer.Return.Errors = true
	config.Net.DialTimeout = 10 * time.Second
	config.Net.ReadTimeout = 10 * time.Second
	config.Net.WriteTimeout = 10 * time.Second
	
	return &KafkaClient{
		brokers: brokers,
		config:  config,
	}
}

// QueryEvents 查询事件（使用sarama，无状态读取）
func (k *KafkaClient) QueryEvents(ctx context.Context, params QueryParams) (*QueryResult, error) {
	// 设置默认值
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Limit > 1000 {
		params.Limit = 1000 // 最大限制
	}

	// 创建consumer
	consumer, err := sarama.NewConsumer(k.brokers, k.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}
	defer consumer.Close()

	// 获取分区列表
	partitions, err := consumer.Partitions(params.Topic)
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}

	var events []EventMessage
	
	// 简化实现：只读取第一个有数据的分区
	for _, partition := range partitions {
		partitionEvents, err := k.readPartitionEvents(consumer, params.Topic, partition, params)
		if err != nil {
			log.Printf("Error reading partition %d: %v", partition, err)
			continue
		}
		
		events = append(events, partitionEvents...)
		if len(events) >= params.Limit {
			break
		}
	}

	// 如果查询最新事件，需要反转顺序
	if params.Latest && len(events) > 1 {
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

// readPartitionEvents 读取单个分区的事件
func (k *KafkaClient) readPartitionEvents(consumer sarama.Consumer, topic string, partition int32, params QueryParams) ([]EventMessage, error) {
	// 创建client来获取offset信息
	client, err := sarama.NewClient(k.brokers, k.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// 获取分区的offset范围
	latestOffset, err := client.GetOffset(topic, partition, sarama.OffsetNewest)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest offset: %w", err)
	}

	earliestOffset, err := client.GetOffset(topic, partition, sarama.OffsetOldest)
	if err != nil {
		return nil, fmt.Errorf("failed to get earliest offset: %w", err)
	}

	// 如果分区为空，返回空结果
	if latestOffset <= earliestOffset {
		return []EventMessage{}, nil
	}

	// 确定起始offset
	var startOffset int64
	if params.Latest {
		// 从最新位置向前读取
		startOffset = latestOffset - int64(params.Limit)
		if startOffset < earliestOffset {
			startOffset = earliestOffset
		}
	} else {
		// 从最早位置开始读取
		startOffset = earliestOffset
	}

	// 创建分区consumer
	partitionConsumer, err := consumer.ConsumePartition(topic, partition, startOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to create partition consumer: %w", err)
	}
	defer partitionConsumer.Close()

	var events []EventMessage
	messageCount := 0
	timeout := time.After(3 * time.Second)

	for messageCount < params.Limit {
		select {
		case msg := <-partitionConsumer.Messages():
			if msg == nil {
				goto done
			}

			// 解析消息
			var event EventMessage
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				messageCount++
				continue
			}

			// 添加Kafka元数据
			event.Partition = int(msg.Partition)
			event.Offset = msg.Offset
			if msg.Key != nil {
				event.Key = string(msg.Key)
			}

			// 应用过滤条件
			if k.matchesFilter(event, params) {
				events = append(events, event)
			}

			messageCount++

		case <-timeout:
			goto done
		}
	}

done:
	return events, nil
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

// GetTopicInfo 获取 Topic 信息 (使用sarama)
func (k *KafkaClient) GetTopicInfo(ctx context.Context, topic string) (*TopicInfo, error) {
	client, err := sarama.NewClient(k.brokers, k.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// 获取分区信息
	partitions, err := client.Partitions(topic)
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}

	info := &TopicInfo{
		Topic:      topic,
		Partitions: len(partitions),
		QueriedAt:  time.Now(),
	}

	// 获取每个分区的详细信息
	for _, partitionID := range partitions {
		latestOffset, err := client.GetOffset(topic, partitionID, sarama.OffsetNewest)
		if err != nil {
			continue
		}

		earliestOffset, err := client.GetOffset(topic, partitionID, sarama.OffsetOldest)
		if err != nil {
			continue
		}

		// 获取分区leader信息
		leader, err := client.Leader(topic, partitionID)
		if err != nil {
			continue
		}

		partInfo := PartitionInfo{
			ID:          int(partitionID),
			Leader:      leader.Addr(),
			Replicas:    1, // 简化，实际应该查询replica信息
			FirstOffset: earliestOffset,
			LastOffset:  latestOffset,
		}

		info.PartitionInfo = append(info.PartitionInfo, partInfo)
	}

	return info, nil
}

// ListTopics 列出所有 Topic (使用sarama)
func (k *KafkaClient) ListTopics(ctx context.Context) ([]string, error) {
	client, err := sarama.NewClient(k.brokers, k.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	topics, err := client.Topics()
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %w", err)
	}

	return topics, nil
}
