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
}

func NewExecClient() ExecClient {
	return &execClient{
		Timeout: defaultExecTimeout * time.Second,
	}
}

func NewExecClientTimeout(timeout time.Duration) ExecClient {
	return &execClient{
		Timeout: timeout,
	}
}
