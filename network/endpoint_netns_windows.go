package network

import "github.com/pkg/errors"

// errNetnsReconstructionUnsupported is returned on platforms where endpoint state
// cannot be recovered by inspecting the pod network namespace.
var errNetnsReconstructionUnsupported = errors.New("reconstructing endpoint info from netns is not supported on this platform")

// GetEndpointInfoFromNetns is not supported on Windows: HNS owns the endpoint
// state rather than the namespace, so there is nothing equivalent to read back.
func (nm *networkManager) GetEndpointInfoFromNetns(_, _, _ string) (*EndpointInfo, error) {
	return nil, errNetnsReconstructionUnsupported
}
