package multitenantcontroller

// MultiTenantController defines the interface for multi-tenant network container operations.
type MultiTenantController interface {
	StartMultiTenantController(exitChan <-chan struct{}) error
	IsStarted() bool
}
