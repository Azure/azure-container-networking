package slogai

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

// DropPolicy determines what happens when the async buffer is full.
type DropPolicy int

const (
	// DropNewest drops the newest log entry when buffer is full.
	DropNewest DropPolicy = iota
	// DropOldest drops the oldest log entry when buffer is full.
	DropOldest
	// Block blocks until space is available (may cause backpressure).
	Block
)

// AsyncSinkConfig configures the AsyncSink behavior.
type AsyncSinkConfig struct {
	// BufferSize is the size of the async buffer channel.
	// Default: 1000
	BufferSize int

	// DropPolicy determines behavior when buffer is full.
	// Default: DropNewest
	DropPolicy DropPolicy

	// OnDropped is called when logs are dropped due to buffer overflow.
	// The count parameter indicates how many logs were dropped since last call.
	// This is called asynchronously and should be fast.
	OnDropped func(count int64)

	// DrainTimeout is the maximum time to wait for draining the buffer on Close.
	// Default: 5 seconds
	DrainTimeout time.Duration
}

// DefaultAsyncSinkConfig returns a default AsyncSinkConfig.
func DefaultAsyncSinkConfig() *AsyncSinkConfig {
	return &AsyncSinkConfig{
		BufferSize:   1000,
		DropPolicy:   DropNewest,
		DrainTimeout: 5 * time.Second,
	}
}

// AsyncSink wraps a Sink with async buffered writes.
// It implements the writeSyncer interface for use with Handler.
type AsyncSink struct {
	sink         *Sink
	buffer       chan []byte
	cfg          *AsyncSinkConfig
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	droppedCount atomic.Int64
	closed       atomic.Bool
}

// NewAsyncSink creates a new AsyncSink wrapping the provided Sink.
// If asyncCfg is nil, default configuration is used.
func NewAsyncSink(sink *Sink, asyncCfg *AsyncSinkConfig) *AsyncSink {
	if asyncCfg == nil {
		asyncCfg = DefaultAsyncSinkConfig()
	}

	if asyncCfg.BufferSize <= 0 {
		asyncCfg.BufferSize = 1000
	}

	if asyncCfg.DrainTimeout <= 0 {
		asyncCfg.DrainTimeout = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	a := &AsyncSink{
		sink:   sink,
		buffer: make(chan []byte, asyncCfg.BufferSize),
		cfg:    asyncCfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Start the background worker
	a.wg.Add(1)
	go a.worker()

	return a
}

// worker processes buffered logs in the background.
func (a *AsyncSink) worker() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			// Drain remaining items from buffer
			a.drain()
			return
		case data := <-a.buffer:
			_, _ = a.sink.Write(data)
		}
	}
}

// drain writes remaining buffered items before shutdown.
func (a *AsyncSink) drain() {
	for {
		select {
		case data := <-a.buffer:
			_, _ = a.sink.Write(data)
		default:
			return
		}
	}
}

// Write implements the writeSyncer interface.
// It buffers the data for async writing. Depending on DropPolicy,
// it may drop logs when the buffer is full.
func (a *AsyncSink) Write(b []byte) (int, error) {
	if a.closed.Load() {
		return 0, errors.New("async sink is closed")
	}

	// Make a copy of the data since b may be reused by caller
	data := make([]byte, len(b))
	copy(data, b)

	switch a.cfg.DropPolicy {
	case Block:
		select {
		case a.buffer <- data:
			return len(b), nil
		case <-a.ctx.Done():
			return 0, errors.New("async sink is closing")
		}
	case DropOldest:
		select {
		case a.buffer <- data:
			return len(b), nil
		default:
			// Buffer is full, try to drop oldest
			select {
			case <-a.buffer:
				// Dropped oldest, now add new
				a.recordDrop()
			default:
				// Someone else already consumed, try again
			}
			// Try to add again (non-blocking)
			select {
			case a.buffer <- data:
				return len(b), nil
			default:
				a.recordDrop()
				return len(b), nil // Return success, but log was dropped
			}
		}
	default: // DropNewest
		select {
		case a.buffer <- data:
			return len(b), nil
		default:
			// Buffer is full, drop this log
			a.recordDrop()
			return len(b), nil // Return success, but log was dropped
		}
	}
}

// recordDrop increments the drop counter and optionally calls the callback.
func (a *AsyncSink) recordDrop() {
	count := a.droppedCount.Add(1)
	if a.cfg.OnDropped != nil {
		a.cfg.OnDropped(count)
	}
}

// Sync flushes buffered data by waiting for the buffer to drain.
// It waits up to DrainTimeout for pending logs to be written.
func (a *AsyncSink) Sync() error {
	// Wait for buffer to empty (with timeout)
	timeout := time.After(a.cfg.DrainTimeout)
	for {
		select {
		case <-timeout:
			if len(a.buffer) > 0 {
				return errors.Errorf("async sink sync timeout with %d items remaining", len(a.buffer))
			}
			return a.sink.Sync()
		default:
			if len(a.buffer) == 0 {
				return a.sink.Sync()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Close gracefully shuts down the AsyncSink.
// It stops accepting new writes, drains the buffer, and closes the underlying sink.
func (a *AsyncSink) Close() error {
	if a.closed.Swap(true) {
		return nil // Already closed
	}

	// Signal worker to stop
	a.cancel()

	// Wait for worker to finish draining
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Worker finished
	case <-time.After(a.cfg.DrainTimeout):
		// Timeout waiting for worker
	}

	return a.sink.Close()
}

// DroppedCount returns the total number of logs dropped due to buffer overflow.
func (a *AsyncSink) DroppedCount() int64 {
	return a.droppedCount.Load()
}

// BufferLen returns the current number of items in the buffer.
func (a *AsyncSink) BufferLen() int {
	return len(a.buffer)
}
