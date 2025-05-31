package logger

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/internal/time"
	"github.com/Azure/azure-container-networking/zapai"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AppInsightsConfig struct {
	level            zapcore.Level   `json:"-"` // Zero value is default Info level.
	Level            string          `json:"level"`
	IKey             string          `json:"ikey"`
	GracePeriod      time.Duration   `json:"grace_period"`
	MaxBatchInterval time.Duration   `json:"max_batch_interval"`
	MaxBatchSize     int             `json:"max_batch_size"`
	Fields           []zapcore.Field `json:"fields"`
}

// UnmarshalJSON implements json.Unmarshaler for the Config.
// It only differs from the default by parsing the
// Level string into a zapcore.Level and setting the level field.
func (c *AppInsightsConfig) UnmarshalJSON(data []byte) error {
	type Alias AppInsightsConfig
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal AppInsightsConfig")
	}
	lvl, err := zapcore.ParseLevel(c.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse AppInsightsConfig Level")
	}
	c.level = lvl
	return nil
}

// ApplicationInsightsCore builds a zapcore.Core that sends logs to Application Insights.
// The first return is the core, the second is a function to close the sink.
func ApplicationInsightsCore(cfg *AppInsightsConfig) (zapcore.Core, func(), error) {
	// build the AI config
	aicfg := *appinsights.NewTelemetryConfiguration(cfg.IKey)
	aicfg.MaxBatchSize = cfg.MaxBatchSize
	aicfg.MaxBatchInterval = cfg.MaxBatchInterval.Duration
	sinkcfg := zapai.SinkConfig{
		GracePeriod:            cfg.GracePeriod.Duration,
		TelemetryConfiguration: aicfg,
	}
	// open the AI zap sink
	sink, aiclose, err := zap.Open(sinkcfg.URI())
	if err != nil {
		return nil, aiclose, errors.Wrap(err, "failed to open AI sink")
	}
	// build the AI core
	core := zapai.NewCore(cfg.level, sink)
	core = core.WithFieldMappers(zapai.DefaultMappers)
	// add normalized fields for the built-in AI Tags

	return core.With(cfg.Fields), aiclose, nil
}
