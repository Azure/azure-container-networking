package cnireconciler

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/client"
	semver "github.com/hashicorp/go-version"
	"k8s.io/utils/exec"
)

// >= 1.4.7 is required due to a bug in CNI when the statefile is empty
// even though the command existed since 1.4.2.
const lastCNIWithoutDumpStateVer = "1.4.6"

// IsDumpStateVer checks if the CNI executable is a version that
// has the dump state command required to initialize CNS from CNI
// state and returns the result of that test or an error. Will always
// return false when there is an error.
func IsDumpStateVer() (bool, error) {
	return isDumpStateVer(exec.New())
}

func isDumpStateVer(exec exec.Interface) (bool, error) {
	needVer, err := semver.NewVersion(lastCNIWithoutDumpStateVer)
	if err != nil {
		return false, err
	}
	cnicli := client.New(exec)
	ver, err := cnicli.GetVersion()
	if err != nil {
		return false, fmt.Errorf("failed to invoke CNI client.GetVersion(): %w", err)
	}
	return ver.GreaterThan(needVer), nil
}
