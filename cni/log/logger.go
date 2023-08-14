package log

import (
	"context"
	"fmt"
	"os"

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
	Component   string
}

var Logger *zap.Logger

// Initializes a Zap logger and returns a cleanup function so logger can be cleaned up from caller
func Initialize(ctx context.Context, cfg *Config) {
	Logger = newFileLogger(cfg)

	go func() {
		<-ctx.Done()
		err := Logger.Sync()
		if err != nil {
			fmt.Println("failed to sync logger")
		}
	}()
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
	Logger = Logger.With(zap.Int("pid", os.Getpid()))
	Logger = Logger.With(zap.String("component", cfg.Component))

	return Logger
}
