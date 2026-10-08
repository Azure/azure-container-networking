package slogai

import (
	"context"
	"encoding/gob"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
)

const (
	// SinkScheme is the scheme for Application Insights sinks
	SinkScheme            = "appinsights"
	paramEndpointURL      = "endpointURL"
	paramMaxBatchInterval = "maxBatchInterval"
	paramMaxBatchSize     = "maxBatchSize"
	paramGracePeriod      = "gracePeriod"
)

func init() {
	// Register the TraceTelemetry type for gob encoding/decoding
	gob.Register(appinsights.TraceTelemetry{})
}

// SinkConfig is a container struct for an Application Insights Sink configuration.
type SinkConfig struct {
	GracePeriod time.Duration
	appinsights.TelemetryConfiguration
	// OnError is called when telemetry operations fail. If nil, errors are silently ignored.
	// This is called synchronously, so it should be fast and non-blocking.
	OnError func(err error)
}

// NewSinkConfig creates a new SinkConfig with default values.
// Returns an error if instrumentationKey is empty.
func NewSinkConfig(instrumentationKey string) (*SinkConfig, error) {
	if instrumentationKey == "" {
		return nil, errors.New("instrumentationKey cannot be empty")
	}
	return &SinkConfig{
		GracePeriod: 10 * time.Second,
		TelemetryConfiguration: appinsights.TelemetryConfiguration{
			InstrumentationKey: instrumentationKey,
			EndpointUrl:        "https://dc.services.visualstudio.com/v2/track",
			MaxBatchSize:       1024,
			MaxBatchInterval:   time.Duration(10) * time.Second,
		},
	}, nil
}

// URI generates a URI string from the SinkConfig that can be used with slog handlers.
func (sc *SinkConfig) URI() string {
	u := &url.URL{
		Scheme: SinkScheme,
		Host:   sc.InstrumentationKey,
	}

	params := url.Values{}
	if sc.EndpointUrl != "" {
		params.Set(paramEndpointURL, sc.EndpointUrl)
	}
	if sc.MaxBatchInterval > 0 {
		params.Set(paramMaxBatchInterval, sc.MaxBatchInterval.String())
	}
	if sc.MaxBatchSize > 0 {
		params.Set(paramMaxBatchSize, strconv.Itoa(sc.MaxBatchSize))
	}
	if sc.GracePeriod > 0 {
		params.Set(paramGracePeriod, sc.GracePeriod.String())
	}

	u.RawQuery = params.Encode()
	return u.String()
}

// Sink is an Application Insights sink that implements the writeSyncer interface.
type Sink struct {
	decoder traceDecoder
	client  appinsights.TelemetryClient
	cfg     *SinkConfig
	ctx     context.Context
	cancel  context.CancelFunc
	closed  atomic.Bool
}

// NewSink creates a new Application Insights sink.
func NewSink(cfg *SinkConfig) *Sink {
	client := appinsights.NewTelemetryClientFromConfig(&cfg.TelemetryConfiguration)

	ctx, cancel := context.WithCancel(context.Background())

	return &Sink{
		decoder: newTraceDecoder(),
		client:  client,
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Write implements the writeSyncer interface.
func (s *Sink) Write(b []byte) (int, error) {
	trace, err := s.decoder.decode(b)
	if err != nil {
		return 0, errors.Wrap(err, "sink failed to decode trace")
	}

	s.client.Track(trace)
	return len(b), nil
}

// Sync implements the writeSyncer interface.
func (s *Sink) Sync() error {
	// Flush any pending telemetry
	select {
	case <-s.client.Channel().Close(s.cfg.GracePeriod):
		return nil
	case <-time.After(s.cfg.GracePeriod + time.Second):
		err := errors.New("sink failed to flush within grace period")
		s.reportError(err)
		return err
	}
}

// reportError calls the OnError callback if configured.
func (s *Sink) reportError(err error) {
	if s.cfg.OnError != nil {
		s.cfg.OnError(err)
	}
}

// Close closes the sink and flushes any remaining telemetry.
// Close is idempotent - calling it multiple times has no effect.
func (s *Sink) Close() error {
	if s.closed.Swap(true) {
		return nil // Already closed
	}
	defer s.cancel()
	return s.Sync()
}
