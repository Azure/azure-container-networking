package log

import (
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// globalMetadata holds metadata fields shared across all loggers built from initZapLog.
// It is set once from main() via SetMetadata after host metadata becomes available.
var globalMetadata atomic.Pointer[[]zapcore.Field]

// SetMetadata sets metadata fields that will be appended to every log entry
// written through CNILogger, IPamLogger, and TelemetryLogger (and any loggers
// derived from them). Safe to call once from main() after metadata is available.
func SetMetadata(fields ...zap.Field) {
	globalMetadata.Store(&fields)
}

// metadataCore is a zapcore.Core wrapper that appends shared metadata fields
// to every log entry at write time. All cores derived via With() share the
// same globalMetadata pointer, so setting metadata once propagates everywhere.
type metadataCore struct {
	zapcore.Core
}

func (c *metadataCore) With(fields []zapcore.Field) zapcore.Core {
	return &metadataCore{Core: c.Core.With(fields)}
}

// Check registers this wrapper (not the inner core) as the writer so that
// Write is called on metadataCore, allowing metadata fields to be appended.
func (c *metadataCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *metadataCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if extra := globalMetadata.Load(); extra != nil {
		fields = append(fields, *extra...)
	}
	if err := c.Core.Write(entry, fields); err != nil {
		return fmt.Errorf("writing log entry: %w", err)
	}
	return nil
}
