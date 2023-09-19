package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
)

var logFileCNIWriter = zapcore.AddSync(&lumberjack.Logger{
	Filename:   LogPath + "azure-vnet.log",
	MaxSize:    maxLogFileSizeInMb,
	MaxBackups: maxLogFileCount,
})

func initZapCNILog() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	logLevel := zapcore.DebugLevel

	core := zapcore.NewCore(jsonEncoder, logFileCNIWriter, logLevel)
	Logger := zap.New(core)
	return Logger
}

var CNILogger = initZapCNILog().With(zap.Int("pid", os.Getpid()))
