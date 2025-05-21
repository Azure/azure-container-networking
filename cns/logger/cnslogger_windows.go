package logger

import (
	"github.com/Azure/azure-container-networking/zapetw"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	etwCNSEventName = "AzureCNS"
)

func getPlatformCores(loggingLevel zapcore.Level, encoder zapcore.Encoder) (zapcore.Core, error) {
	etwcore, err := getETWCore(loggingLevel, encoder)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ETW core")
	}
	return etwcore, nil
}

func getETWCore(loggingLevel zapcore.Level, encoder zapcore.Encoder) (zapcore.Core, error) {
	etwcore, _, err := zapetw.New("ACN-Monitoring", etwCNSEventName, encoder, loggingLevel)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ETW core")
	}
	return etwcore, nil
}
