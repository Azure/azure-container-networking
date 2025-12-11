package deviceplugin

import (
	"context"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

const defaultStatInterval time.Duration = 5 * time.Second

type SocketWatcherOption func(*socketWatcherOptions)

type socketWatcherOptions struct {
	statInterval time.Duration
}

func SocketWatcherStatInterval(d time.Duration) SocketWatcherOption {
	return func(o *socketWatcherOptions) {
		o.statInterval = d
	}
}

type SocketWatcher struct {
	socketChans map[string]<-chan struct{}
	mutex       sync.Mutex
	logger      *zap.Logger
	options     socketWatcherOptions
}

func NewSocketWatcher(logger *zap.Logger, opts ...SocketWatcherOption) *SocketWatcher {
	defaultOptions := socketWatcherOptions{
		statInterval: defaultStatInterval,
	}
	for _, o := range opts {
		o(&defaultOptions)
	}
	return &SocketWatcher{
		socketChans: make(map[string]<-chan struct{}),
		logger:      logger,
		options:     defaultOptions,
	}
}

// watchSocket returns a channel that will be closed when the socket is removed or the context is cancelled
func (s *SocketWatcher) WatchSocket(ctx context.Context, socket string) <-chan struct{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// if a socket is already being watched, return its channel
	if ch, ok := s.socketChans[socket]; ok {
		return ch
	}
	// otherwise, start watching it and return a new channel
	socketChan := make(chan struct{})
	s.socketChans[socket] = socketChan
	go func() {
		defer func() {
			s.mutex.Lock()
			delete(s.socketChans, socket)
			s.mutex.Unlock()
			close(socketChan)
		}()
		ticker := time.NewTicker(s.options.statInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Lstat(socket); err != nil {
					s.logger.Info("failed to stat socket", zap.Error(err))
					return
				}
			}
		}
	}()
	return socketChan
}
