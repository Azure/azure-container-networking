package platform

import (
	"time"

	"go.uber.org/zap"
)

const (
	defaultExecTimeout = 10
)

type execClient struct {
	Timeout time.Duration
	logger  *zap.Logger
}

//nolint:revive // ExecClient make sense
type ExecClient interface {
	ExecuteCommand(command string) (string, error)
	GetLastRebootTime() (time.Time, error)
	ClearNetworkConfiguration() (bool, error)
	ExecutePowershellCommand(command string) (string, error)
	KillProcessByName(processName string) error
}

func NewExecClient(logger *zap.Logger) ExecClient {
	return &execClient{
		Timeout: defaultExecTimeout * time.Second,
		logger:  logger,
	}
}

func NewExecClientTimeout(timeout time.Duration) ExecClient {
	return &execClient{
		Timeout: timeout,
	}
}
