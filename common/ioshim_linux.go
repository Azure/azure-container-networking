package common

import (
	testutils "github.com/Azure/azure-container-networking/test/utils"
	utilexec "k8s.io/utils/exec"
)

type IOShim struct {
	Exec utilexec.Interface
}

func NewIOShim() *IOShim {
	return &IOShim{
		Exec: utilexec.New(),
	}
}

func NewMockIOShim(calls []testutils.TestCmd) *IOShim {
	return &IOShim{
		Exec: testutils.GetFakeExecWithScripts(calls),
	}
}
