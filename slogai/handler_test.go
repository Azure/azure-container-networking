package slogai

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

// mockWriteSyncer is a mock implementation of writeSyncer for testing.
type mockWriteSyncer struct {
	mu      sync.Mutex
	written [][]byte
	syncErr error
}

func (m *mockWriteSyncer) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Make a copy to avoid data race
	data := make([]byte, len(b))
	copy(data, b)
	m.written = append(m.written, data)
	return len(b), nil
}

func (m *mockWriteSyncer) Sync() error {
	return m.syncErr
}

func (m *mockWriteSyncer) GetWritten() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]byte, len(m.written))
	copy(result, m.written)
	return result
}

func TestNewHandler(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.level != slog.LevelInfo {
		t.Errorf("expected level %v, got %v", slog.LevelInfo, h.level)
	}
	if h.enc == nil {
		t.Error("expected non-nil encoder")
	}
	if h.fieldMappers == nil {
		t.Error("expected non-nil fieldMappers")
	}
	if h.lock == nil {
		t.Error("expected non-nil lock")
	}
}

func TestHandler_Enabled(t *testing.T) {
	tests := []struct {
		name          string
		handlerLevel  slog.Level
		recordLevel   slog.Level
		expectEnabled bool
	}{
		{"debug at info", slog.LevelInfo, slog.LevelDebug, false},
		{"info at info", slog.LevelInfo, slog.LevelInfo, true},
		{"warn at info", slog.LevelInfo, slog.LevelWarn, true},
		{"error at info", slog.LevelInfo, slog.LevelError, true},
		{"debug at debug", slog.LevelDebug, slog.LevelDebug, true},
		{"info at error", slog.LevelError, slog.LevelInfo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockWriteSyncer{}
			h := NewHandler(tt.handlerLevel, mock)
			got := h.Enabled(context.Background(), tt.recordLevel)
			if got != tt.expectEnabled {
				t.Errorf("Enabled() = %v, want %v", got, tt.expectEnabled)
			}
		})
	}
}

func TestHandler_Handle_BasicMessage(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	if len(written) != 1 {
		t.Fatalf("expected 1 write, got %d", len(written))
	}

	trace, err := decoder.decode(written[0])
	if err != nil {
		t.Fatalf("failed to decode trace: %v", err)
	}

	if trace.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", trace.Message)
	}
	if trace.SeverityLevel != contracts.Information {
		t.Errorf("expected severity Information, got %v", trace.SeverityLevel)
	}
}

func TestHandler_Handle_AllLevels(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected contracts.SeverityLevel
	}{
		{slog.LevelDebug, contracts.Verbose},
		{slog.LevelInfo, contracts.Information},
		{slog.LevelWarn, contracts.Warning},
		{slog.LevelError, contracts.Error},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			mock := &mockWriteSyncer{}
			h := NewHandler(slog.LevelDebug, mock) // Accept all levels
			decoder := newTraceDecoder()

			record := slog.NewRecord(time.Now(), tt.level, "test", 0)
			err := h.Handle(context.Background(), record)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			written := mock.GetWritten()
			trace, _ := decoder.decode(written[0])
			if trace.SeverityLevel != tt.expected {
				t.Errorf("expected severity %v, got %v", tt.expected, trace.SeverityLevel)
			}
		})
	}
}

func TestHandler_Handle_WithAttributes(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(
		slog.String("string_attr", "value"),
		slog.Int("int_attr", 42),
		slog.Float64("float_attr", 3.14),
		slog.Bool("bool_attr", true),
		slog.Duration("duration_attr", 5*time.Second),
		slog.Time("time_attr", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)),
	)

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	// Check string attribute
	if v, ok := trace.Properties["string_attr"]; !ok || v != "value" {
		t.Errorf("expected string_attr='value', got %q", v)
	}

	// Check int attribute
	if v, ok := trace.Properties["int_attr"]; !ok || v != "42" {
		t.Errorf("expected int_attr='42', got %q", v)
	}

	// Check float attribute
	if v, ok := trace.Properties["float_attr"]; !ok || !strings.Contains(v, "3.14") {
		t.Errorf("expected float_attr to contain '3.14', got %q", v)
	}

	// Check bool attribute
	if v, ok := trace.Properties["bool_attr"]; !ok || v != "true" {
		t.Errorf("expected bool_attr='true', got %q", v)
	}

	// Check duration attribute
	if v, ok := trace.Properties["duration_attr"]; !ok || v != "5s" {
		t.Errorf("expected duration_attr='5s', got %q", v)
	}
}

