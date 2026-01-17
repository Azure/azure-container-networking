package logger

import (
	"encoding/json"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type FileConfig struct {
	Filepath   string          `json:"filepath"`
	Level      string          `json:"level"`
	level      zapcore.Level   `json:"-"`
	MaxBackups int             `json:"maxBackups"`
	MaxSize    int             `json:"maxSize"`
	Fields     []zapcore.Field `json:"fields"`
}

// UnmarshalJSON implements json.Unmarshaler for the Config.
// It only differs from the default by parsing the
// Level string into a zapcore.Level and setting the level field.
func (cfg *FileConfig) UnmarshalJSON(data []byte) error {
	type Alias FileConfig
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(cfg),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.Wrap(err, "failed to unmarshal FileConfig")
	}
	lvl, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return errors.Wrap(err, "failed to parse FileConfig Level")
	}
	cfg.level = lvl
	return nil
}

// FileCore builds a zapcore.Core that writes to a file.
// The first return is the core, the second is a function to close the file.
func FileCore(cfg *FileConfig) (zapcore.Core, func(), error) {
	filesink := &lumberjack.Logger{
		Filename:   cfg.Filepath,
		MaxSize:    cfg.MaxSize, // MB
		MaxBackups: cfg.MaxBackups,
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	return zapcore.NewCore(&ctrlzap.KubeAwareEncoder{Encoder: jsonEncoder}, zapcore.AddSync(filesink), cfg.level), func() { _ = filesink.Close() }, nil
}
