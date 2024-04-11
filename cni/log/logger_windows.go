package log

import (
	"github.com/Azure/azure-container-networking/zapetw"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// LogPath is the path where log files are stored.
	LogPath = ""
)

func JoinPlatformCores(core zapcore.Core, loggingLevel zapcore.Level) (zapcore.Core, error) {
	etwcore, err := etwCore(loggingLevel)
	if err != nil {
		return core, err
	}
	teecore := zapcore.NewTee(core, etwcore)
	return teecore, nil
}

func etwCore(loggingLevel zapcore.Level) (zapcore.Core, error) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	etwcore, err := zapetw.NewETWCore(etwCNIEventName, jsonEncoder, loggingLevel)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ETW core")
	}
	return etwcore, nil
}
