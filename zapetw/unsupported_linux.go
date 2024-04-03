package zapetw

import (
	"errors"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ETWWriteSyncer struct{}

var ErrETWNotSupported = errors.New("ETW is not supported for Linux")

func NewETWWriteSyncer(_ string, _ zapcore.Level) (*ETWWriteSyncer, error) {
	return nil, ErrETWNotSupported
}

func (e *ETWWriteSyncer) Write(_ []byte) (int, error) {
	return 0, ErrETWNotSupported
}

func InitETWLogger(baseLogger *zap.Logger, _ string, _ zapcore.Level) (*zap.Logger, error) {
	return baseLogger, ErrETWNotSupported
}

func AttachETWLogger(baseLogger *zap.Logger, _ string, _ zapcore.Level) (*zap.Logger, error) {
	return baseLogger, ErrETWNotSupported
}
