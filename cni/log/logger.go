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
	etwCNIEventName    = "AzureCNI"
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

	textFileCore := zapcore.NewCore(jsonEncoder, logFileCNIWriter, loggingLevel)
	core, err := JoinPlatformCores(textFileCore, loggingLevel)
	if err != nil {
		// If we fail to join the platform cores, fallback to the original core.
		core = textFileCore
	}
	return zap.New(core, zap.AddCaller()).With(zap.Int("pid", os.Getpid()))
}

var (
	CNILogger       = initZapLog(zapCNILogFile)
	IPamLogger      = initZapLog(zapIpamLogFile)
	TelemetryLogger = initZapLog(zapTelemetryLogFile)
)
