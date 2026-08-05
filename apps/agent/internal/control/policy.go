package control

import "context"

type PolicySource string

const (
	PolicySourceStandalone PolicySource = "standalone"
	PolicySourceManaged    PolicySource = "managed"
)

type PolicyCommand struct {
	Context    RequestContext
	PolicyType string
	Document   string
	DryRun     bool
	Source     PolicySource
}

type PendingPolicy struct {
	PolicyID string
	Version  uint64
	Status   string
	Message  string
}

type PolicySnapshot struct {
	PolicyID string
	Version  uint64
	RawJSON  string
	Pending  *PendingPolicy
}

type PolicyController interface {
	ApplyPolicy(context.Context, PolicyCommand) Result
	CurrentPolicy(context.Context) (PolicySnapshot, error)
}
