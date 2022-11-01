package common

import (
	"testing"

	"github.com/Azure/azure-container-networking/network/hnswrapper"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/Microsoft/hcsshim/hcn"
	utilexec "k8s.io/utils/exec"
)

const FakeHNSNetworkID = "1234"

type IOShim struct {
	Exec utilexec.Interface
	Hns  hnswrapper.HnsV2WrapperInterface
}

func NewIOShim() *IOShim {
	return &IOShim{
		Exec: utilexec.New(),
		Hns:  &hnswrapper.Hnsv2wrapper{},
	}
}

func NewMockIOShim(calls []testutils.TestCmd) *IOShim {
	hns := hnswrapper.NewHnsv2wrapperFake()
	network := &hcn.HostComputeNetwork{
		Id:   FakeHNSNetworkID,
		Name: "azure",
	}

	// CreateNetwork will never return an error
	_, _ = hns.CreateNetwork(network)

	return &IOShim{
		Exec: testutils.GetFakeExecWithScripts(calls),
		Hns:  hns,
	}
}

func NewMockIOShimWithFakeHNS(hns *hnswrapper.Hnsv2wrapperFake) *IOShim {
	return &IOShim{
		Exec: testutils.GetFakeExecWithScripts([]testutils.TestCmd{}),
		Hns:  hns,
	}
}

// VerifyCalls is used for Unit Testing of linux. In windows this is no-op
func (ioshim *IOShim) VerifyCalls(_ *testing.T, _ []testutils.TestCmd) {}
