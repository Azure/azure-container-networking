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
)

func initZapCNILog(logFile string) *zap.Logger {
	logFileCNIWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   LogPath + logFile,
		MaxSize:    maxLogFileSizeInMb,
		MaxBackups: maxLogFileCount,
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	core := zapcore.NewCore(jsonEncoder, logFileCNIWriter, zapcore.DebugLevel)
	Logger := zap.New(core)
	return Logger
}

var (
	CNILogger       = initZapCNILog(zapCNILogFile).With(zap.Int("pid", os.Getpid()))
	IPamLogger      = initZapCNILog(zapIpamLogFile).With(zap.Int("pid", os.Getpid()))
	TelemetryLogger = initZapCNILog(zapTelemetryLogFile).With(zap.Int("pid", os.Getpid()))
)
