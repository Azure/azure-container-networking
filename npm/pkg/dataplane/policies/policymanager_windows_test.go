package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/require"
)

func TestCompareAndRemovePolicies(t *testing.T) {
	epbuilder := newEndpointPolicyBuilder()

	testPol := &NPMACLPolSettings{
		Id:        "test1",
		Protocols: string(TCP),
	}
	testPol2 := &NPMACLPolSettings{
		Id:        "test1",
		Protocols: string(UDP),
	}

	epbuilder.aclPolicies = append(epbuilder.aclPolicies, []*NPMACLPolSettings{testPol, testPol2}...)

	epbuilder.compareAndRemovePolicies("test1", 2)

	if len(epbuilder.aclPolicies) != 0 {
		t.Errorf("Expected 0 policies, got %d", len(epbuilder.aclPolicies))
	}
}

func TestAddPolicies(t *testing.T) {
	hns := ipsets.GetHNSFake()
	io := common.NewMockIOShimWithFakeHNS(hns)
	pMgr := NewPolicyManager(io)

	endPointIDList := map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	}
	for ip, epID := range endPointIDList {
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

	err := pMgr.AddPolicy(TestNetworkPolicies[0], endPointIDList)
	require.NoError(t, err)
}
