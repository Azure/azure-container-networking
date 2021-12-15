package transport

import (
	"context"
	"time"

	"google.golang.org/grpc/stats"
)

// Defined type for context key
type watchdogContextKey string

// String implements the Stringer interface
func (c watchdogContextKey) String() string {
	return string(c)
}

// contextRemoteAddrKey is the key used to store the remote address in the context
var contextRemoteAddrKey = watchdogContextKey("remote-addr")

// deregistrationEvent is the type of event that is sent to the deregistration channel
type deregistrationEvent struct {
	remoteAddr string
	timestamp  int64
}

// Watchdog is a stats handler that watches for connection and RPC events.
// It implements the gRPC stats.Handler interface.
type Watchdog struct {
	// deregCh is used by the Watchdog to signal the Watchdog to deregister a remote address/client
	deregCh chan<- deregistrationEvent
}

// NewWatchdog creates a new Watchdog instance
func NewWatchdog(deregCh chan<- deregistrationEvent) stats.Handler {
	return &Watchdog{
		deregCh: deregCh,
	}
}

func (h *Watchdog) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *Watchdog) HandleRPC(ctx context.Context, _ stats.RPCStats) {
	_ = ctx
}

func (h *Watchdog) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	// Add the remote address to the context so that we can use it during a connection end event
	return context.WithValue(ctx, contextRemoteAddrKey, info.RemoteAddr.String())
}

// HandleConn processes the Conn stats.
func (h *Watchdog) HandleConn(c context.Context, s stats.ConnStats) {
	if _, ok := s.(*stats.ConnEnd); ok {
		// Watch for connection end events
		remoteAddr := c.Value(contextRemoteAddrKey).(string)
		h.deregCh <- deregistrationEvent{
			remoteAddr: remoteAddr,
			timestamp:  time.Now().Unix(),
		}
	}
}
