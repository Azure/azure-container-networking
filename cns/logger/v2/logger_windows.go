package logger

import (
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap/zapcore"
)

// platformCore returns a zapcore.Core that sends logs to ETW.
func platformCore(cfg *Config) (zapcore.Core, func(), error) {
	return cores.ETWCore(&cores.ETWConfig{ //nolint:wrapcheck // ignore
		EventName:    "AzureCNS",
		Level:        cfg.Level,
		ProviderName: "ACN-Monitoring",
	})
}
