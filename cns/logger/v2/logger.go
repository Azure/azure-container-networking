package logger

import (
	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	// Level is the general logging Level. If cores have more specific config it will override this.
	Level      zapcore.Level
	AIConfig   cores.AIConfig
	FileConfig cores.FileConfig
}

type compoundCloser []func()

func (c compoundCloser) Close() {
	for _, closer := range c {
		closer()
	}
}

func New(cfg *Config) (*zap.Logger, func(), error) {
	stdoutCore := cores.StdoutCore(cfg.Level)
	closer := compoundCloser{}
	fileCore, fileCloser, err := cores.FileCore(&cfg.FileConfig)
	closer = append(closer, fileCloser)
	if err != nil {
		return nil, closer.Close, err //nolint:wrapcheck // it's an internal pkg
	}
	aiCore, aiCloser, err := cores.ApplicationInsightsCore(&cfg.AIConfig)
	closer = append(closer, aiCloser)
	if err != nil {
		return nil, closer.Close, err //nolint:wrapcheck // it's an internal pkg
	}
	platformCore, platformCloser, err := platformCore(cfg)
	closer = append(closer, platformCloser)
	if err != nil {
		return nil, closer.Close, err
	}
	core := zapcore.NewTee(stdoutCore, fileCore, aiCore, platformCore)
	return zap.New(core), closer.Close, nil
}
