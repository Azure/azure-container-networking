package log

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level       string
	Filepath    string
	MaxSizeInMB int
	MaxBackups  int
	Name        string
}

var Logger *zap.Logger

func New(cfg *Config) (func(), error) {

	logLevel, err := zapcore.ParseLevel(cfg.Level)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse log level")
	}

	Logger = newFileLogger(cfg, logLevel)
	cleanup := func() {
		_ = Logger.Sync()
	}

	return cleanup, nil
}

func newFileLogger(cfg *Config, logLevel zapcore.Level) *zap.Logger {

	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.Filepath,
		MaxSize:    cfg.MaxSizeInMB,
		MaxBackups: cfg.MaxBackups,
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	core := zapcore.NewCore(jsonEncoder, logFileWriter, logLevel)
	Logger = zap.New(core)

	return Logger.Named(cfg.Name)
}