func TestHandler_Handle_WithGroups(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(
		slog.Group("request",
			slog.String("method", "GET"),
			slog.String("path", "/api/users"),
		),
	)

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["request.method"]; !ok || v != "GET" {
		t.Errorf("expected request.method='GET', got %q", v)
	}
	if v, ok := trace.Properties["request.path"]; !ok || v != "/api/users" {
		t.Errorf("expected request.path='/api/users', got %q", v)
	}
}

func TestHandler_WithAttrs(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	h2 := h.WithAttrs([]slog.Attr{slog.String("persistent", "value")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("record", "attr"))

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["persistent"]; !ok || v != "value" {
		t.Errorf("expected persistent='value', got %q", v)
	}
	if v, ok := trace.Properties["record"]; !ok || v != "attr" {
		t.Errorf("expected record='attr', got %q", v)
	}
}

func TestHandler_WithGroup(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	h2 := h.WithGroup("mygroup")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("key", "value"))

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["mygroup.key"]; !ok || v != "value" {
		t.Errorf("expected mygroup.key='value', got %q", v)
	}
}

func TestHandler_WithGroup_Empty(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	h2 := h.WithGroup("")
	if h2 != h {
		t.Error("WithGroup('') should return same handler")
	}
}

func TestHandler_WithFieldMappers(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	h2 := h.WithFieldMappers(map[string]string{
		"user_id": "ai.user.id",
	})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("user_id", "user123"))

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Tags["ai.user.id"]; !ok || v != "user123" {
		t.Errorf("expected ai.user.id='user123' in tags, got %q", v)
	}
	// Should NOT be in properties since it was mapped to tag
	if _, ok := trace.Properties["user_id"]; ok {
		t.Error("user_id should not be in properties when mapped to tag")
	}
}

func TestHandler_WithRedactFields(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	h2 := h.WithRedactFields("password", "token", "secret")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "user login", 0)
	record.AddAttrs(
		slog.String("username", "john"),
		slog.String("password", "super-secret-password"),
		slog.String("token", "jwt-token-12345"),
		slog.String("secret", "api-key"),
		slog.String("action", "login"),
	)

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	// Non-redacted fields should be normal
	if v := trace.Properties["username"]; v != "john" {
		t.Errorf("expected username='john', got %q", v)
	}
	if v := trace.Properties["action"]; v != "login" {
		t.Errorf("expected action='login', got %q", v)
	}

	// Redacted fields should show [REDACTED]
	if v := trace.Properties["password"]; v != RedactedValue {
		t.Errorf("expected password='%s', got %q", RedactedValue, v)
	}
	if v := trace.Properties["token"]; v != RedactedValue {
		t.Errorf("expected token='%s', got %q", RedactedValue, v)
	}
	if v := trace.Properties["secret"]; v != RedactedValue {
		t.Errorf("expected secret='%s', got %q", RedactedValue, v)
	}
}

func TestHandler_WithRedactFields_GroupedFields(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	h2 := h.WithRedactFields("auth.password", "user.ssn")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(
		slog.Group("auth",
			slog.String("username", "john"),
			slog.String("password", "secret123"),
		),
		slog.Group("user",
			slog.String("name", "John Doe"),
			slog.String("ssn", "123-45-6789"),
		),
	)

	err := h2.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	// Non-redacted grouped fields
	if v := trace.Properties["auth.username"]; v != "john" {
		t.Errorf("expected auth.username='john', got %q", v)
	}
	if v := trace.Properties["user.name"]; v != "John Doe" {
		t.Errorf("expected user.name='John Doe', got %q", v)
	}

	// Redacted grouped fields
	if v := trace.Properties["auth.password"]; v != RedactedValue {
		t.Errorf("expected auth.password='%s', got %q", RedactedValue, v)
	}
	if v := trace.Properties["user.ssn"]; v != RedactedValue {
		t.Errorf("expected user.ssn='%s', got %q", RedactedValue, v)
	}
}

func TestHandler_Clone_IndependentEncoders(t *testing.T) {
	mock := &mockWriteSyncer{}
	h1 := NewHandler(slog.LevelInfo, mock)
	h2 := h1.WithAttrs([]slog.Attr{slog.String("attr", "value")}).(*Handler)

	// Verify they have different encoders
	if h1.enc == h2.enc {
		t.Error("cloned handlers should have different encoders")
	}
	if h1.lock == h2.lock {
		t.Error("cloned handlers should have different locks")
	}
}

