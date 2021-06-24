package cnireconciler

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/client"
	semver "github.com/hashicorp/go-version"
	"k8s.io/utils/exec"
)

const cniDumpStateVer = "1.4.2"

// IsDumpStateVer checks if the CNI executable is a version that
// has the dump state command required to initialize CNS from CNI
// state and returns the result of that test or an error. Will always
// return false when there is an error.
func IsDumpStateVer() (bool, error) {
	return isDumpStateVer(exec.New())
}

func isDumpStateVer(exec exec.Interface) (bool, error) {
	needVer, err := semver.NewVersion(cniDumpStateVer)
	if err != nil {
		return false, err
	}
	cnicli := client.New(exec)
	if ver, err := cnicli.GetVersion(); err != nil {
		return false, fmt.Errorf("failed to invoke CNI client.GetVersion(): %w", err)
	} else if ver.LessThan(needVer) {
		return false, nil
	}
	return true, nil
}
