package logger

import (
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap/zapcore"
)

const defaultFilePath = "/k/azurecns/azure-cns.log"

type Config struct {
	// Level is the general logging Level. If cores have more specific config it will override this.
	Level       string                   `json:"level"`
	level       zapcore.Level            `json:"-"`
	AppInsights *cores.AppInsightsConfig `json:"appInsights,omitempty"`
	File        *cores.FileConfig        `json:"file,omitempty"`
	ETW         *cores.ETWConfig         `json:"etw,omitempty"`
}

func (c *Config) normalize() {
	if c.ETW != nil {
		if c.ETW.EventName == "" {
			c.ETW.EventName = "AzureCNS"
		}
		if c.ETW.ProviderName == "" {
			c.ETW.ProviderName = "ACN-Monitoring"
		}
	}
}
