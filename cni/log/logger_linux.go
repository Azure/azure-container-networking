package log

import (
	"go.uber.org/zap/zapcore"
)

const (
	// LogPath is the path where log files are stored.
	LogPath = "/var/log/"
)

func JoinPlatformCores(c zapcore.Core, _ zapcore.Level) (zapcore.Core, error) {
	return c, nil
}
