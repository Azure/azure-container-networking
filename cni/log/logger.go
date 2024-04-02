package log

import (
	"os"
	"runtime"

	"github.com/Azure/azure-container-networking/zapetw"
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
	etwCNIEventName    = "Azure-CNI"
)

func initZapLog(logFile string, isEtwLoggingEnabled bool) *zap.Logger {
	logFileCNIWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   LogPath + logFile,
		MaxSize:    maxLogFileSizeInMb,
		MaxBackups: maxLogFileCount,
	})

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	loggingLevel := zapcore.DebugLevel

	textfilecore := zapcore.NewCore(jsonEncoder, logFileCNIWriter, loggingLevel)
	Logger := zap.New(textfilecore, zap.AddCaller())

	// Initialize ETW logger
	if isEtwLoggingEnabled && runtime.GOOS == "windows" {
		etwSyncer, err := zapetw.NewEtwWriteSyncer(etwCNIEventName, loggingLevel)
		if err != nil {
			Logger.Warn("Failed to initialize ETW logger.", zap.Error(err))
		} else {
			etwcore := zapcore.NewCore(jsonEncoder, zapcore.AddSync(etwSyncer), loggingLevel)
			teecore := zapcore.NewTee(textfilecore, etwcore)
			Logger = zap.New(teecore, zap.AddCaller())
		}
	}
	return Logger.With(zap.Int("pid", os.Getpid()))
}

var (
	CNILogger       = initZapLog(zapCNILogFile, true)
	IPamLogger      = initZapLog(zapIpamLogFile, true)
	TelemetryLogger = initZapLog(zapTelemetryLogFile, false)
)
