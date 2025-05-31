package logger

import (
	"go.uber.org/zap/zapcore"
)

// On Linux, platformCore returns a no-op core.
func platformCore(*Config) (zapcore.Core, func(), error) {
	return zapcore.NewNopCore(), func() {}, nil
}
