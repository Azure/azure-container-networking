package ipsets

import (
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/Microsoft/hcsshim/hcn"
)

func getMockIOShim(_ []testutils.TestCmd) *common.IOShim {
	return common.NewMockIOShimWithFakeHNS(GetHNSFake())
}

func GetHNSFake() *hnswrapper.Hnsv2wrapperFake {
	hns := hnswrapper.NewHnsv2wrapperFake()
	network := &hcn.HostComputeNetwork{
		Id:   "1234",
		Name: "azure",
	}

	hns.CreateNetwork(network)

	return hns
}

func GetApplyIPSetsTestCalls(_, _ []*IPSetMetadata) []testutils.TestCmd {
	return []testutils.TestCmd{}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{}
}