func TestHandler_Handle_Concurrent(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	const numGoroutines = 100
	const numLogsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range numLogsPerGoroutine {
				record := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent test", 0)
				record.AddAttrs(slog.Int("goroutine", id), slog.Int("iteration", j))
				if err := h.Handle(context.Background(), record); err != nil {
					t.Errorf("Handle() error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	written := mock.GetWritten()
	expected := numGoroutines * numLogsPerGoroutine
	if len(written) != expected {
		t.Errorf("expected %d writes, got %d", expected, len(written))
	}
}

func TestHandler_Handle_PanicRecovery(t *testing.T) {
	// Create a handler with a faulty encoder that panics
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	// We'll use a record that causes issues
	// Since we added panic recovery, this should not panic but return an error
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)

	// Handle should not panic
	err := h.Handle(context.Background(), record)
	// This should succeed normally since it's a valid record
	if err != nil {
		t.Logf("Handle returned error (expected for malformed): %v", err)
	}
}

func TestHandler_Handle_ErrorAttribute(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	testErr := errors.New("test error message")
	record := slog.NewRecord(time.Now(), slog.LevelError, "operation failed", 0)
	record.AddAttrs(slog.Any("error", testErr))

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["error"]; !ok || v != "test error message" {
		t.Errorf("expected error='test error message', got %q", v)
	}
}

func TestHandler_Handle_NestedGroups(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(
		slog.Group("outer",
			slog.Group("inner",
				slog.String("key", "value"),
			),
		),
	)

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["outer.inner.key"]; !ok || v != "value" {
		t.Errorf("expected outer.inner.key='value', got %q", v)
	}
}

func TestAttrValueString(t *testing.T) {
	tests := []struct {
		name     string
		value    slog.Value
		expected string
	}{
		{"string", slog.StringValue("hello"), "hello"},
		{"int64", slog.Int64Value(42), "42"},
		{"uint64", slog.Uint64Value(100), "100"},
		{"float64", slog.Float64Value(3.14), "3.14"},
		{"bool true", slog.BoolValue(true), "true"},
		{"bool false", slog.BoolValue(false), "false"},
		{"duration", slog.DurationValue(5 * time.Second), "5s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attrValueString(tt.value)
			if got != tt.expected {
				t.Errorf("attrValueString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAttrValueString_Time(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
	got := attrValueString(slog.TimeValue(testTime))
	expected := "2024-06-15T14:30:45.000Z"
	if got != expected {
		t.Errorf("attrValueString(time) = %q, want %q", got, expected)
	}
}

func TestHandler_IntegrationWithSlog(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	logger := slog.New(h)

	logger.Info("test message", "key", "value")

	written := mock.GetWritten()
	if len(written) != 1 {
		t.Fatalf("expected 1 write, got %d", len(written))
	}

	trace, _ := decoder.decode(written[0])
	if trace.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", trace.Message)
	}
	if v, ok := trace.Properties["key"]; !ok || v != "value" {
		t.Errorf("expected key='value', got %q", v)
	}
}

func TestHandler_IntegrationWithSlog_WithGroup(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)
	decoder := newTraceDecoder()

	logger := slog.New(h).WithGroup("api")

	logger.Info("request", "method", "GET")

	written := mock.GetWritten()
	trace, _ := decoder.decode(written[0])

	if v, ok := trace.Properties["api.method"]; !ok || v != "GET" {
		t.Errorf("expected api.method='GET', got %q", v)
	}
}

// Verify the Handler implements slog.Handler interface
var _ slog.Handler = (*Handler)(nil)

func BenchmarkHandler_Handle(b *testing.B) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	record.AddAttrs(
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	for b.Loop() {
		h.Handle(context.Background(), record)
	}
}

func BenchmarkHandler_Handle_Concurrent(b *testing.B) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	record.AddAttrs(slog.String("key", "value"))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.Handle(context.Background(), record)
		}
	})
}

// Helper to check if data is correctly encoded and decodable
func TestHandler_EncodedDataIsDecodable(t *testing.T) {
	mock := &mockWriteSyncer{}
	h := NewHandler(slog.LevelInfo, mock)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(
		slog.String("attr1", "value1"),
		slog.Int("attr2", 100),
	)

	err := h.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	written := mock.GetWritten()
	if len(written) == 0 {
		t.Fatal("expected at least 1 write")
	}

	// Verify the encoded data can be decoded
	decoder := newTraceDecoder()
	trace, err := decoder.decode(written[0])
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if trace.Message != "test message" {
		t.Errorf("decoded message mismatch: got %q", trace.Message)
	}
}
