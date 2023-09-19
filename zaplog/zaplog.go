package zaplog

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

const (
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
)

var (
	loggerName string
	loggerFile string
)

var LoggerCfg = Config{
	Level:       zapcore.DebugLevel,
	LogPath:     loggerFile,
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        loggerName,
}

func InitZapLog(cfg *Config) *zap.Logger {
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
	return Logger
}
