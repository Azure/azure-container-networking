package slogai

import (
	"context"
	"log/slog"
	"maps"
	"runtime"
	"strconv"
	"sync"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
	"github.com/pkg/errors"
)

var levelToSev = map[slog.Level]contracts.SeverityLevel{
	slog.LevelDebug: contracts.Verbose,
	slog.LevelInfo:  contracts.Information,
	slog.LevelWarn:  contracts.Warning,
	slog.LevelError: contracts.Error,
}

var _ slog.Handler = (*Handler)(nil)

// Handler implements slog.Handler for Application Insights.
// Handler builds Application Insights TraceTelemetry from slog records and sends them
// through an encoder to a sink for transmission to Application Insights.
type Handler struct {
	level        slog.Level
	enc          traceEncoder
	fieldMappers map[string]fieldTagMapper
	redactFields map[string]bool // Fields to redact from logs
	attrs        []slog.Attr
	groups       []string
	out          writeSyncer
	lock         *sync.Mutex
}

// RedactedValue is the placeholder used for redacted fields.
const RedactedValue = "[REDACTED]"

// writeSyncer is an interface that can write and sync data
type writeSyncer interface {
	Write([]byte) (int, error)
	Sync() error
}

// NewHandler creates a new Application Insights slog handler.
func NewHandler(level slog.Level, out writeSyncer) *Handler {
	return &Handler{
		level:        level,
		enc:          newTraceEncoder(),
		fieldMappers: make(map[string]fieldTagMapper),
		redactFields: make(map[string]bool),
		out:          out,
		lock:         &sync.Mutex{},
	}
}

// WithRedactFields returns a new Handler that redacts the specified fields.
// Redacted fields will have their values replaced with "[REDACTED]" in logs.
// This is useful for filtering sensitive data like passwords, tokens, or PII.
func (h *Handler) WithRedactFields(fields ...string) *Handler {
	clone := h.clone()
	for _, field := range fields {
		clone.redactFields[field] = true
	}
	return clone
}

// WithFieldMappers adds field mappers to transform slog attribute names to Application Insights tags.
func (h *Handler) WithFieldMappers(fieldMappers ...map[string]string) *Handler {
	clone := h.clone()
	for _, fieldMapper := range fieldMappers {
		for field, tag := range fieldMapper {
			clone.fieldMappers[field] = func(t *appinsights.TraceTelemetry, val string) {
				t.Tags[tag] = val
			}
		}
	}
	return clone
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles the Record.
// Handle is safe for concurrent use and recovers from panics caused by malformed records.
func (h *Handler) Handle(ctx context.Context, record slog.Record) (err error) {
	// Recover from panics to prevent crashes from malformed records
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("handler panic recovered: %v", r)
		}
	}()

	h.lock.Lock()
	defer h.lock.Unlock()

	// Map slog level to Application Insights severity
	severity, ok := levelToSev[record.Level]
	if !ok {
		// Default to Information for unknown levels
		severity = contracts.Information
	}

	t := appinsights.NewTraceTelemetry(record.Message, severity)

	// Set timestamp
	t.Timestamp = record.Time

	// Add PC (program counter) information if available
	if record.PC != 0 {
		// Convert PC to file:line format
		if frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next(); frame.File != "" {
			caller := frame.File + ":" + strconv.Itoa(frame.Line)
			t.Properties["caller"] = caller
		} else {
			// Fallback to hex representation if we can't resolve the frame
			t.Properties["pc"] = "0x" + strconv.FormatUint(uint64(record.PC), 16)
		}
	}

	// Reset the trace telemetry in encoder
	h.enc.setTraceTelemetry(t)

	// Process existing attributes from the handler
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)

	// Add attributes from the record
	record.Attrs(func(attr slog.Attr) bool {
		allAttrs = append(allAttrs, attr)
		return true
	})

	// Process all attributes
	for _, attr := range allAttrs {
		h.processAttr(t, attr, h.groups)
	}

	// Encode and write
	b, err := h.enc.encode(t)
	if err != nil {
		return errors.Wrap(err, "handler failed to encode trace")
	}
	if _, err = h.out.Write(b); err != nil {
		return errors.Wrap(err, "handler failed to write to sink")
	}
	return nil
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

// processAttr processes a single slog.Attr and adds it to the TraceTelemetry
func (h *Handler) processAttr(t *appinsights.TraceTelemetry, attr slog.Attr, groups []string) {
	key := attr.Key

	// Build the full key with group prefixes
	if len(groups) > 0 {
		groupPrefix := ""
		for _, group := range groups {
			if groupPrefix != "" {
				groupPrefix += "."
			}
			groupPrefix += group
		}
		key = groupPrefix + "." + key
	}

	// Check if this field should be redacted
	if h.redactFields[key] {
		t.Properties[key] = RedactedValue
		return
	}

	// Check if this field has a custom mapper
	if mapper, ok := h.fieldMappers[key]; ok {
		mapper(t, attrValueString(attr.Value))
		return
	}

	// Handle different value types
	switch attr.Value.Kind() {
	case slog.KindGroup:
		// For group values, process each attribute in the group
		groupAttrs := attr.Value.Group()
		newGroups := append(groups, attr.Key)
		for _, groupAttr := range groupAttrs {
			h.processAttr(t, groupAttr, newGroups)
		}
	default:
		// Add to properties
		t.Properties[key] = attrValueString(attr.Value)
	}
}

// clone creates a deep copy of the handler
func (h *Handler) clone() *Handler {
	fieldMappers := make(map[string]fieldTagMapper, len(h.fieldMappers))
	maps.Copy(fieldMappers, h.fieldMappers)
	redactFields := make(map[string]bool, len(h.redactFields))
	maps.Copy(redactFields, h.redactFields)
	attrs := make([]slog.Attr, len(h.attrs))
	copy(attrs, h.attrs)
	groups := make([]string, len(h.groups))
	copy(groups, h.groups)

	return &Handler{
		level:        h.level,
		enc:          newTraceEncoder(), // Create new encoder to avoid lock contention
		fieldMappers: fieldMappers,
		redactFields: redactFields,
		attrs:        attrs,
		groups:       groups,
		out:          h.out,
		lock:         &sync.Mutex{}, // Create new mutex for independent locking
	}
}

// attrValueString converts a slog.Value to its string representation
func attrValueString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.String()
	case slog.KindUint64:
		return v.String()
	case slog.KindFloat64:
		return v.String()
	case slog.KindBool:
		return v.String()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format("2006-01-02T15:04:05.000Z")
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.String()
	default:
		return v.String()
	}
}
