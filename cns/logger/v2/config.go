package logger

import (
	"encoding/json"

	loggerv1 "github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/internal/time"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

//nolint:unused // will be used
const (
	defaultMaxBackups       = 10
	defaultMaxSize          = 10 // MB
	defaultMaxBatchInterval = 30 * time.Second
	defaultMaxBatchSize     = 32000
	defaultGracePeriod      = 30 * time.Second
)

//nolint:unused // will be used
var defaultIKey = loggerv1.AppInsightsIKey

// UnmarshalJSON implements json.Unmarshaler for the Config.
// It only differs from the default by parsing the
// Level string into a zapcore.Level and setting the level field.
func (c *Config) UnmarshalJSON(data []byte) error {
	type Alias Config
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil { //nolint:musttag // doesn't understand the embedding strategy
		return errors.Wrap(err, "failed to unmarshal Config")
	}
	lvl, err := zapcore.ParseLevel(c.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse Config Level")
	}
	c.level = lvl
	return nil
}

// Normalize checks the Config for missing/default values and sets them
// if appropriate.
func (c *Config) Normalize() {
	if c.File != nil {
		if c.File.Filepath == "" {
			c.File.Filepath = defaultFilePath
		}
		if c.File.MaxBackups == 0 {
			c.File.MaxBackups = defaultMaxBackups
		}
		if c.File.MaxSize == 0 {
			c.File.MaxSize = defaultMaxSize
		}
	}
	if c.AppInsights != nil {
		if c.AppInsights.IKey == "" {
			c.AppInsights.IKey = defaultIKey
		}
		if c.AppInsights.GracePeriod.Duration == 0 {
			c.AppInsights.GracePeriod.Duration = defaultGracePeriod
		}
		if c.AppInsights.MaxBatchInterval.Duration == 0 {
			c.AppInsights.MaxBatchInterval.Duration = defaultMaxBatchInterval
		}
		if c.AppInsights.MaxBatchSize == 0 {
			c.AppInsights.MaxBatchSize = defaultMaxBatchSize
		}
	}
	c.normalize()
}
