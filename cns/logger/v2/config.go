package logger

import (
	"encoding/json"
	"time"

	loggerv1 "github.com/Azure/azure-container-networking/cns/logger"
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
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal Config")
	}
	lvl, err := zapcore.ParseLevel(c.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse Config Level")
	}
	c.level = lvl
	return nil
}
