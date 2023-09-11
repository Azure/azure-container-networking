package logger

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	Filepath = "/var/log/azure-ipam.log"
)

type Config struct {
	Level       string // Debug by default
	Filepath    string // default /var/log/azure-ipam.log
	MaxSizeInMB int    // MegaBytes
	MaxBackups  int    // # of backups, no limitation by default
}

// NewLogger creates and returns a zap logger and a clean up function
func New(cfg *Config) (*zap.Logger, func(), error) {
	logLevel, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse log level")
	}
	if cfg.Filepath == "" {
		cfg.Filepath = Filepath
	}
	logger := newFileLogger(cfg, logLevel)
	cleanup := func() {
		_ = logger.Sync()
	}
	return logger, cleanup, nil
}

// create and return a zap logger via lumberjack with rotation
func newFileLogger(cfg *Config, logLevel zapcore.Level) *zap.Logger {
	// define a lumberjack fileWriter
	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.Filepath,
		MaxSize:    cfg.MaxSizeInMB, // MegaBytes
		MaxBackups: cfg.MaxBackups,
	})
	// define the log encoding
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	// create a new zap logger
	core := zapcore.NewCore(jsonEncoder, logFileWriter, logLevel)
	logger := zap.New(core)
	return logger
}
