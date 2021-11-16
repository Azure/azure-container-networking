package common

import (
	"testing"

	testutils "github.com/Azure/azure-container-networking/test/utils"
	utilexec "k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
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

// VerifyCalls is used for Unit Testing with a mock ioshim. It asserts that the number of calls made is equal to the number given to the mock ioshim.
func (ioshim *IOShim) VerifyCalls(t *testing.T, calls []testutils.TestCmd) {
	fexec, couldCast := ioshim.Exec.(*testingexec.FakeExec)
	if couldCast {
		testutils.VerifyCalls(t, fexec, calls)
	}
}
