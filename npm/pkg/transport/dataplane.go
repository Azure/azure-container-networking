package transport

import (
	"context"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc"
)

// DataplaneEventsClient is a client for the DataplaneEvents service
type DataplaneEventsClient struct {
	protos.DataplaneEventsClient
	pod        string
	node       string
	serverAddr string
}

func NewDataplaneEventsClient(ctx context.Context, pod, node, addr string) (*DataplaneEventsClient, error) {
	cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	return &DataplaneEventsClient{
		DataplaneEventsClient: protos.NewDataplaneEventsClient(cc),
		pod:                   pod,
		node:                  node,
		serverAddr:            addr,
	}, nil
}

func (c *DataplaneEventsClient) Start(ctx context.Context) error {
	clientMetadata := &protos.DatapathPodMetadata{
		PodName:  c.pod,
		NodeName: c.node,
	}

	opts := []grpc.CallOption{}
	connectClient, err := c.Connect(ctx, clientMetadata, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to dataplane events server: %w", err)
	}

	return c.run(ctx, connectClient)
}

func (c *DataplaneEventsClient) run(ctx context.Context, connectClient protos.DataplaneEvents_ConnectClient) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		default:
			event, err := connectClient.Recv()
			if err != nil {
				break
			}

			// TODO: REMOVE ME
			// This is for debugging purposes only
			fmt.Printf(
				"Received event type %s object type %s: \n",
				event.GetType(),
				event.GetObject(),
			)

			for _, e := range event.GetEvent() {
				for _, d := range e.GetData() {
					eventAsMap := d.AsMap()
					fmt.Printf("%s: %s\n", eventAsMap["Type"], eventAsMap["Payload"])
				}
			}
		}
	}
}
