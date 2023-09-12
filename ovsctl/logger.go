package ovsctl

import (
	"github.com/Azure/azure-container-networking/zaplog"
	"go.uber.org/zap"
)

func InitZapLogOVS(loggerName, loggerFile string) *zap.Logger {
	zaplog.LoggerCfg.Name = loggerName
	zaplog.LoggerCfg.LogPath = zaplog.LogPath + loggerFile
	logger := zaplog.InitZapLog(&zaplog.LoggerCfg)

	return logger
}
