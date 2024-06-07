package grpc

import (
	"context"

	pb "github.com/Azure/azure-container-networking/cns/grpc/v1alpha"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"go.uber.org/zap"
)

// CNSService defines the CNS gRPC service.
type CNS struct {
	pb.UnimplementedCNSServer
	Logger *zap.Logger
	State  *restserver.HTTPRestService
}

func (s *CNS) SetOrchestratorInfo(_ context.Context, req *pb.SetOrchestratorInfoRequest) (*pb.SetOrchestratorInfoResponse, error) {
	s.Logger.Info("SetOrchestratorInfo called", zap.String("nodeID", req.GetNodeID()), zap.String("orchestratorType", req.GetOrchestratorType()))
	// todo: Implement the logic
	return &pb.SetOrchestratorInfoResponse{}, nil
}

func (s *CNS) GetNodeInfo(_ context.Context, req *pb.NodeInfoRequest) (*pb.NodeInfoResponse, error) {
	s.Logger.Info("GetNodeInfo called", zap.String("nodeID", req.GetNodeID()))
	// todo: Implement the logic
	return &pb.NodeInfoResponse{}, nil
}
