package endpointmanager

import (
	"context"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/stretchr/testify/assert"
)

type stubReleaseIPsClient struct{}

func (stubReleaseIPsClient) ReleaseIPs(context.Context, cns.IPConfigsRequest) error {
	return nil
}

func (stubReleaseIPsClient) GetEndpoint(context.Context, string) (*restserver.GetEndpointResponse, error) {
	return nil, nil //nolint:nilnil // stub
}

func TestWithPlatformReleaseIPsManager(t *testing.T) {
	cli := stubReleaseIPsClient{}
	em := WithPlatformReleaseIPsManager(cli)
	assert.NotNil(t, em)
	assert.Equal(t, cli, em.cli)
}

type stubHNS struct{}

func (stubHNS) GetHNSEndpointbyIP(_, _ []net.IPNet) (string, error) { return "", nil }
func (stubHNS) DeleteHNSEndpointbyID(string) error                  { return nil }
func (stubHNS) DeleteNetworkByIDHnsV2(string) error                 { return nil }

func TestWithHNSClient_Override(t *testing.T) {
	hns := stubHNS{}
	em := WithPlatformReleaseIPsManager(stubReleaseIPsClient{}, WithHNSClient(hns))
	assert.Equal(t, hns, em.hns)
}
