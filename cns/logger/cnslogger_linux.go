package logger

import (
	"go.uber.org/zap/zapcore"
)

func getPlatformCores(zapcore.Level, zapcore.Encoder) (zapcore.Core, error) {
	return zapcore.NewNopCore(), nil
}
