package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agentplane"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	platformredis "github.com/sysarmor/sysarmor-next-project/internal/platform/redis"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"google.golang.org/protobuf/encoding/protojson"
)

type DataAppendResult = agentplane.DataAppendResult
type ResumeCursor = agentplane.ResumeCursor

type Store interface {
	agentplane.ControlStore
	BindAgentIdentity(store.AgentIdentity) error
	ListAgentSessions(string, string) []store.AgentSession
	RecordDataBatchAppend(store.AgentIdentity, string, string, time.Time) store.AgentSession
}

type Runtime struct {
	store          Store
	producer       platformkafka.Producer
	hotState       platformredis.HotState
	localProcessor *ingestworker.Processor
	agentToken     string
	owner          string
}

type RuntimeOptions struct {
	Store          Store
	Producer       platformkafka.Producer
	HotState       platformredis.HotState
	LocalProcessor *ingestworker.Processor
	AgentToken     string
	Owner          string
}

func NewRuntime(opts RuntimeOptions) *Runtime {
	producer := opts.Producer
	if producer == nil {
		producer = platformkafka.NoopProducer{}
	}
	hotState := opts.HotState
	if hotState == nil {
		hotState = platformredis.NoopHotState{}
	}
	owner := opts.Owner
	if owner == "" {
		owner = "sysarmor-gateway"
	}
	return &Runtime{
		store:          opts.Store,
		producer:       producer,
		hotState:       hotState,
		localProcessor: opts.LocalProcessor,
		agentToken:     opts.AgentToken,
		owner:          owner,
	}
}

func (r *Runtime) AgentToken() string {
	return r.agentToken
}

func (r *Runtime) Store() agentplane.ControlStore {
	return r.store
}

func (r *Runtime) BindAgentIdentity(agent store.AgentIdentity) error {
	return r.store.BindAgentIdentity(agent)
}

func (r *Runtime) TouchHotSession(session store.AgentSession) {
	if session.AgentID == "" {
		return
	}
	_ = r.hotState.TouchAgentSession(context.Background(), platformredis.AgentSession{
		TenantID:          session.TenantID,
		AgentID:           session.AgentID,
		Owner:             r.owner,
		LastSeenAt:        session.LastSeenAt,
		LastDataSeenAt:    session.LastDataSeenAt,
		LastControlSeenAt: session.LastControlSeenAt,
		LastAckCursor:     session.LastAckCursor,
		DataTransport:     session.DataTransport,
		ControlTransport:  session.ControlTransport,
	})
}

func (r *Runtime) ResumeCursor(tenantID, agentID string) agentplane.ResumeCursor {
	resume := agentplane.ResumeCursor{TenantID: tenantID, AgentID: agentID}
	sessions := r.store.ListAgentSessions(tenantID, agentID)
	if len(sessions) > 0 {
		resume.SessionID = sessions[0].SessionID
		resume.ResumeCursor = sessions[0].LastAckCursor
	}
	return resume
}

func (r *Runtime) AppendDataBatchWithTransport(batch *dataplanev1.DataBatch, transport string) (agentplane.DataAppendResult, error) {
	if err := validateUploadIdentity(batch); err != nil {
		return agentplane.DataAppendResult{}, err
	}
	header := batch.GetHeader()
	if r.isDuplicateBatch(header.GetTenantId(), header.GetAgentId(), header.GetBatchId()) {
		session := r.store.RecordDataBatchAppend(store.AgentIdentityFromDataBatch(batch), header.GetBatchId(), transport, time.Now().UTC())
		r.TouchHotSession(session)
		return agentplane.DataAppendResult{Duplicate: true}, r.store.Save()
	}
	if err := r.appendRawBatch(batch); err != nil {
		return agentplane.DataAppendResult{}, err
	}
	session := r.store.RecordDataBatchAppend(store.AgentIdentityFromDataBatch(batch), header.GetBatchId(), transport, time.Now().UTC())
	r.TouchHotSession(session)
	if err := r.store.Save(); err != nil {
		return agentplane.DataAppendResult{}, err
	}
	return r.processLocal(batch)
}

func (r *Runtime) appendRawBatch(batch *dataplanev1.DataBatch) error {
	raw, err := protojson.Marshal(batch)
	if err != nil {
		return fmt.Errorf("encode raw data batch: %w", err)
	}
	header := batch.GetHeader()
	key := strings.Join([]string{header.GetTenantId(), header.GetAgentId(), header.GetBatchId()}, ":")
	return r.producer.Append(context.Background(), platformkafka.Message{Topic: "sysarmor.agent.databatch.raw", Key: key, Value: raw})
}

func (r *Runtime) processLocal(batch *dataplanev1.DataBatch) (agentplane.DataAppendResult, error) {
	if r.localProcessor == nil {
		return agentplane.DataAppendResult{}, nil
	}
	result, err := r.localProcessor.Process(context.Background(), batch)
	if err != nil {
		return agentplane.DataAppendResult{}, err
	}
	return agentplane.DataAppendResult{
		AcceptedEvents:  result.AcceptedEvents,
		AcceptedSignals: result.AcceptedSignals,
		CloudSignals:    result.CloudSignals,
		Incidents:       result.Incidents,
	}, nil
}

func (r *Runtime) isDuplicateBatch(tenantID, agentID, batchID string) bool {
	if batchID == "" {
		return false
	}
	for _, session := range r.store.ListAgentSessions(tenantID, agentID) {
		if session.LastAckCursor == batchID {
			return true
		}
	}
	return false
}

func validateUploadIdentity(batch *dataplanev1.DataBatch) error {
	if batch == nil || batch.GetHeader() == nil {
		return fmt.Errorf("%w: batch header identity is required", agentplane.ErrInvalidUpload)
	}
	header := batch.GetHeader()
	missing := []string{}
	if header.GetAgentId() == "" {
		missing = append(missing, "agent_id")
	}
	if header.GetHostId() == "" {
		missing = append(missing, "host_id")
	}
	if header.GetTenantId() == "" {
		missing = append(missing, "tenant_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: agent identity missing %s", agentplane.ErrInvalidUpload, strings.Join(missing, ", "))
	}
	return nil
}
