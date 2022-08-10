package logger

import (
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level       string // Debug by default
	Filename   	string
	MaxSize    	int    // megabytes
	MaxBackups 	int    // # of backups, no limitation by default
	MaxAge     	int    // days, no limitation by default
}

// NewLogger creates and returns a zap logger and a clean up function
func New(cfg *Config) (*zap.Logger, func(), error) {
	//check the filepath is not empty
	if cfg.Filename == "" {
		err := errors.New("no Filename is provided")
		return nil, nil, errors.Wrapf(err, "failed to build zap logger")
	}
	// set the log level - Debug by default
	var logLevel zapcore.Level
	if strings.ToLower(cfg.Level) == "info" {
		logLevel = zap.InfoLevel
	} else if strings.ToLower(cfg.Level) == "error" {
		logLevel = zap.ErrorLevel
	} else {
		logLevel = zap.DebugLevel
	}
	//define a lumberjack fileWriter
	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize, // megabytes
	})
	//define the log encoding
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	//create a new zap logger
	core := zapcore.NewCore(jsonEncoder, logFileWriter, logLevel)
	logger := zap.New(core)
	cleanup := func() {
		_ = logger.Sync()
	}

	return logger, cleanup, nil
}