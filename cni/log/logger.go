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
	loggerName string
	loggerFile string
)

var LoggerCfg = &zaplog.Config{
	Level:       zapcore.DebugLevel,
	LogPath:     loggerFile,
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        loggerName,
}

func InitZapLogCNI(loggerName, loggerFile string) *zap.Logger {
	LoggerCfg.Name = loggerName
	LoggerCfg.LogPath = LogPath + loggerFile
	logger := zaplog.InitZapLog(LoggerCfg)

	// only log process id on CNI package
	logger = logger.With(zap.Int("pid", os.Getpid()))
	logger = logger.With(zap.String("component", "cni"))
	return logger
}
