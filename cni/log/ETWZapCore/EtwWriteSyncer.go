//go:build windows
// +build windows

package ETWZapCore

import (
	"fmt"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/Microsoft/go-winio/pkg/guid"
	"go.uber.org/zap/zapcore"
)

const providername = "Azure-Container-Networking-CCP"

type EtwWriteSyncer struct {
	provider  *etw.Provider
	eventName string
	etwLevel  etw.Level
}

func etwEventCallback(sourceID guid.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	fmt.Printf("ETW Callback: isEnabled=%d, level=%d, matchAnyKeyword=%d\n", state, level, matchAnyKeyword)
}

func NewEtwWriteSyncer(eventName string, zapLevel zapcore.Level) (*EtwWriteSyncer, error) {

	provider, err := etw.NewProviderWithOptions(providername, etw.WithCallback(etwEventCallback))
	if err != nil {
		return nil, err
	}

	return &EtwWriteSyncer{
		provider:  provider,
		eventName: eventName,
		etwLevel:  mapZapLevelToETWLevel(zapLevel),
	}, nil
}

func (e *EtwWriteSyncer) Write(p []byte) (int, error) {

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
		return 0, err
	}
	return len(p), nil

}

// flush any buffered data to the underlying log destination,
// ensuring that all logged data is actually written out and not just held in memory.
func (e *EtwWriteSyncer) Sync() error {
	return nil
}

func mapZapLevelToETWLevel(zapLevel zapcore.Level) etw.Level {
	switch zapLevel {
	case zapcore.DebugLevel:
		return etw.LevelVerbose // ETW doesn't have a Debug level, so we use Verbose instead
	case zapcore.InfoLevel:
		return etw.LevelInfo
	case zapcore.WarnLevel:
		return etw.LevelWarning
	case zapcore.ErrorLevel:
		return etw.LevelError
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return etw.LevelCritical
	default:
		return etw.LevelAlways
	}
}
