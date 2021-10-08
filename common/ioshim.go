package common

import (
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	utilexec "k8s.io/utils/exec"
)

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
	return &IOShim{
		Exec: testutils.GetFakeExecWithScripts(calls),
		Hns:  &hnswrapper.Hnsv2wrapperFake{},
	}
}
