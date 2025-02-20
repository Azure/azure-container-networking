package logger

import (
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/zapai"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AIConfig struct {
	GracePeriod      time.Duration
	IKey             string
	Level            zapcore.Level
	MaxBatchInterval time.Duration
	MaxBatchSize     int
}

// ApplicationInsightsCore builds a zapcore.Core that sends logs to Application Insights.
// The first return is the core, the second is a function to close the sink.
func ApplicationInsightsCore(cfg *AIConfig) (zapcore.Core, func(), error) {
	// build the AI config
	aicfg := *appinsights.NewTelemetryConfiguration(cfg.IKey)
	aicfg.MaxBatchSize = cfg.MaxBatchSize
	aicfg.MaxBatchInterval = cfg.MaxBatchInterval
	sinkcfg := zapai.SinkConfig{
		GracePeriod:            cfg.GracePeriod,
		TelemetryConfiguration: aicfg,
	}
	// open the AI zap sink
	sink, aiclose, err := zap.Open(sinkcfg.URI())
	if err != nil {
		return nil, aiclose, errors.Wrap(err, "failed to open AI sink")
	}
	// build the AI core
	core := zapai.NewCore(cfg.Level, sink)
	core = core.WithFieldMappers(zapai.DefaultMappers)
	// add normalized fields for the built-in AI Tags
	// TODO(rbtr): move to the caller
	return core.With([]zapcore.Field{
		zap.String("user_id", runtime.GOOS),
		zap.String("operation_id", ""),
		zap.String("parent_id", "v0.0.0"),
		zap.String("version", "v0.0.0"),
		zap.String("account", "SubscriptionID"),
		zap.String("anonymous_user_id", "VMName"),
		zap.String("session_id", "VMID"),
		zap.String("AppName", "name"),
		zap.String("Region", "Location"),
		zap.String("ResourceGroup", "ResourceGroupName"),
		zap.String("VMSize", "VMSize"),
		zap.String("OSVersion", "OSVersion"),
		zap.String("VMID", "VMID"),
	}), aiclose, nil
}
