/*
Package zapetw provides a stub implementation of ETW (Event Tracing for Windows)
logging functionality for Linux environments.
Since ETW is specific to Windows, this package returns errors indicating the lack of support on Linux.
*/
package zapetw

import (
	"errors"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

/*
ETWWriteSyncer is a no-op structure implementing a zapcore.WriteSyncer interface for Linux.
It provides stub methods that simply return an error indicating that ETW is not supported on Linux.
*/
type ETWWriteSyncer struct{}

var ErrETWNotSupported = errors.New("ETW is not supported for Linux")

func NewETWWriteSyncer(_ string, _ zapcore.Level) (*ETWWriteSyncer, error) {
	return nil, ErrETWNotSupported
}

func (e *ETWWriteSyncer) Write(_ []byte) (int, error) {
	return 0, ErrETWNotSupported
}

func InitETWLogger(_ string, _ zapcore.Level) (*zap.Logger, error) {
	return nil, ErrETWNotSupported
}

func AttachETWLogger(baseLogger *zap.Logger, _ string, _ zapcore.Level) (*zap.Logger, error) {
	return baseLogger, ErrETWNotSupported
}
