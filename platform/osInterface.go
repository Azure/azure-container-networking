package platform

import (
	"time"
)

const (
	defaultExecTimeout = 10
)

type execClient struct {
	Timeout time.Duration
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
