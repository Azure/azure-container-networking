package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	zapCNILogFile       = "azure-vnet.log"
	zapIpamLogFile      = "azure-vnet-ipam.log"
	zapTelemetryLogFile = "azure-vnet-telemetry.log"
)

const (
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
	ETWCNIEventName    = "Azure-CNI"
	loggingLevel       = zapcore.DebugLevel
)

func initZapLog(logFile string) *zap.Logger {
	logFileCNIWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   LogPath + logFile,
		MaxSize:    maxLogFileSizeInMb,
		MaxBackups: maxLogFileCount,
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	textfilecore := zapcore.NewCore(jsonEncoder, logFileCNIWriter, loggingLevel)
	Logger := zap.New(textfilecore, zap.AddCaller())
	return Logger.With(zap.Int("pid", os.Getpid()))
}

func initZapLogWithETW(logFile string) *zap.Logger {
	Logger := initZapLog(logFile)
	etwcore, err := GetETWCore(ETWCNIEventName, loggingLevel)
	if err != nil {
		Logger.Error("Failed to attach ETW core to logger", zap.Error(err))
		return Logger
	}
	teecore := zapcore.NewTee(Logger.Core(), etwcore)
	Logger = zap.New(teecore, zap.AddCaller())
	return Logger.With(zap.Int("pid", os.Getpid()))
}

var (
	CNILogger       = initZapLogWithETW(zapCNILogFile)
	IPamLogger      = initZapLogWithETW(zapIpamLogFile)
	TelemetryLogger = initZapLog(zapTelemetryLogFile)
)
