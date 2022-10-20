package zapai

import (
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"sync"
)

var levelToSev = map[zapcore.Level]contracts.SeverityLevel{
	zapcore.DebugLevel:  contracts.Verbose,
	zapcore.InfoLevel:   contracts.Information,
	zapcore.WarnLevel:   contracts.Warning,
	zapcore.ErrorLevel:  contracts.Error,
	zapcore.DPanicLevel: contracts.Critical,
	zapcore.PanicLevel:  contracts.Critical,
	zapcore.FatalLevel:  contracts.Critical,
}

var _ zapcore.Core = (*Core)(nil)

// Core implements zapcore.Core for appinsights.
// Core only knows how to write to, and should only be used with, a corresponding appinsights Sink.
//
// Internally, the Core implements Write(zapcore.Entry, []zapcore.Field) by building an appinsights.TraceTelemetry
// out of the zap inputs, then serializing it to a []byte using a shared common gob.Encoder, and passes that to
// the Sink.Write([]byte) (which knows how to decode it back in to a TraceTelemetry).
type Core struct {
	zapcore.LevelEnabler
	enc          traceEncoder
	fieldMappers map[string]fieldTagMapper
	fields       []zapcore.Field
	out          zapcore.WriteSyncer
	lock         *sync.Mutex
}

// NewCore creates a new appinsights zap core. Should only be initialized using an appinsights Sink as the
// zapcore.WriteSyncer argument - other sinks will not error but will also not produce meaningful output.
func NewCore(le zapcore.LevelEnabler, out zapcore.WriteSyncer) *Core {
	return &Core{
		LevelEnabler: le,
		enc:          newTraceEncoder(),
		fieldMappers: make(map[string]fieldTagMapper),
		out:          out,
		lock:         &sync.Mutex{},
	}
}

func (c *Core) WithFieldMappers(fieldMappers ...map[string]string) *Core {
	clone := c.clone()
	for _, fieldMapper := range fieldMappers {
		for field, tag := range fieldMapper {
			tag := tag // anchor tag var in the closure
			clone.fieldMappers[field] = func(t *appinsights.TraceTelemetry, val string) {
				// set the tag to the value of the field
				t.Tags[tag] = val
			}
		}
	}
	return clone
}

func (c *Core) With(fields []zapcore.Field) zapcore.Core {
	clone := c.clone()
	clone.fields = append(clone.fields, fields...)
	return clone
}

// Check implements zapcore.Core
//nolint:gocritic // ignore hugeparam in interface impl
func (c *Core) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

// Write implements zapcore.Core
//nolint:gocritic // ignore hugeparam in interface impl
func (c *Core) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	t := appinsights.NewTraceTelemetry(entry.Message, levelToSev[entry.Level])

	// add fields from core
	fields = append(c.fields, fields...)

	// reset the traceTelemetry in encoder
	c.enc.setTraceTelemetry(t)

	// set caller
	if entry.Caller.Defined {
		t.Properties["caller"] = entry.Caller.String()
	}

	// set fields
	for i := range fields {
		// handle zap object first
		if fields[i].Type == zapcore.ObjectMarshalerType {
			fields[i].AddTo(c.enc)
		} else if mapper, ok := c.fieldMappers[fields[i].Key]; ok {
			// check mapped fields
			mapper(t, fieldStringer(&fields[i]))
		} else {
			t.Properties[fields[i].Key] = fieldStringer(&fields[i])
		}
	}
	b, err := c.enc.encode(t)
	if err != nil {
		return errors.Wrap(err, "core failed to encode trace")
	}
	if _, err = c.out.Write(b); err != nil {
		return errors.Wrap(err, "core failed to write to sink")
	}
	return nil
}

func (c *Core) Sync() error {
	return errors.Wrap(c.out.Sync(), "core failed to sync")
}

// clone derives a new Core from this Core, copying the references to the encoder and sink
// and duplicating the contents of the fields and fieldMappers so that children and parent
// cores may mutate their fields and mappers independently after cloning.
func (c *Core) clone() *Core {
	fieldMappers := make(map[string]fieldTagMapper, len(c.fieldMappers))
	for k, v := range c.fieldMappers {
		fieldMappers[k] = v
	}
	fields := make([]zapcore.Field, len(c.fields))
	copy(fields, c.fields)
	return &Core{
		LevelEnabler: c.LevelEnabler,
		enc:          c.enc,
		fieldMappers: fieldMappers,
		fields:       fields,
		out:          c.out,
		lock:         c.lock,
	}
}
