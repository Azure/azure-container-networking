package transport

import (
	"context"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"google.golang.org/grpc/peer"
)

// clientStreamConnection represents a client stream connection
type clientStreamConnection struct {
	stream protos.DataplaneEvents_ConnectServer
	*protos.DatapathPodMetadata
	addr      string
	timestamp int64
}

// String returns the address of the client
func (c clientStreamConnection) String() string {
	return c.addr
}

// DataplaneEventsServer is the gRPC server for the DataplaneEvents service
type DataplaneEventsServer struct {
	protos.UnimplementedDataplaneEventsServer
	ctx   context.Context
	regCh chan<- clientStreamConnection
}

// NewServer creates a new DataplaneEventsServer instance
func NewServer(ctx context.Context, ch chan clientStreamConnection) *DataplaneEventsServer {
	return &DataplaneEventsServer{
		ctx:   ctx,
		regCh: ch,
	}
}

// Connect is called when a client connects to the server
func (d *DataplaneEventsServer) Connect(m *protos.DatapathPodMetadata, stream protos.DataplaneEvents_ConnectServer) error {
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return ErrNoPeer
	}

	conn := clientStreamConnection{
		DatapathPodMetadata: m,
		stream:              stream,
		addr:                p.Addr.String(),
		timestamp:           time.Now().Unix(),
	}

	// Add stream to the list of active streams
	d.regCh <- conn

	// This should block until the client disconnects
	<-d.ctx.Done()

	return nil
}
