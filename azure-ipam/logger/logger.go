package logger

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Level           string // Debug by default
	Filepath        string // if Empty log into os.Stderr
	MaxSizeInMB     int    // MegaBytes
	MaxBackups      int    // # of backups, no limitation by default
}

// NewLogger creates and returns a zap logger and a clean up function
func New(cfg *Config) (*zap.Logger, func(), error) {
	logLevel, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse log level")
	}
	var logger *zap.Logger
	if cfg.Filepath == "" {
		logger, err = newStdLogger(cfg, logLevel)
	} else {
		logger = newFileLogger(cfg, logLevel)
	}

	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to build zap logger")
	}
	cleanup := func() {
		_ = logger.Sync()
	}
	return logger, cleanup, nil
}

// creates and returns a zap logger that is wrting to os.Stderr
func newStdLogger(cfg *Config, logLevel zapcore.Level) (*zap.Logger, error) {
	loggerCfg := &zap.Config{}
	loggerCfg.Level = zap.NewAtomicLevelAt(logLevel)
	loggerCfg.Encoding = "json"
	loggerCfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:     "time",
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
		EncodeTime:  zapcore.ISO8601TimeEncoder,
	}
	logger, err := loggerCfg.Build()
	return logger, err
}

// create and return a zap logger via lumbejack with rotation
func newFileLogger(cfg *Config, logLevel zapcore.Level) (*zap.Logger) {
	// define a lumberjack fileWriter
	logFileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:    cfg.Filepath,
		MaxSize:     cfg.MaxSizeInMB, // MegaBytes
		MaxBackups:  cfg.MaxBackups,
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
