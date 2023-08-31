package log

import (
	"os"

	"github.com/Azure/azure-container-networking/zaplog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
)

var (
	LoggerName string
	LoggerFile string
	LogLevel   int8
)

var LoggerCfg = &zaplog.Config{
	Level:       zapcore.DebugLevel,
	LogPath:     LoggerFile,
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        LoggerName,
}

func InitZapLogCNI(LoggerName, LoggerFile string) *zap.Logger {
	LoggerCfg.Name = LoggerName
	LoggerCfg.LogPath = LogPath + LoggerFile
	logger := zaplog.InitZapLog(LoggerCfg)

	// only log process id on CNI package
	logger = logger.With(zap.Int("pid", os.Getpid()))
	return logger
}
