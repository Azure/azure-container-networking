// Package logger provides an opinionated logger for CNS which knows how to
// log to Application Insights, file, stdout and ETW (based on platform).
package logger

import (
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type compoundCloser []func()

func (c compoundCloser) Close() {
	for _, closer := range c {
		closer()
	}
}

// New creates a v2 CNS logger built with Zap.
func New(cfg *Config) (*zap.Logger, func(), error) {
	cfg.Normalize()
	core := cores.StdoutCore(cfg.level)
	closer := compoundCloser{}
	if cfg.File != nil {
		fileCore, fileCloser, err := cores.FileCore(cfg.File)
		closer = append(closer, fileCloser)
		if err != nil {
			return nil, closer.Close, err //nolint:wrapcheck // it's an internal pkg
		}
		core = zapcore.NewTee(core, fileCore)
	}
	if cfg.AppInsights != nil {
		aiCore, aiCloser, err := cores.ApplicationInsightsCore(cfg.AppInsights)
		closer = append(closer, aiCloser)
		if err != nil {
			return nil, closer.Close, err //nolint:wrapcheck // it's an internal pkg
		}
		core = zapcore.NewTee(core, aiCore)
	}
	platformCore, platformCloser, err := platformCore(cfg)
	closer = append(closer, platformCloser)
	if err != nil {
		return nil, closer.Close, err
	}
	core = zapcore.NewTee(core, platformCore)
	return zap.New(core), closer.Close, nil
}
