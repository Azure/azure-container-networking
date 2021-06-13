package multitenantcontroller

import "context"

// MultiTenantController defines the interface for multi-tenant network container operations.
type MultiTenantController interface {
	StartMultiTenantController(context.Context) error
	IsStarted() bool
}
