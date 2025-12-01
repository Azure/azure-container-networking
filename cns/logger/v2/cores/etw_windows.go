package logger

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/zapetw"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type ETWConfig struct {
	EventName    string          `json:"eventname"`
	Level        string          `json:"level"`
	level        zapcore.Level   `json:"-"`
	ProviderName string          `json:"providername"`
	Fields       []zapcore.Field `json:"fields"`
}

// UnmarshalJSON implements json.Unmarshaler for the Config.
// It only differs from the default by parsing the
// Level string into a zapcore.Level and setting the level field.
func (cfg *ETWConfig) UnmarshalJSON(data []byte) error {
	type Alias ETWConfig
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(cfg),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal ETWConfig")
	}
	lvl, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse ETWConfig Level")
	}
	cfg.level = lvl
	return nil
}

// ETWCore builds a zapcore.Core that sends logs to ETW.
// The first return is the core, the second is a function to close the sink.
func ETWCore(cfg *ETWConfig) (zapcore.Core, func(), error) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	return zapetw.New(cfg.ProviderName, cfg.EventName, &ctrlzap.KubeAwareEncoder{Encoder: jsonEncoder}, cfg.level) //nolint:wrapcheck // ignore
}
