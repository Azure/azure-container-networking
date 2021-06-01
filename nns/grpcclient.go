package nns

import (
	"context"
	"fmt"
	"github.com/Azure/azure-container-networking/log"
	contracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"google.golang.org/grpc"
	"time"
)

const (
	nnsPort           = "6668"          // port where node network service listens at
	apiTimeout        = 5 * time.Minute // recommended timeout from node service
	connectionTimeout = 2 * time.Minute
)

// client to invoke Node network service APIs using grpc
type GrpcClient struct {
}

// Add container to the network. Container Id is appended to the podName
func (c *GrpcClient) AddContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (error, *contracts.ConfigureContainerNetworkingResponse) {

	return configureContainerNetworking(ctx, contracts.RequestType_Setup, podName, nwNamespace)
}

// Add container to the network. Container Id is appended to the podName
func (c *GrpcClient) DeleteContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (error, *contracts.ConfigureContainerNetworkingResponse) {

	return configureContainerNetworking(ctx, contracts.RequestType_Teardown, podName, nwNamespace)
}

// create a grpc connection to the node network service (nns) and call the appropriate
// API in nns based on RequestType parameter using the grpc connection
func configureContainerNetworking(
	ctx context.Context,
	reqtype contracts.RequestType,
	podName, nwNamespace string) (error, *contracts.ConfigureContainerNetworkingResponse) {

	// create a client. This also establishes grpc connection with nns
	client, conn, err := newGrpcClient(ctx)
	if err != nil {
		return err, nil
	}

	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("[baremetal]grpc connection close for container %s failed with : %s", podName, err)
		}
	}()

	// create request parameter for nns
	req := &contracts.ConfigureContainerNetworkingRequest{
		RequestType:        reqtype,
		ContainerId:        podName,
		NetworkNamespaceId: nwNamespace,
	}

	localCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	// Invoke nns API via grpc client
	res, err := client.ConfigureContainerNetworking(localCtx, req)
	if err != nil {
		log.Printf("[baremetal] ConfigureContainerNetworking for container %s for nw namespace %s failed with error: %s \n",
			podName, nwNamespace, err)
	} else {
		log.Printf("[baremetal] ConfigureContainerNetworking for container %s for nw namespace %s succeeded with result: %v \n",
			podName, nwNamespace, res)
	}

	return err, res
}

// create a connection to the node network service listening at localhost:6678
// and return nns client that encapsulates that connection
func newGrpcClient(ctx context.Context) (contracts.NodeNetworkServiceClient, *grpc.ClientConn, error) {

	localCtx, cancel := context.WithTimeout(ctx, connectionTimeout)
	defer cancel()

	nnsEndpoint := fmt.Sprintf(":%s", nnsPort)
	log.Printf("[baremetal] Creating grpc connection to nns at endpoint :%s", nnsEndpoint)
	conn, err := grpc.DialContext(
		localCtx, nnsEndpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, fmt.Errorf("connection to nns failed at endpoint %s with error: %s", nnsEndpoint, err)
	}

	return contracts.NewNodeNetworkServiceClient(conn), conn, nil
}
