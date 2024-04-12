package zapetw

import (
	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

// <product_name>-<component_name>
const providername = "ACN-Monitoring"

type ETWCore struct {
	provider  *etw.Provider
	eventName string
	encoder   zapcore.Encoder
	fields    []zapcore.Field
	zapcore.LevelEnabler
}

func NewETWCore(eventName string, encoder zapcore.Encoder, levelEnabler zapcore.LevelEnabler) (*ETWCore, error) {
	provider, err := etw.NewProviderWithOptions(providername)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ETW provider")
	}
	return &ETWCore{
		provider:     provider,
		eventName:    eventName,
		encoder:      encoder,
		LevelEnabler: levelEnabler,
	}, nil
}

func (core *ETWCore) With(fields []zapcore.Field) zapcore.Core {
	return &ETWCore{
		provider:     core.provider,
		eventName:    core.eventName,
		encoder:      core.encoder,
		LevelEnabler: core.LevelEnabler,
		fields:       append(core.fields, fields...),
	}
}

// Check is an implementation of the zapcore.Core interface's Check method.
// Check determines whether the logger core is enabled at the supplied zapcore.Entry's Level.
// If enabled, it adds the core to the CheckedEntry and returns it, otherwise returns the CheckedEntry unchanged.
func (core *ETWCore) Check(entry zapcore.Entry, checkedEntry *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checkedEntry.AddCore(entry, core)
	}
	return checkedEntry
}

func (core *ETWCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	etwLevel := zapLevelToETWLevel(entry.Level)

	buffer, err := core.encoder.EncodeEntry(entry, fields)
	if err != nil {
		return errors.Wrap(err, "failed to encode entry")
	}

	err = core.provider.WriteEvent(
		core.eventName,
		[]etw.EventOpt{etw.WithLevel(etwLevel)},
		[]etw.FieldOpt{etw.StringField("Message", buffer.String())},
	)
	if err != nil {
		return errors.Wrap(err, "failed to write event")
	}

	return nil
}

func (core *ETWCore) Sync() error {
	return nil
}

func zapLevelToETWLevel(level zapcore.Level) etw.Level {
	switch level {
	case zapcore.DebugLevel:
		return etw.LevelVerbose // ETW doesn't have a Debug level, so Verbose is used instead.
	case zapcore.InfoLevel:
		return etw.LevelInfo
	case zapcore.WarnLevel:
		return etw.LevelWarning
	case zapcore.ErrorLevel:
		return etw.LevelError
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel, zapcore.InvalidLevel:
		return etw.LevelCritical
	default:
		return etw.LevelAlways
	}
}
