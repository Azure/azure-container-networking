package logger

import (
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap/zapcore"
)

const defaultFilePath = "/var/log/azure-cns.log"

type Config struct {
	// Level is the general logging Level. If cores have more specific config it will override this.
	Level       string                   `json:"level"`
	level       zapcore.Level            `json:"-"`
	AppInsights *cores.AppInsightsConfig `json:"appInsights,omitempty"`
	File        *cores.FileConfig        `json:"file,omitempty"`
}

func (c *Config) normalize() {}
