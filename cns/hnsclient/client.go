package hnsclient

// Client wraps the platform-specific HNS operations exposed by this package.
// It is a concrete, method-based facade so consumers can define their own
// minimal interfaces for testing (see acn-go-interfaces-dependencies skill).
//
// Client carries no per-instance state today. Package-scoped resources such
// as namedLock represent machine-global HNS state and remain package-level.
type Client struct{}

// NewClient returns the default HNS client.
func NewClient() Client {
	return Client{}
}
