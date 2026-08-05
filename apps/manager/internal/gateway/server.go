package gateway

import (
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/dataplane/v1"
	"google.golang.org/grpc"
)

func RegisterAgentServices(server *grpc.Server, backend Backend) {
	dataplanev1.RegisterAgentDataPlaneServiceServer(server, NewDataServer(backend))
	controlplanev1.RegisterAgentControlPlaneServiceServer(server, NewControlServer(backend))
}
