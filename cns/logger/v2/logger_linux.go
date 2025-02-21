package logger

import (
	"go.uber.org/zap/zapcore"
)

const defaultFilePath = "/var/log/azure-cns.log"

// platformCore returns a no-op core for Linux.
func platformCore(*Config) (zapcore.Core, func(), error) {
	return zapcore.NewNopCore(), func() {}, nil
}
