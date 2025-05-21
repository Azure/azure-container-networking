package logger

import (
	"go.uber.org/zap/zapcore"
)

// platformCore returns a no-op core for Linux.
func platformCore(*Config) (zapcore.Core, func(), error) {
	return zapcore.NewNopCore(), func() {}, nil
}
