package gateway

import (
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agentplane"
	"google.golang.org/grpc"
)

func RegisterAgentServices(server *grpc.Server, backend agentplane.Backend) {
	dataplanev1.RegisterAgentDataPlaneServiceServer(server, agentplane.NewDataServer(backend))
	controlplanev1.RegisterAgentControlPlaneServiceServer(server, agentplane.NewControlServer(backend))
}
