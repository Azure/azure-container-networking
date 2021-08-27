package singletenantcontroller

import (
	"context"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	Init(context.Context) error
	Start(context.Context) error
	IsStarted() bool
}
