package control

import (
	"context"

	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
)

type ResponseController interface {
	ExecuteResponse(context.Context, responsemodel.Command) responsemodel.Ack
	CollectEvidence(context.Context, controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackResult
}
