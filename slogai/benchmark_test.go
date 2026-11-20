package slogai

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

// discardWriteSyncer is a no-op writeSyncer for benchmarking.
type discardWriteSyncer struct{}

func (d *discardWriteSyncer) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardWriteSyncer) Sync() error                 { return nil }

// BenchmarkEndToEnd_SimpleLog benchmarks a simple log message end-to-end.
func BenchmarkEndToEnd_SimpleLog(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	logger := slog.New(h)
	ctx := context.Background()

	for b.Loop() {
		logger.InfoContext(ctx, "benchmark message")
	}
}

// BenchmarkEndToEnd_WithAttributes benchmarks logging with attributes.
func BenchmarkEndToEnd_WithAttributes(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	logger := slog.New(h)
	ctx := context.Background()

	for b.Loop() {
		logger.InfoContext(ctx, "benchmark message",
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
			slog.Float64("key3", 3.14),
		)
	}
}

// BenchmarkEndToEnd_WithGroups benchmarks logging with grouped attributes.
func BenchmarkEndToEnd_WithGroups(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	logger := slog.New(h)
	ctx := context.Background()

	for b.Loop() {
		logger.InfoContext(ctx, "benchmark message",
			slog.Group("request",
				slog.String("method", "GET"),
				slog.String("path", "/api/users"),
				slog.Duration("duration", 150*time.Millisecond),
			),
		)
	}
}

// BenchmarkEndToEnd_WithFieldMappers benchmarks logging with field mappers.
func BenchmarkEndToEnd_WithFieldMappers(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	h = h.WithFieldMappers(DefaultMappers)
	logger := slog.New(h)
	ctx := context.Background()

	for b.Loop() {
		logger.InfoContext(ctx, "benchmark message",
			slog.String("user_id", "user123"),
			slog.String("operation_id", "op456"),
		)
	}
}

// BenchmarkEndToEnd_WithPersistentAttrs benchmarks logging with persistent attributes.
func BenchmarkEndToEnd_WithPersistentAttrs(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	logger := slog.New(h).With(
		slog.String("service", "api"),
		slog.String("version", "1.0.0"),
	)
	ctx := context.Background()

	for b.Loop() {
		logger.InfoContext(ctx, "benchmark message",
			slog.String("key", "value"),
		)
	}
}

// BenchmarkHandler_Clone benchmarks the handler clone operation.
func BenchmarkHandler_Clone(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	h = h.WithFieldMappers(DefaultMappers)

	for b.Loop() {
		h.WithAttrs([]slog.Attr{slog.String("key", "value")})
	}
}

// BenchmarkEncoder_LargePayload benchmarks encoding large payloads.
func BenchmarkEncoder_LargePayload(b *testing.B) {
	enc := newTraceEncoder()

	// Create a trace with many properties
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	for i := range 50 {
		key := string(rune('a'+i/26)) + string(rune('a'+i%26))
		trace.Properties[key] = "some value that is moderately long to simulate real data"
	}

	for b.Loop() {
		enc.encode(trace)
	}
}

// BenchmarkAttrValueString benchmarks the attribute value string conversion.
func BenchmarkAttrValueString(b *testing.B) {
	values := []slog.Value{
		slog.StringValue("test string"),
		slog.Int64Value(12345678),
		slog.Float64Value(3.14159265359),
		slog.BoolValue(true),
		slog.DurationValue(5 * time.Second),
		slog.TimeValue(time.Now()),
	}

	for b.Loop() {
		for _, v := range values {
			attrValueString(v)
		}
	}
}

// BenchmarkHandler_Parallel tests handler throughput under concurrent load.
func BenchmarkHandler_Parallel(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "parallel benchmark", 0)
		record.AddAttrs(
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		)
		for pb.Next() {
			h.Handle(ctx, record)
		}
	})
}

// BenchmarkHandler_ClonedParallel tests throughput with cloned handlers (independent encoders).
func BenchmarkHandler_ClonedParallel(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own cloned handler
		localHandler := h.WithAttrs([]slog.Attr{}).(*Handler)
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "cloned parallel benchmark", 0)
		record.AddAttrs(slog.String("key", "value"))

		for pb.Next() {
			localHandler.Handle(ctx, record)
		}
	})
}

// BenchmarkMemoryAllocation reports memory allocations per log.
func BenchmarkMemoryAllocation(b *testing.B) {
	mock := &discardWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "allocation test", 0)
		record.AddAttrs(
			slog.String("key1", "value1"),
			slog.String("key2", "value2"),
			slog.Int("key3", 42),
		)
		h.Handle(ctx, record)
	}
}

// BenchmarkAsyncVsSync compares async and sync sink performance.
func BenchmarkAsyncVsSync(b *testing.B) {
	b.Run("Sync", func(b *testing.B) {
		sinkCfg, _ := NewSinkConfig("test-key")
		sink := NewSink(sinkCfg)
		defer sink.Close()

		h := NewHandler(slog.LevelInfo, sink)
		ctx := context.Background()

		b.ResetTimer()
		for b.Loop() {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "sync test", 0)
			h.Handle(ctx, record)
		}
	})

	b.Run("Async", func(b *testing.B) {
		sinkCfg, _ := NewSinkConfig("test-key")
		sink := NewSink(sinkCfg)
		defer sink.Close()

		asyncCfg := &AsyncSinkConfig{BufferSize: 100000}
		asyncSink := NewAsyncSink(sink, asyncCfg)
		defer asyncSink.Close()

		h := NewHandler(slog.LevelInfo, asyncSink)
		ctx := context.Background()

		b.ResetTimer()
		for b.Loop() {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "async test", 0)
			h.Handle(ctx, record)
		}
	})
}
