package log

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
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

// InitHcnshimFileLogger creates (if missing) and sets a dedicated log file
// for all hcnshim/hcsshim logs (logrus global logger).
func initHcnshimFileLogger(path string) string {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Println("Failed to create log dir:", err)
		return ""
	}

	// Open file for append (create if missing)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		return ""
	}

	// Configure global logrus (used by hcsshim)
	logrus.SetOutput(f)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	logrus.SetReportCaller(true)       // include file:line
	logrus.SetLevel(logrus.DebugLevel) // verbose for testing

	fmt.Println("HCN shim logs redirected to:", path)
	return "success"
}

var (
	CNILogger       = initZapLog(zapCNILogFile)
	IPamLogger      = initZapLog(zapIpamLogFile)
	TelemetryLogger = initZapLog(zapTelemetryLogFile)
	logrusLogger    = initHcnshimFileLogger(`C:\hcnshim.log`)
)
