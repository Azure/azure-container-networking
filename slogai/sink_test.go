package slogai

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

func TestNewSinkConfig(t *testing.T) {
	cfg, err := NewSinkConfig("test-key")
	if err != nil {
		t.Fatalf("NewSinkConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.InstrumentationKey != "test-key" {
		t.Errorf("expected InstrumentationKey='test-key', got %q", cfg.InstrumentationKey)
	}
	if cfg.GracePeriod != 10*time.Second {
		t.Errorf("expected GracePeriod=10s, got %v", cfg.GracePeriod)
	}
	if cfg.MaxBatchSize != 1024 {
		t.Errorf("expected MaxBatchSize=1024, got %d", cfg.MaxBatchSize)
	}
	if cfg.MaxBatchInterval != 10*time.Second {
		t.Errorf("expected MaxBatchInterval=10s, got %v", cfg.MaxBatchInterval)
	}
}

func TestNewSinkConfig_EmptyKey(t *testing.T) {
	cfg, err := NewSinkConfig("")
	if err == nil {
		t.Error("expected error for empty instrumentation key")
	}
	if cfg != nil {
		t.Error("expected nil config for error case")
	}
	if !strings.Contains(err.Error(), "instrumentationKey cannot be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSinkConfig_URI(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key-123")
	cfg.EndpointUrl = "https://example.com/track"
	cfg.MaxBatchInterval = 5 * time.Second
	cfg.MaxBatchSize = 500
	cfg.GracePeriod = 15 * time.Second

	uri := cfg.URI()

	if !strings.HasPrefix(uri, "appinsights://test-key-123") {
		t.Errorf("URI should start with appinsights://test-key-123, got %q", uri)
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("failed to parse URI: %v", err)
	}

	if parsed.Scheme != SinkScheme {
		t.Errorf("expected scheme %q, got %q", SinkScheme, parsed.Scheme)
	}
	if parsed.Host != "test-key-123" {
		t.Errorf("expected host 'test-key-123', got %q", parsed.Host)
	}

	params := parsed.Query()
	if params.Get(paramEndpointURL) != "https://example.com/track" {
		t.Errorf("unexpected endpointURL: %q", params.Get(paramEndpointURL))
	}
	if params.Get(paramMaxBatchInterval) != "5s" {
		t.Errorf("unexpected maxBatchInterval: %q", params.Get(paramMaxBatchInterval))
	}
	if params.Get(paramMaxBatchSize) != "500" {
		t.Errorf("unexpected maxBatchSize: %q", params.Get(paramMaxBatchSize))
	}
	if params.Get(paramGracePeriod) != "15s" {
		t.Errorf("unexpected gracePeriod: %q", params.Get(paramGracePeriod))
	}
}

func TestSinkConfig_URI_DefaultEndpoint(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	uri := cfg.URI()

	if !strings.Contains(uri, "endpointURL") {
		t.Error("URI should contain endpointURL parameter")
	}
}

func TestNewSink(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	sink := NewSink(cfg)

	if sink == nil {
		t.Fatal("expected non-nil sink")
	}
	if sink.client == nil {
		t.Error("expected non-nil client")
	}
	if sink.decoder == nil {
		t.Error("expected non-nil decoder")
	}
	if sink.cfg != cfg {
		t.Error("expected config to be stored")
	}

	// Clean up
	sink.Close()
}

func TestSink_Write(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	sink := NewSink(cfg)
	defer sink.Close()

	// Create and encode a trace
	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test message", contracts.Information)
	trace.Properties["key"] = "value"

	data, err := enc.encode(trace)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	// Write to sink
	n, err := sink.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, want %d", n, len(data))
	}
}

func TestSink_Write_InvalidData(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	sink := NewSink(cfg)
	defer sink.Close()

	_, err := sink.Write([]byte("invalid gob data"))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestSink_Close_Idempotent(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	sink := NewSink(cfg)

	// First close should work
	err := sink.Close()
	if err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// Note: Second close may fail or succeed depending on implementation
	// The important thing is it shouldn't panic
	_ = sink.Close()
}

func TestSinkConfig_OnError(t *testing.T) {
	var errorCalled bool
	var capturedErr error

	cfg, _ := NewSinkConfig("test-key")
	cfg.OnError = func(err error) {
		errorCalled = true
		capturedErr = err
	}
	cfg.GracePeriod = 1 * time.Millisecond // Very short grace period

	sink := NewSink(cfg)

	// Write some data
	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)
	sink.Write(data)

	// Sync with very short grace period might trigger timeout
	// (depending on timing, this may or may not trigger the error callback)
	_ = sink.Sync()

	sink.Close()

	// The callback may or may not have been called depending on timing
	// This test verifies the callback mechanism works without panicking
	if errorCalled {
		t.Logf("OnError was called with: %v", capturedErr)
	}
}

func TestSink_ReportError(t *testing.T) {
	var errorCalled bool

	cfg, _ := NewSinkConfig("test-key")
	cfg.OnError = func(err error) {
		errorCalled = true
	}

	sink := NewSink(cfg)
	defer sink.Close()

	// Call reportError directly
	sink.reportError(nil)
	if !errorCalled {
		t.Error("expected OnError to be called")
	}
}

func TestSink_ReportError_NilCallback(t *testing.T) {
	cfg, _ := NewSinkConfig("test-key")
	// OnError is nil by default

	sink := NewSink(cfg)
	defer sink.Close()

	// Should not panic with nil callback
	sink.reportError(nil)
}

func BenchmarkSink_Write(b *testing.B) {
	cfg, _ := NewSinkConfig("test-key")
	sink := NewSink(cfg)
	defer sink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	trace.Properties["key"] = "value"
	data, _ := enc.encode(trace)

	for b.Loop() {
		sink.Write(data)
	}
}
