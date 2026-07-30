//go:build linux

package endpointmanager

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errReleaseBoom = errors.New("endpointmanager test: release boom")

type fakeReleaseIPsClient struct {
	releaseErr error
	gotReq     cns.IPConfigsRequest
	calls      int
}

func (f *fakeReleaseIPsClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	f.calls++
	f.gotReq = req
	return f.releaseErr
}

func (*fakeReleaseIPsClient) GetEndpoint(context.Context, string) (*restserver.GetEndpointResponse, error) {
	return nil, nil //nolint:nilnil // unused on linux
}

func TestReleaseIPs_Success(t *testing.T) {
	f := &fakeReleaseIPsClient{}
	em := WithPlatformReleaseIPsManager(f)
	req := cns.IPConfigsRequest{InfraContainerID: "container-1"}

	err := em.ReleaseIPs(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 1, f.calls)
	assert.Equal(t, req, f.gotReq)
}

func TestReleaseIPs_WrapsError(t *testing.T) {
	f := &fakeReleaseIPsClient{releaseErr: errReleaseBoom}
	em := WithPlatformReleaseIPsManager(f)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to release IP from CNS")
	require.ErrorIs(t, err, errReleaseBoom)
}
