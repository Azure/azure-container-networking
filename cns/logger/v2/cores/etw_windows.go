package logger

import (
	"github.com/Azure/azure-container-networking/zapetw"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ETWConfig struct {
	EventName    string
	Level        zapcore.Level
	ProviderName string
}

// ETWCore builds a zapcore.Core that sends logs to ETW.
// The first return is the core, the second is a function to close the sink.
func ETWCore(cfg *ETWConfig) (zapcore.Core, func(), error) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	return zapetw.New(cfg.ProviderName, cfg.EventName, jsonEncoder, cfg.Level) //nolint:wrapcheck // ignore
}
