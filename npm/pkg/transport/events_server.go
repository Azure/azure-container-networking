package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/dpshim"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
	"k8s.io/klog/v2"
)

// EventsServer contains of the grpc server and the watchdog server
type EventsServer struct {
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
	inCh chan *protos.Events

	// regCh is the registration channel
	regCh chan clientStreamConnection

	// deregCh is the deregistration channel
	deregCh chan deregistrationEvent

	// errCh is the error channel
	errCh chan error

	// dp has the dataplane instance, helps in hydration calls
	dp *dpshim.DPShim
}

// NewEventsServer creates an instance of the EventsServer
func NewEventsServer(ctx context.Context, port int, dp *dpshim.DPShim) *EventsServer {
	// Create a registration channel
	regCh := make(chan clientStreamConnection, grpcMaxConcurrentStreams)

	// Create a deregistration channel
	deregCh := make(chan deregistrationEvent, grpcMaxConcurrentStreams)

	return &EventsServer{
		ctx:           ctx,
		Server:        NewServer(ctx, regCh),
		Watchdog:      NewWatchdog(deregCh),
		Registrations: make(map[string]clientStreamConnection),
		port:          port,
		inCh:          dp.OutChannel,
		errCh:         make(chan error),
		deregCh:       deregCh,
		regCh:         regCh,
		dp:            dp,
	}
}

// InputChannel returns the input channel for the manager
func (m *EventsServer) InputChannel() chan *protos.Events {
	return m.inCh
}

// Start starts the events manager (grpc server and watchdog)
func (m *EventsServer) Start(stopCh <-chan struct{}) error {
	klog.Info("Starting transport manager")
	if err := m.start(stopCh); err != nil {
		klog.Errorf("Failed to Start transport manager: %v", err)
		return err
	}
	return nil
}

func (m *EventsServer) start(stopCh <-chan struct{}) error {
	if err := m.handle(); err != nil {
		return fmt.Errorf("failed to start transport manager handlers: %w", err)
	}

	for {
		select {
		case client := <-m.regCh:
			// (TODO) Hydration is a very expensive event, so we want to make sure
			// that pagination is done for large clusters. In case of a daemon restart in a large cluster
			// we should be able to hydrate daemon in multiple phases,
			// 1. 1st Level IPSets
			// 2. Nested IPSets
			// 3. Network Policies
			// within the same castegory we will have to paginate.
			klog.Infof("Registering remote client %s", client)
			m.Registrations[client.String()] = client
			event, err := m.dp.HydrateClients()
			if err != nil {
				klog.Errorf("Failed to hydrate client %s: %v", client, err)
			}
			// (TODO) Hydration event takes a lock of whole DPShim instance, essentially blocking the
			// controllers from receiving any more new events or servicing existing daemons.
			// So we will need to add a buffering mechanism to wait until either we have a N number of daemons
			// or hit S milliseconds of wait time and send huydration event to all the buffered daemons.
			go func() {
				klog.Infof("Hydrating remote client %s", client)
				if err := client.stream.SendMsg(event); err != nil {
					klog.Errorf("Failed to hydrate client %s: %v", client, err)
				}
			}()
		case ev := <-m.deregCh:
			// (TODO) A heart beat for each daemon should also be added alongside watchdog to monitor
			// daemon restarts and then if that fails, we will need to delete the client.
			klog.Infof("Degregistering remote client %s", ev.remoteAddr)
			if v, ok := m.Registrations[ev.remoteAddr]; ok {
				if v.timestamp <= ev.timestamp {
					klog.Infof("Deregistering remote client %s", ev.remoteAddr)
					delete(m.Registrations, ev.remoteAddr)
				} else {
					klog.Info("Ignoring stale deregistration event")
				}
			}
		case msg := <-m.inCh:
			klog.Infof("######## Received event to broadcast ######")
			for clientName, client := range m.Registrations {
				// (TODO) Should we call this SendMsg per client in a separate go routine?
				klog.Infof("######## Servicing the event to %s ######", clientName)
				if err := client.stream.SendMsg(msg); err != nil {
					// (TODO) What happens if a portion of the clients fails?
					// there should be a mechanism to retry the failed clients.
					klog.Errorf("Failed to send message to client %s: %v", client, err)
				}
			}
		case <-m.ctx.Done():
			klog.Info("Context Done. Stopping transport manager")
			return nil
		case err := <-m.errCh:
			klog.Errorf("Error in transport manager: %v", err)
			return err
		case <-stopCh:
			klog.Info("Received message on stop channel. Stopping transport manager")
			return nil
		}
	}
}

func (m *EventsServer) handle() error {
	klog.Infof("Starting transport manager listener on port %v", m.port)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", m.port))
	if err != nil {
		return fmt.Errorf("failed to handle server connections: %w", err)
	}

	// load the server certificates
	creds, err := serverTLSCreds()
	if err != nil {
		return fmt.Errorf("failed to load TLS certificates: %w", err)
	}

	var opts []grpc.ServerOption = []grpc.ServerOption{
		grpc.Creds(creds),
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
