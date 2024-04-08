package log

import (
	"errors"

	"go.uber.org/zap/zapcore"
)

const (
	// LogPath is the path where log files are stored.
	LogPath = "/var/log/"
)

func GetETWCore(eventName string, loggingLevel zapcore.Level) (zapcore.Core, error) {
	return nil, errors.New("ETW is not supported for Linux")
}
