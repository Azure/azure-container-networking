package controllers

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"k8s.io/utils/exec"
)

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	exec := exec.New()
	iptMgr := iptm.NewIptablesManager(exec, iptm.NewFakeIptOperationShim())
	iptMgr.UninitNpmChains()

	ipsMgr := ipsm.NewIpsetManager(exec)
	// Do not check returned error here to proceed all UTs.
	// TODO(jungukcho): are there any side effect?
	ipsMgr.DestroyNpmIpsets()

	exitCode := m.Run()
	os.Exit(exitCode)
}
