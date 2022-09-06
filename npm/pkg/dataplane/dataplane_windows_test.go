package dataplane

import (
	"github.com/stretchr/testify/require"
	"network/hnswrapper"
	"testing"

	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
)

func TestRefreshAllPodEndpoints(t *testing.T) {
	hns := ipsets.GetHNSFake(t)
	ioshim := common.NewMockIOShimWithFakeHNS(hns)
	dptestutils.addIPsToHNS(t, hns, map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	})
	// TODO create
}
