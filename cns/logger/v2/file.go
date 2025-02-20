package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type FileConfig struct {
	Filepath   string
	Level      zapcore.Level
	MaxBackups int
	MaxSize    int
}

// FileCore builds a zapcore.Core that writes to a file.
// The first return is the core, the second is a function to close the file.
func FileCore(cfg *FileConfig) (zapcore.Core, func(), error) {
	filesink := &lumberjack.Logger{
		Filename:   cfg.Filepath,
		MaxSize:    cfg.MaxSize, // MegaBytes
		MaxBackups: cfg.MaxBackups,
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	return zapcore.NewCore(jsonEncoder, zapcore.AddSync(filesink), cfg.Level), func() { _ = filesink.Close() }, nil
}
