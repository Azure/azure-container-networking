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

	fmt.Printf("Received request of type :%s \n", req.RequestType)
	if err := isValidPodName(req.ContainerId); err != nil {
		return nil, fmt.Errorf("NnsMockServer: RequestType:%s failed with error: %v", req.RequestType, err)
	}

	ipaddress := &nns.ContainerIPAddress{
		Ip:             "10.91.149.1",
		DefaultGateway: "10.91.148.1",
		PrefixLength:   "24",
		Version:        "4",
	}

	contTnterface := &nns.ContainerNetworkInterface{
		Name:               "azurevnet_45830dd4-1778-4735-9173-bba59b74cc8b_4ab80fb9-147e-4461-a213-56f4d44e806f",
		NetworkNamespaceId: req.NetworkNamespaceId,
		Ipaddresses:        []*nns.ContainerIPAddress{ipaddress},
		MacAddress:         "0036578BB0F1",
	}

	res := nns.ConfigureContainerNetworkingResponse{
		Interfaces: []*nns.ContainerNetworkInterface{contTnterface},
	}

	return &res, nil
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

func (s *NnsMockServer) StartGrpcServer(port string) {

	endpoint := fmt.Sprintf(":%s", port)
	lis, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Errorf("nnsmockserver: failed to listen at endpoint %s with error %v", endpoint, err)
	}

	nns.RegisterNodeNetworkServiceServer(s.srv, &serverApi{})
	if err := s.srv.Serve(lis); err != nil {
		log.Errorf("nnsmockserver: failed to serve at port %s with error %s", port, err)
	}
}

func (s *NnsMockServer) StopGrpcServer() {
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
