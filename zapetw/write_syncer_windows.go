package zapetw

import (
	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const providername = "ACN-Data-Plane"

type ETWWriteSyncer struct {
	provider  *etw.Provider
	eventName string
	etwLevel  etw.Level
}

func NewETWWriteSyncer(eventName string, zapLevel zapcore.Level) (*ETWWriteSyncer, error) {
	provider, err := etw.NewProviderWithOptions(providername)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ETW provider")
	}

	return &ETWWriteSyncer{
		provider:  provider,
		eventName: eventName,
		etwLevel:  mapZapLevelToETWLevel(zapLevel),
	}, nil
}

func (e *ETWWriteSyncer) Write(p []byte) (int, error) {
	err := e.provider.WriteEvent(
		e.eventName,
		etw.WithEventOpts(
			etw.WithLevel(e.etwLevel),
		),
		[]etw.FieldOpt{
			etw.StringField("Message", string(p)),
		},
	)
	if err != nil {
		return 0, errors.Wrap(err, "failed to write to ETW")
	}
	return len(p), nil
}

// flush any buffered data to the underlying log destination,
// ensuring that all logged data is actually written out and not just held in memory.
func (e *ETWWriteSyncer) Sync() error {
	return nil
}

func mapZapLevelToETWLevel(zapLevel zapcore.Level) etw.Level {
	switch zapLevel {
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
