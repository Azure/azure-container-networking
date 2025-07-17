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

// AssignIBDevicesToPod assigns InfiniBand devices to a pod via gRPC.
func (s *CNS) AssignIBDevicesToPod(_ context.Context, req *pb.AssignIBDevicesToPodRequest) (*pb.AssignIBDevicesToPodResponse, error) {
	s.Logger.Info("AssignIBDevicesToPod called",
		zap.String("podID", req.GetPodID()),
		zap.Strings("deviceIDs", req.GetDeviceIds()))

	// TODO: Implement the actual logic by calling the IBDeviceManager
	// For now, return a success response
	return &pb.AssignIBDevicesToPodResponse{
		ErrorCode: 0,
		Message:   "InfiniBand devices assigned successfully",
	}, nil
}

// GetIBDeviceInfo retrieves information about a specific InfiniBand device via gRPC.
func (s *CNS) GetIBDeviceInfo(_ context.Context, req *pb.GetIBDeviceInfoRequest) (*pb.GetIBDeviceInfoResponse, error) {
	s.Logger.Info("GetIBDeviceInfo called",
		zap.String("deviceID", req.GetDeviceID()))

	// TODO: Implement the actual logic by calling the IBDeviceManager
	// For now, return a placeholder response
	return &pb.GetIBDeviceInfoResponse{
		DeviceID:  req.GetDeviceID(),
		PodID:     "",
		Status:    "Available",
		ErrorCode: 0,
		Msg:       "Device information retrieved successfully",
	}, nil
}
