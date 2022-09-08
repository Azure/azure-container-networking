package dataplane

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
)

func TestRefreshAllPodEndpoints(t *testing.T) {
	hns := ipsets.GetHNSFake(t)
	ioshim := common.NewMockIOShimWithFakeHNS(hns)
	dptestutils.AddIPsToHNS(t, hns, map[string]string{
		"10.0.0.1": "test1",
		"10.0.0.2": "test2",
	})
	fmt.Println(ioshim)
	// TODO create
}
