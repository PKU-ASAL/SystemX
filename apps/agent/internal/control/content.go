package control

import (
	"context"

	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
)

type ContentCommand struct {
	Context       RequestContext
	Document      string
	DryRun        bool
	AllowUnsigned bool
	Source        PolicySource
}

type ContentController interface {
	ApplyContent(context.Context, ContentCommand) Result
	ListContent(context.Context, string) ([]agentcontent.Record, error)
	GetContent(context.Context, string) (agentcontent.Record, bool, error)
}
