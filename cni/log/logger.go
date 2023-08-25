package log

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/zaplog"
	"go.uber.org/zap/zapcore"
)

const (
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
)

var (
	LoggerVnetName      string
	LoggerIpamName      string
	LoggerTelemetryName string
	LogLevel            int8
)

var LoggerVnetCfg = &zaplog.Config{
	Level:       zapcore.DebugLevel,
	LogPath:     log.LogPath + "azure-vnet.log",
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        LoggerVnetName,
}

var LoggerIpamCfg = &zaplog.Config{
	Level:       zapcore.DebugLevel,
	LogPath:     log.LogPath + "azure-ipam.log",
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        LoggerIpamName,
}

var LoggerTelemetryCfg = &zaplog.Config{
	Level:       zapcore.Level(zapcore.Int8Type),
	LogPath:     log.LogPath + "azure-vnet-telemetry.log",
	MaxSizeInMB: maxLogFileSizeInMb,
	MaxBackups:  maxLogFileCount,
	Name:        LoggerTelemetryName,
}
