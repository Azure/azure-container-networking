package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/klog/v2"
)

type Manager struct {
	ctx context.Context

	// Server is the gRPC server
	Server protos.DataplaneEventsServer

	// Watchdog is the watchdog for the gRPC server that implements the
	// gRPC stats handler interface
	Watchdog stats.Handler

	// Registrations is a map of dataplane pod address to their associate connection stream
	Registrations map[string]clientStreamConnection

	// port is the port the manager is listening on
	port int

	// inCh is the input channel for the manager
	inCh chan interface{}

	// regCh is the registration channel
	regCh chan clientStreamConnection

	// deregCh is the deregistration channel
	deregCh chan deregistrationEvent

	// errCh is the error channel
	errCh chan error
}

// New creates a new transport manager
func NewManager(ctx context.Context, port int) *Manager {
	// Create a registration channel
	regCh := make(chan clientStreamConnection, grpcMaxConcurrentStreams)

	// Create a deregistration channel
	deregCh := make(chan deregistrationEvent, grpcMaxConcurrentStreams)

	return &Manager{
		ctx:           ctx,
		Server:        NewServer(ctx, regCh),
		Watchdog:      NewWatchdog(deregCh),
		Registrations: make(map[string]clientStreamConnection),
		port:          port,
		inCh:          make(chan interface{}),
		errCh:         make(chan error),
		deregCh:       deregCh,
		regCh:         regCh,
	}
}

// InputChannel returns the input channel for the manager
func (m *Manager) InputChannel() chan interface{} {
	return m.inCh
}

func (m *Manager) Start() error {
	klog.Info("Starting transport manager")
	if err := m.start(); err != nil {
		klog.Errorf("Failed to Start transport manager: %v", err)
		return err
	}
	return nil
}

func (m *Manager) start() error {
	if err := m.handle(); err != nil {
		return fmt.Errorf("failed to start transport manager handlers: %w", err)
	}

	for {
		select {
		case client := <-m.regCh:
			klog.Infof("Registering remote client %s", client)
			m.Registrations[client.String()] = client
		case ev := <-m.deregCh:
			klog.Infof("Degregistering remote client %s", ev.remoteAddr)
			if v, ok := m.Registrations[ev.remoteAddr]; ok {
				if v.timestamp <= ev.timestamp {
					delete(m.Registrations, ev.remoteAddr)
				} else {
					klog.Info("Ignoring stale deregistration event")
				}
			}
		case msg := <-m.inCh:
			for _, client := range m.Registrations {
				if err := client.stream.SendMsg(&protos.Events{
					Type:   *protos.Events_APPLY.Enum(),
					Object: *protos.Events_IPSET.Enum(),
					Event: []*protos.Event{
						{
							Data: []*structpb.Struct{
								msg.(*structpb.Struct),
							},
						},
					},
				}); err != nil {
					klog.Errorf("Failed to send message to client %s: %v", client, err)
				}
			}
		case <-m.ctx.Done():
			klog.Info("Stopping transport manager")
			return nil
		case err := <-m.errCh:
			klog.Errorf("Error in transport manager: %v", err)
			return err
		}
	}
}

func (m *Manager) handle() error {
	klog.Info("Starting transport manager listener")
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", m.port))
	if err != nil {
		return fmt.Errorf("failed to handle server connections: %w", err)
	}

	var opts []grpc.ServerOption = []grpc.ServerOption{
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.StatsHandler(m.Watchdog),
	}

	server := grpc.NewServer(opts...)
	protos.RegisterDataplaneEventsServer(
		server,
		m.Server,
	)

	// Register reflection service on gRPC server.
	// This is useful for debugging and testing with grpcurl and other CLI tools.
	reflection.Register(server)

	klog.Info("Starting transport manager server")

	// Start gRPC Server in background
	go func() {
		if err := server.Serve(lis); err != nil {
			m.errCh <- fmt.Errorf("failed to start gRPC server: %w", err)
		}
	}()

	return nil
}
