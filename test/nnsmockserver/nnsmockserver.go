package nnsmockserver

import (
	"context"
	"fmt"
	"github.com/Azure/azure-container-networking/log"
	nns "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"net"
	"strings"
)

type NnsMockServer struct {
	srv *grpc.Server
}

// node network service mock server implementation
type serverApi struct {
}

func (s *serverApi) ConfigureContainerNetworking(
	ctx context.Context,
	req *nns.ConfigureContainerNetworkingRequest) (*nns.ConfigureContainerNetworkingResponse, error) {

	if err := isValidPodName(req.ContainerId); err != nil {
		return nil, fmt.Errorf("NnsMockServer: RequestType:%s failed with error: %v", req.RequestType, err)
	}

    return &nns.ConfigureContainerNetworkingResponse{}, nil
}

func (s *serverApi) ConfigureNetworking(
	context.Context,
	*nns.ConfigureNetworkingRequest) (*nns.ConfigureNetworkingResponse, error) {
	return nil, nil
}

func (s *serverApi) PingNodeNetworkService(
	context.Context,
	*nns.PingNodeNetworkServiceRequest) (*nns.PingNodeNetworkServiceResponse, error) {
	return nil, nil
}

func NewNnsMockServer() *NnsMockServer {
	return &NnsMockServer{
		srv: grpc.NewServer(),
	}
}

func (s *NnsMockServer)StartGrpcServer(port string) {

	endpoint := fmt.Sprintf(":%s", port)
	lis, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Errorf("nnsmockserver: failed to listen at endpoint %s with error %v", endpoint, err)
	}

	nns.RegisterNodeNetworkServiceServer(s.srv, &serverApi{})
	if err := s.srv.Serve(lis); err != nil {
		log.Errorf("nnsmockserver: failed to serve at port %s with error %s", err)
	}
}

func (s *NnsMockServer)StopGrpcServer() {
	if s.srv == nil {
		fmt.Printf("s.srv is nil \n")
	}
	s.srv.Stop()
}

func isValidPodName(podName string) error {

	var splits []string
	splits = strings.Split(podName, "_")
	podNamelength := len(splits)
	if podNamelength != 2 {
		return fmt.Errorf(
			"Invalid PodName format. Should be in format podname_logicalContainerId. containerId should be uuid")
	}

	if _, err := uuid.Parse(splits[podNamelength-1]); err != nil {
		return fmt.Errorf(
			"Failed to parse container id. Should be in format podname_logicalContainerId. containerId should be uuid")
	}

	return nil
}