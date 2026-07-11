package kafka

import (
	"context"
	"errors"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

var ErrDisabled = errors.New("kafka ingest is disabled")

type Message struct {
	Topic     string
	Partition int
	Offset    int64
	Key       string
	Value     []byte

	raw kafkago.Message
}

type Producer interface {
	Append(context.Context, Message) error
}

type Consumer interface {
	Fetch(context.Context) (Message, error)
	Commit(context.Context, Message) error
	Close() error
}

type DisabledProducer struct{}

func (DisabledProducer) Append(context.Context, Message) error {
	return ErrDisabled
}

type NoopProducer struct{}

func (NoopProducer) Append(context.Context, Message) error {
	return nil
}

type WriterProducer struct {
	writer *kafkago.Writer
}

func NewWriterProducer(brokers []string) (*WriterProducer, error) {
	clean := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			clean = append(clean, broker)
		}
	}
	if len(clean) == 0 {
		return nil, ErrDisabled
	}
	return &WriterProducer{writer: &kafkago.Writer{
		Addr:         kafkago.TCP(clean...),
		Balancer:     &kafkago.Hash{},
		RequiredAcks: kafkago.RequireAll,
		Async:        false,
	}}, nil
}

func (p *WriterProducer) Append(ctx context.Context, msg Message) error {
	if p == nil || p.writer == nil {
		return ErrDisabled
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic: msg.Topic,
		Key:   []byte(msg.Key),
		Value: msg.Value,
	})
}

func (p *WriterProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

type ReaderConsumer struct {
	reader *kafkago.Reader
}

func NewReaderConsumer(brokers []string, topic string, groupID string) (*ReaderConsumer, error) {
	clean := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			clean = append(clean, broker)
		}
	}
	topic = strings.TrimSpace(topic)
	groupID = strings.TrimSpace(groupID)
	if len(clean) == 0 || topic == "" || groupID == "" {
		return nil, ErrDisabled
	}
	if err := ensureTopic(clean, topic); err != nil {
		return nil, err
	}
	return &ReaderConsumer{reader: kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: clean,
		Topic:   topic,
		GroupID: groupID,
	})}, nil
}

func ensureTopic(brokers []string, topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var lastErr error
	for _, broker := range brokers {
		conn, err := kafkago.DialContext(ctx, "tcp", broker)
		if err != nil {
			lastErr = err
			continue
		}
		err = conn.CreateTopics(kafkago.TopicConfig{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
		_ = conn.Close()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func (c *ReaderConsumer) Fetch(ctx context.Context) (Message, error) {
	if c == nil || c.reader == nil {
		return Message{}, ErrDisabled
	}
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return Message{}, err
	}
	return Message{Topic: msg.Topic, Partition: msg.Partition, Offset: msg.Offset, Key: string(msg.Key), Value: msg.Value, raw: msg}, nil
}

func (c *ReaderConsumer) Commit(ctx context.Context, msg Message) error {
	if c == nil || c.reader == nil {
		return ErrDisabled
	}
	return c.reader.CommitMessages(ctx, msg.raw)
}

func (c *ReaderConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
