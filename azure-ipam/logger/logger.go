package logger

import (
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level            string
	OutputPaths      string // comma separated list of paths
	ErrorOutputPaths string // comma separated list of paths
}

// NewLogger creates and returns a zap logger and a clean up function
func New(cfg *Config) (*zap.Logger, func(), error) {
	loggerCfg := &zap.Config{}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse log level")
	}
	loggerCfg.Level = zap.NewAtomicLevelAt(level)

	loggerCfg.Encoding = "json"
	loggerCfg.OutputPaths = getLogOutputPath(cfg.OutputPaths)
	loggerCfg.ErrorOutputPaths = getErrOutputPath(cfg.ErrorOutputPaths)
	loggerCfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:     "time",
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
		EncodeTime:  zapcore.ISO8601TimeEncoder,
	}

	logger, err := loggerCfg.Build()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to build zap logger")
	}

	cleanup := func() {
		_ = logger.Sync()
	}
	return logger, cleanup, nil
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
