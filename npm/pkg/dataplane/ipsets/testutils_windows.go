package ipsets

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/require"
)

func GetHNSFake(t *testing.T) *hnswrapper.Hnsv2wrapperFake {
	hns := hnswrapper.NewHnsv2wrapperFake()
	network := &hcn.HostComputeNetwork{
		Id:   common.FakeHNSNetworkID,
		Name: "azure",
	}

	_, err := hns.CreateNetwork(network)
	require.NoError(t, err)

	return hns
}

func GetApplyIPSetsTestCalls(_, _ []*IPSetMetadata) []testutils.TestCmd {
	return []testutils.TestCmd{}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{}
}
