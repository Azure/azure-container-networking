package main

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates and returns a zap logger and a clean up function
func NewLogger(buildEnv Env) (*zap.Logger, func(_ *zap.Logger) error, error) {
	loggerCfg := &zap.Config{}
	loggerCfg.Level = zap.NewAtomicLevelAt(getLogLevel(buildEnv))
	loggerCfg.Encoding = "json"
	loggerCfg.OutputPaths = getLogOutputPath(buildEnv)
	loggerCfg.ErrorOutputPaths = getErrOutputPath(buildEnv)
	loggerCfg.EncoderConfig = zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
	}

	logger, err := loggerCfg.Build()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to build zap logger")
	}
	return logger, CleanUpLog, nil
}

// CleanUpLog flushes the given logger's buffered log entries
func CleanUpLog(logger *zap.Logger) error {
	err := logger.Sync()
	return errors.Wrapf(err, "failed to flush log")
}

// getLogLevel return zap's log level based on the env the ipam is built in
func getLogLevel(buildEnv Env) zapcore.Level {
	switch buildEnv {
	case Prod:
		return zapcore.InfoLevel
	case Dev:
		return zapcore.DebugLevel
	case Test:
		return zapcore.DebugLevel
	default:
		return zapcore.DebugLevel
	}
}

// getLogLevel return logs output paths based on the env the ipam is built in
func getLogOutputPath(buildEnv Env) []string {
	switch buildEnv {
	case Prod:
		return []string{"var/logs"}
	case Dev:
		return []string{"stdout"}
	case Test:
		return []string{}
	default:
		return []string{}
	}
}

// getErrOutputPath return logs output paths based on the env the ipam is built in
func getErrOutputPath(buildEnv Env) []string {
	switch buildEnv {
	case Prod:
		return []string{"var/logs"}
	case Dev:
		return []string{"stderr"}
	case Test:
		return []string{}
	default:
		return []string{}
	}
}
