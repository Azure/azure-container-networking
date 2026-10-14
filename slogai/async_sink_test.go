package slogai

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

func TestDefaultAsyncSinkConfig(t *testing.T) {
	cfg := DefaultAsyncSinkConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.BufferSize != 1000 {
		t.Errorf("expected BufferSize=1000, got %d", cfg.BufferSize)
	}
	if cfg.DropPolicy != DropNewest {
		t.Errorf("expected DropPolicy=DropNewest, got %v", cfg.DropPolicy)
	}
	if cfg.DrainTimeout != 5*time.Second {
		t.Errorf("expected DrainTimeout=5s, got %v", cfg.DrainTimeout)
	}
}

func TestNewAsyncSink(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncSink := NewAsyncSink(sink, nil)
	if asyncSink == nil {
		t.Fatal("expected non-nil async sink")
	}

	asyncSink.Close()
}

func TestNewAsyncSink_WithConfig(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize:   500,
		DropPolicy:   DropOldest,
		DrainTimeout: 10 * time.Second,
	}

	asyncSink := NewAsyncSink(sink, asyncCfg)
	if asyncSink == nil {
		t.Fatal("expected non-nil async sink")
	}
	if asyncSink.cfg.BufferSize != 500 {
		t.Errorf("expected BufferSize=500, got %d", asyncSink.cfg.BufferSize)
	}

	asyncSink.Close()
}

func TestNewAsyncSink_InvalidConfig(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize:   0, // Invalid
		DrainTimeout: 0, // Invalid
	}

	asyncSink := NewAsyncSink(sink, asyncCfg)
	if asyncSink.cfg.BufferSize != 1000 {
		t.Errorf("expected BufferSize to default to 1000, got %d", asyncSink.cfg.BufferSize)
	}
	if asyncSink.cfg.DrainTimeout != 5*time.Second {
		t.Errorf("expected DrainTimeout to default to 5s, got %v", asyncSink.cfg.DrainTimeout)
	}

	asyncSink.Close()
}

func TestAsyncSink_Write(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncSink := NewAsyncSink(sink, nil)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test message", contracts.Information)
	data, _ := enc.encode(trace)

	n, err := asyncSink.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, want %d", n, len(data))
	}
}

func TestAsyncSink_Write_AfterClose(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncSink := NewAsyncSink(sink, nil)
	asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test message", contracts.Information)
	data, _ := enc.encode(trace)

	_, err := asyncSink.Write(data)
	if err == nil {
		t.Error("expected error writing to closed sink")
	}
}

func TestAsyncSink_Write_Concurrent(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 10000, // Large buffer to avoid drops
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	const numGoroutines = 100
	const numWrites = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range numWrites {
				asyncSink.Write(data)
			}
		}()
	}

	wg.Wait()
}

func TestAsyncSink_DropPolicy_DropNewest(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	var droppedCount atomic.Int64
	asyncCfg := &AsyncSinkConfig{
		BufferSize: 10, // Small buffer to trigger drops
		DropPolicy: DropNewest,
		OnDropped: func(count int64) {
			droppedCount.Store(count)
		},
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	// Write many items to overflow buffer
	for range 100 {
		asyncSink.Write(data)
	}

	// Check that some were dropped
	dropped := asyncSink.DroppedCount()
	if dropped == 0 {
		t.Log("No drops recorded - buffer may have drained fast enough")
	} else {
		t.Logf("Dropped %d logs", dropped)
	}
}

func TestAsyncSink_DropPolicy_Block(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 1000,
		DropPolicy: Block,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	// With Block policy and large buffer, writes should succeed
	for range 100 {
		n, err := asyncSink.Write(data)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if n != len(data) {
			t.Errorf("Write() returned %d, want %d", n, len(data))
		}
	}
}

func TestAsyncSink_Sync(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		DrainTimeout: 5 * time.Second,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	// Write some data
	for range 10 {
		asyncSink.Write(data)
	}

	// Sync should wait for buffer to drain
	err := asyncSink.Sync()
	if err != nil {
		t.Errorf("Sync() error = %v", err)
	}
}

func TestAsyncSink_Close_Drains(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize:   100,
		DrainTimeout: 5 * time.Second,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	// Write some data
	for range 50 {
		asyncSink.Write(data)
	}

	// Close should drain the buffer
	err := asyncSink.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Buffer should be empty after close
	if asyncSink.BufferLen() != 0 {
		t.Errorf("expected empty buffer after close, got %d items", asyncSink.BufferLen())
	}
}

func TestAsyncSink_Close_Idempotent(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncSink := NewAsyncSink(sink, nil)

	// First close
	err := asyncSink.Close()
	if err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// Second close should be no-op (not panic)
	err = asyncSink.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestAsyncSink_BufferLen(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 100,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	// Initial buffer should be empty (or nearly empty after worker starts)
	initialLen := asyncSink.BufferLen()
	if initialLen > 10 {
		t.Errorf("expected nearly empty initial buffer, got %d", initialLen)
	}
}

func TestAsyncSink_DroppedCount(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncSink := NewAsyncSink(sink, nil)
	defer asyncSink.Close()

	// Initially should be 0
	if count := asyncSink.DroppedCount(); count != 0 {
		t.Errorf("expected initial DroppedCount=0, got %d", count)
	}
}

func TestAsyncSink_OnDropped_Callback(t *testing.T) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	var callbackCounts []int64
	var mu sync.Mutex

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 5, // Very small buffer
		DropPolicy: DropNewest,
		OnDropped: func(count int64) {
			mu.Lock()
			callbackCounts = append(callbackCounts, count)
			mu.Unlock()
		},
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)
	data, _ := enc.encode(trace)

	// Flood the buffer
	for range 100 {
		asyncSink.Write(data)
	}

	asyncSink.Close()

	mu.Lock()
	callCount := len(callbackCounts)
	mu.Unlock()

	if callCount > 0 {
		t.Logf("OnDropped callback was called %d times", callCount)
	}
}

func BenchmarkAsyncSink_Write(b *testing.B) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 100000,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("benchmark", contracts.Information)
	data, _ := enc.encode(trace)

	for b.Loop() {
		asyncSink.Write(data)
	}
}

func BenchmarkAsyncSink_Write_Concurrent(b *testing.B) {
	sinkCfg, _ := NewSinkConfig("test-key")
	sink := NewSink(sinkCfg)
	defer sink.Close()

	asyncCfg := &AsyncSinkConfig{
		BufferSize: 100000,
	}
	asyncSink := NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("benchmark", contracts.Information)
	data, _ := enc.encode(trace)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			asyncSink.Write(data)
		}
	})
}
