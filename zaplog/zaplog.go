package zaplog

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var ZapDefaultLogger = InitLog(&Config{})

type Config struct {
	Level       zapcore.Level
	LogPath     string
	MaxSizeInMB int
	MaxBackups  int
	Name        string
}

func InitLog(cfg *Config) *zap.Logger {
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
	Logger := zap.New(core)
	Logger = Logger.With(zap.Int("pid", os.Getpid()))
	return Logger
}
