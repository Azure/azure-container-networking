package log

import (
	"errors"

	"go.uber.org/zap/zapcore"
)

const (
	// LogPath is the path where log files are stored.
	LogPath = "/var/log/"
)

var ErrETWNotSupported = errors.New("ETW is not supported for Linux")

func GetETWCore(_ string, _ zapcore.Level) (zapcore.Core, error) {
	return nil, ErrETWNotSupported
}
