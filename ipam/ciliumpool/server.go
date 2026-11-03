package ciliumpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Serve stops with the process context. Binding is done before bridge startup.
func (m *Monitor) Serve(ctx context.Context, listener net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{
		Handler: m.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second,
	}
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		err := server.Shutdown(shutdown)
		if err != nil {
			// Close bounds shutdown even if a scrape client stopped reading.
			err = errors.Join(err, server.Close())
		}
		done <- err
	}()
	err := server.Serve(listener)
	cancel()
	shutdownErr := <-done
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Join(fmt.Errorf("serving pool health: %w", err), shutdownErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("stopping pool health: %w", shutdownErr)
	}
	return nil
}
