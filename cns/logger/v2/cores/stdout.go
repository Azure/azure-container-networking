package logger

import (
	"encoding/json"
	"os"

	logfmt "github.com/jsternberg/zap-logfmt"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type StdoutConfig struct {
	Level  string          `json:"level"`
	level  zapcore.Level   `json:"-"`
	Fields []zapcore.Field `json:"fields"`
}

// UnmarshalJSON implements json.Unmarshaler for the Config.
// It only differs from the default by parsing the
// Level string into a zapcore.Level and setting the level field.
func (cfg *StdoutConfig) UnmarshalJSON(data []byte) error {
	type Alias StdoutConfig
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(cfg),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal StdoutConfig")
	}
	lvl, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse StdoutConfig Level")
	}
	cfg.level = lvl
	return nil
}

// StdoutCore builds a zapcore.Core that writes to stdout.
func StdoutCore(l zapcore.Level) zapcore.Core {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewCore(&ctrlzap.KubeAwareEncoder{Encoder: logfmt.NewEncoder(encoderConfig)}, os.Stdout, l)
}
