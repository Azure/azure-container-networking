package ipsets

import (
	"testing"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
	"github.com/Microsoft/hcsshim/hcn"
)

func GetHNSFake(t *testing.T) *hnswrapper.Hnsv2wrapperFake {
	hns := hnswrapper.NewHnsv2wrapperFake()
	network := &hcn.HostComputeNetwork{
		Id:   "1234",
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
