package dptestutils

import (
	"testing"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/require"
)

func AddIPsToHNS(t *testing.T, hns *hnswrapper.Hnsv2wrapperFake, ipsToEndpoints map[string]string) {
	for ip, epID := range ipsToEndpoints {
		ep := &hcn.HostComputeEndpoint{
			Id:   epID,
			Name: epID,
			IpConfigurations: []hcn.IpConfig{
				{
					IpAddress: ip,
				},
			},
		}
		_, err := hns.CreateEndpoint(ep)
		require.NoError(t, err)
	}
}
