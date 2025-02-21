package logger

import (
	"encoding/json"
	"time"

	loggerv1 "github.com/Azure/azure-container-networking/cns/logger"
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
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

type Config struct {
	// Level is the general logging Level. If cores have more specific config it will override this.
	Level       string                  `json:"level"`
	level       zapcore.Level           `json:"-"`
	AppInsights cores.AppInsightsConfig `json:"appInsights"`
	File        cores.FileConfig        `json:"file"`
}

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
	if l, err := zapcore.ParseLevel(c.Level); err == nil {
		c.level = l
	}
	return nil
}
