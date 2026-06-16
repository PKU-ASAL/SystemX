package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var ErrDisabled = errors.New("redis hot state is disabled")

type AgentSession struct {
	TenantID      string
	AgentID       string
	Owner         string
	LastSeenAt    time.Time
	LastAckCursor string
}

type HotState interface {
	TouchAgentSession(context.Context, AgentSession) error
}

type DisabledHotState struct{}

func (DisabledHotState) TouchAgentSession(context.Context, AgentSession) error {
	return ErrDisabled
}

type NoopHotState struct{}

func (NoopHotState) TouchAgentSession(context.Context, AgentSession) error {
	return nil
}

type ClientHotState struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewClientHotState(addr string, ttl time.Duration) (*ClientHotState, error) {
	if addr == "" {
		return nil, ErrDisabled
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &ClientHotState{
		client: goredis.NewClient(&goredis.Options{Addr: addr}),
		ttl:    ttl,
	}, nil
}

func (s *ClientHotState) TouchAgentSession(ctx context.Context, session AgentSession) error {
	if s == nil || s.client == nil {
		return ErrDisabled
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	key := "sysarmor:agent_gateway:session:" + session.TenantID + ":" + session.AgentID
	return s.client.Set(ctx, key, raw, s.ttl).Err()
}

func (s *ClientHotState) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}
