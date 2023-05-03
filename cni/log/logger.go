package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level       zapcore.Level
	LogPath     string
	MaxSizeInMB int
	MaxBackups  int
	Name        string
}

var Logger *zap.Logger

func New(cfg *Config) (func(), error) {
	Logger = newFileLogger(cfg)
	cleanup := func() {
		_ = Logger.Sync()
	}

	return cleanup, nil
}

func newFileLogger(cfg *Config) *zap.Logger {
	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxSize:    cfg.MaxSizeInMB,
		MaxBackups: cfg.MaxBackups,
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	logLevel := cfg.Level

	core := zapcore.NewCore(jsonEncoder, logFileWriter, logLevel)
	Logger = zap.New(core)

	return Logger.Named(cfg.Name)
}
