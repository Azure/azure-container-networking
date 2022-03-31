package transport

import (
	"context"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	config, err := clientTLSConfig()
	if err != nil {
		klog.Errorf("failed to load client tls config : %s", err)
		return nil, fmt.Errorf("failed to load client tls config : %w", err)
	}

	cc, err := grpc.DialContext(
		ctx,
		addr,
		grpc.WithTransportCredentials(credentials.NewTLS(config)),
	)
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
	go c.run(c.ctx, stopCh) //nolint:errcheck // ignore error since this is a go routine
	return nil
}

func (c *EventsClient) run(ctx context.Context, stopCh <-chan struct{}) error {
	var connectClient protos.DataplaneEvents_ConnectClient
	var err error
	clientMetadata := &protos.DatapathPodMetadata{
		PodName:  c.pod,
		NodeName: c.node,
	}
	for {
		select {
		case <-ctx.Done():
			klog.Errorf("recevied done event on context channel: %v", ctx.Err())
			return fmt.Errorf("recevied done event on context channel: %w", ctx.Err())
		case <-stopCh:
			klog.Info("Received message on stop channel. Stopping transport client")
			return nil
		default:
			if connectClient == nil {
				klog.Info("Reconnecting to gRPC server controller")
				opts := []grpc.CallOption{grpc.WaitForReady(false)}
				connectClient, err = c.Connect(ctx, clientMetadata, opts...)
				if err != nil {
					return fmt.Errorf("failed to connect to dataplane events server: %w", err)
				}
				klog.Info("Successfully connected to gRPC server controller")
			}
			event, err := connectClient.Recv()
			if err != nil {
				klog.Errorf("failed to receive event: %v", err)
				connectClient = nil
				continue
			}
			klog.Infof("### Received event: %v", event)
			c.outCh <- event
		}
	}
}
