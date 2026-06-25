package agentplane

import (
	"errors"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

var ErrInvalidUpload = errors.New("invalid upload")

const (
	DataAckReasonAccepted             = "accepted"
	DataAckReasonDuplicate            = "duplicate"
	DataAckReasonInvalidUpload        = "invalid_data_batch"
	DataAckReasonRetryableServerError = "retryable_server_error"
	DataAckReasonServerError          = "server_error"
	DataAckReasonRetryable            = "retryable"
	DataAckReasonRejected             = "rejected"
	DataAckReasonUnspecified          = "unspecified"
)

type DataAppendResult struct {
	AcceptedEvents  int
	AcceptedSignals int
	CloudSignals    int
	Incidents       int
	Duplicate       bool
}

type Backend interface {
	AgentToken() string
	AppendDataBatchWithTransport(*dataplanev1.DataBatch, string) (DataAppendResult, error)
	BindAgentIdentity(store.AgentIdentity) error
	Store() ControlStore
	ResumeCursor(string, string) ResumeCursor
	TouchHotSession(store.AgentSession)
}

type ControlStore interface {
	AckResponse(responsemodel.Ack) (responsemodel.Command, bool)
	AddAgent(store.AgentIdentity)
	AttachIncidentEvidence(string, store.LabelSelector, *incidentv1.EvidenceSubgraph) (*incidentv1.Incident, bool)
	CompleteEvidencePullback(controlmodel.EvidencePullbackResult) (controlmodel.EvidencePullbackRequest, bool)
	AckControlCommand(controlmodel.ControlCommandAck) (controlmodel.ControlCommand, bool)
	EffectivePolicy(string, string, string, string) (policymodel.Policy, bool)
	GetEvidencePullback(string, string, string) (controlmodel.EvidencePullbackRequest, bool)
	MarkControlCommandSent(string, string, string, time.Time) (controlmodel.ControlCommand, bool)
	PendingControlCommands(string, string) []controlmodel.ControlCommand
	PendingEvidencePullbacks(string, string) []controlmodel.EvidencePullbackRequest
	PendingResponses(string, string) []responsemodel.Command
	RecordControlSessionOpen(string, string, string, time.Time) store.AgentSession
	Save() error
	UpsertAgentHealth(agenthealth.AgentHealth)
}
