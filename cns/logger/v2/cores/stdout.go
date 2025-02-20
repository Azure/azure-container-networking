package logger

import (
	"os"

	logfmt "github.com/jsternberg/zap-logfmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// StdoutCore builds a zapcore.Core that writes to stdout.
func StdoutCore(l zapcore.Level) zapcore.Core {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewCore(logfmt.NewEncoder(encoderConfig), os.Stdout, l)
}
