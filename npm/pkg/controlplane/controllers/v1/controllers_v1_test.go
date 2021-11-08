package controllers

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"k8s.io/utils/exec"
)

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	realexec := exec.New()
	iptMgr := iptm.NewIptablesManager(realexec, iptm.NewFakeIptOperationShim())
	err := iptMgr.UninitNpmChains()
	if err != nil {
		fmt.Println("uninitnpmchains failed with %w", err)
	}

	ipsMgr := ipsm.NewIpsetManager(realexec)
	// Do not check returned error here to proceed all UTs.
	// TODO(jungukcho): are there any side effect?
	if err := ipsMgr.DestroyNpmIpsets(); err != nil {
		fmt.Println("failed to destroy ipsets with error %w", err)
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}
