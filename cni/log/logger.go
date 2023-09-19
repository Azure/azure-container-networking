package log

import (
	"os"

	"github.com/Azure/azure-container-networking/zaplog"
	"go.uber.org/zap"
)

var CNILogger = zaplog.InitZapCNILog().With(zap.Int("pid", os.Getpid())).With(zap.String("component", "cni"))

func InitZapLogCNI(loggerName, loggerFile string) *zap.Logger {
	zaplog.LoggerCfg.Name = loggerName
	zaplog.LoggerCfg.LogPath = zaplog.LogPath + loggerFile
	logger := zaplog.InitZapLog(&zaplog.LoggerCfg)

	// only log process id on CNI package
	logger = logger.With(zap.Int("pid", os.Getpid()))
	logger = logger.With(zap.String("component", "cni"))
	return logger
}
