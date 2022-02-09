package transport

import (
	"context"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// EventsClient is a client for the DataplaneEvents service
type EventsClient struct {
	ctx context.Context

	protos.DataplaneEventsClient
	pod        string
	node       string
	serverAddr string

	outCh chan *protos.Events
}

var (
	ErrPodNodeNameNil = fmt.Errorf("pod and node name must be set")
	ErrAddressNil     = fmt.Errorf("address must be set")
)

func NewEventsClient(ctx context.Context, pod, node, addr string) (*EventsClient, error) {
	if pod == "" || node == "" {
		return nil, ErrPodNodeNameNil
	}

	if addr == "" {
		return nil, ErrAddressNil
	}

	klog.Infof("Connecting to NPM controller gRPC server at address %s\n", addr)
	// TODO Make this secure
	// TODO Remove WithBlock option post testing
	cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	return &EventsClient{
		ctx:                   ctx,
		DataplaneEventsClient: protos.NewDataplaneEventsClient(cc),
		pod:                   pod,
		node:                  node,
		serverAddr:            addr,
		outCh:                 make(chan *protos.Events),
	}, nil
}

func (c *EventsClient) EventsChannel() chan *protos.Events {
	return c.outCh
}

func (c *EventsClient) Start(stopCh <-chan struct{}) error {
	clientMetadata := &protos.DatapathPodMetadata{
		PodName:  c.pod,
		NodeName: c.node,
	}

	opts := []grpc.CallOption{}
	connectClient, err := c.Connect(c.ctx, clientMetadata, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to dataplane events server: %w", err)
	}

	go c.run(c.ctx, connectClient, stopCh) //nolint:errcheck // ignore error since this is a go routine

	return nil
}

func (c *EventsClient) run(ctx context.Context, connectClient protos.DataplaneEvents_ConnectClient, stopCh <-chan struct{}) error {
	for {
		select {
		case <-ctx.Done():
			klog.Errorf("recevied done event on context channel: %v", ctx.Err())
			return fmt.Errorf("recevied done event on context channel: %w", ctx.Err())
		case <-stopCh:
			klog.Info("Received message on stop channel. Stopping transport client")
			return nil
		default:
			event, err := connectClient.Recv()
			if err != nil {
				klog.Errorf("failed to receive event: %v", err)
				return fmt.Errorf("failed to receive event: %w", err)
			}

			c.outCh <- event
		}
	}
}
