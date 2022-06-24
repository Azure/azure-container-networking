package main

import (
	"strings"

	"github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates and returns a zap logger and a clean up function
func NewLogger() (*zap.Logger, func(_ *zap.Logger) error, error) {
	loggerCfg := &zap.Config{}

	level, err := zapcore.ParseLevel(buildinfo.LogLevel)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse log level")
	}
	loggerCfg.Level = zap.NewAtomicLevelAt(level)

	loggerCfg.Encoding = "json"
	loggerCfg.OutputPaths = getLogOutputPath(buildinfo.OutputPaths)
	loggerCfg.ErrorOutputPaths = getErrOutputPath(buildinfo.ErrorOutputPaths)
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

func getLogOutputPath(paths string) []string {
	if paths == "" {
		return nil
	}
	return strings.Split(paths, ",")
}

func getErrOutputPath(paths string) []string {
	if paths == "" {
		return nil
	}
	return strings.Split(paths, ",")
}
