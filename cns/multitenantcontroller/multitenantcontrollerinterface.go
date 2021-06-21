package multitenantcontroller

import "context"

// RequestController defines the interface for multi-tenant network container operations.
type RequestController interface {
	Start(context.Context) error
	IsStarted() bool
}
