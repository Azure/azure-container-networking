package slogai

import (
	"sync"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

func TestNewTraceEncoder(t *testing.T) {
	enc := newTraceEncoder()
	if enc == nil {
		t.Fatal("expected non-nil encoder")
	}
}

func TestNewTraceDecoder(t *testing.T) {
	dec := newTraceDecoder()
	if dec == nil {
		t.Fatal("expected non-nil decoder")
	}
}

func TestEncoder_Encode_Basic(t *testing.T) {
	enc := newTraceEncoder()

	trace := appinsights.NewTraceTelemetry("test message", contracts.Information)
	trace.Properties["key"] = "value"

	data, err := enc.encode(trace)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty encoded data")
	}
}

func TestDecoder_Decode_Basic(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	original := appinsights.NewTraceTelemetry("test message", contracts.Warning)
	original.Properties["key1"] = "value1"
	original.Properties["key2"] = "value2"

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if decoded.Message != original.Message {
		t.Errorf("message mismatch: got %q, want %q", decoded.Message, original.Message)
	}
	if decoded.SeverityLevel != original.SeverityLevel {
		t.Errorf("severity mismatch: got %v, want %v", decoded.SeverityLevel, original.SeverityLevel)
	}
	if decoded.Properties["key1"] != "value1" {
		t.Errorf("property key1 mismatch: got %q", decoded.Properties["key1"])
	}
	if decoded.Properties["key2"] != "value2" {
		t.Errorf("property key2 mismatch: got %q", decoded.Properties["key2"])
	}
}

func TestEncoder_RoundTrip_AllSeverityLevels(t *testing.T) {
	severities := []contracts.SeverityLevel{
		contracts.Verbose,
		contracts.Information,
		contracts.Warning,
		contracts.Error,
		contracts.Critical,
	}

	enc := newTraceEncoder()
	dec := newTraceDecoder()

	for _, sev := range severities {
		t.Run(sev.String(), func(t *testing.T) {
			original := appinsights.NewTraceTelemetry("test", sev)

			data, err := enc.encode(original)
			if err != nil {
				t.Fatalf("encode() error = %v", err)
			}

			decoded, err := dec.decode(data)
			if err != nil {
				t.Fatalf("decode() error = %v", err)
			}

			if decoded.SeverityLevel != sev {
				t.Errorf("severity mismatch: got %v, want %v", decoded.SeverityLevel, sev)
			}
		})
	}
}

func TestEncoder_RoundTrip_WithTags(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	original := appinsights.NewTraceTelemetry("test", contracts.Information)
	original.Tags["ai.user.id"] = "user123"
	original.Tags["ai.operation.id"] = "op456"

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if decoded.Tags["ai.user.id"] != "user123" {
		t.Errorf("tag ai.user.id mismatch: got %q", decoded.Tags["ai.user.id"])
	}
	if decoded.Tags["ai.operation.id"] != "op456" {
		t.Errorf("tag ai.operation.id mismatch: got %q", decoded.Tags["ai.operation.id"])
	}
}

func TestEncoder_RoundTrip_WithTimestamp(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	original := appinsights.NewTraceTelemetry("test", contracts.Information)
	testTime := time.Date(2024, 6, 15, 10, 30, 45, 123456789, time.UTC)
	original.Timestamp = testTime

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if !decoded.Timestamp.Equal(testTime) {
		t.Errorf("timestamp mismatch: got %v, want %v", decoded.Timestamp, testTime)
	}
}

func TestEncoder_Concurrent(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	const numGoroutines = 50
	const numOps = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines*numOps)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				trace := appinsights.NewTraceTelemetry("concurrent test", contracts.Information)
				trace.Properties["goroutine"] = string(rune('A' + id%26))
				trace.Properties["iteration"] = string(rune('0' + j%10))

				data, err := enc.encode(trace)
				if err != nil {
					errors <- err
					continue
				}

				decoded, err := dec.decode(data)
				if err != nil {
					errors <- err
					continue
				}

				if decoded.Message != "concurrent test" {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("concurrent error: %v", err)
		}
	}
}

func TestEncoder_EmptyMessage(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	original := appinsights.NewTraceTelemetry("", contracts.Information)

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if decoded.Message != "" {
		t.Errorf("expected empty message, got %q", decoded.Message)
	}
}

func TestEncoder_LargeMessage(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	// Create a large message (10KB)
	largeMessage := make([]byte, 10*1024)
	for i := range largeMessage {
		largeMessage[i] = byte('A' + i%26)
	}

	original := appinsights.NewTraceTelemetry(string(largeMessage), contracts.Information)

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if decoded.Message != string(largeMessage) {
		t.Error("large message mismatch")
	}
}

func TestEncoder_ManyProperties(t *testing.T) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()

	original := appinsights.NewTraceTelemetry("test", contracts.Information)
	for i := range 100 {
		key := string(rune('a'+i/26)) + string(rune('a'+i%26))
		original.Properties[key] = string(rune('0' + i%10))
	}

	data, err := enc.encode(original)
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	decoded, err := dec.decode(data)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if len(decoded.Properties) != len(original.Properties) {
		t.Errorf("properties count mismatch: got %d, want %d", len(decoded.Properties), len(original.Properties))
	}
}

func TestDecoder_InvalidData(t *testing.T) {
	dec := newTraceDecoder()

	_, err := dec.decode([]byte("invalid gob data"))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestDecoder_EmptyData(t *testing.T) {
	dec := newTraceDecoder()

	_, err := dec.decode([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestEncoder_SetTraceTelemetry(t *testing.T) {
	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("test", contracts.Information)

	// This method exists for internal use, verify it doesn't panic
	enc.setTraceTelemetry(trace)
}

func BenchmarkEncoder_Encode(b *testing.B) {
	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	trace.Properties["key1"] = "value1"
	trace.Properties["key2"] = "value2"

	for b.Loop() {
		enc.encode(trace)
	}
}

func BenchmarkDecoder_Decode(b *testing.B) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	trace.Properties["key1"] = "value1"
	data, _ := enc.encode(trace)

	for b.Loop() {
		dec.decode(data)
	}
}

func BenchmarkEncoder_RoundTrip(b *testing.B) {
	enc := newTraceEncoder()
	dec := newTraceDecoder()
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	trace.Properties["key"] = "value"

	for b.Loop() {
		data, _ := enc.encode(trace)
		dec.decode(data)
	}
}

func BenchmarkEncoder_Concurrent(b *testing.B) {
	enc := newTraceEncoder()
	trace := appinsights.NewTraceTelemetry("benchmark message", contracts.Information)
	trace.Properties["key"] = "value"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			enc.encode(trace)
		}
	})
}
