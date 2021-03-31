package npm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
)

func newNPMgr(t *testing.T) *NetworkPolicyManager {
	npMgr := &NetworkPolicyManager{
		NsMap:            make(map[string]*Namespace),
		PodMap:           make(map[string]*NpmPod),
		TelemetryEnabled: false,
	}

	// This initialization important as without this NPM will panic
	allNs, _ := newNs(util.KubeAllNamespacesFlag)
	npMgr.NsMap[util.KubeAllNamespacesFlag] = allNs
	return npMgr
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	iptMgr := iptm.NewIptablesManager()
	iptMgr.Save(util.IptablesConfigFile)

	ipsMgr := ipsm.NewIpsetManager()
	ipsMgr.Save(util.IpsetConfigFile)

	exitCode := m.Run()

	iptMgr.Restore(util.IptablesConfigFile)
	ipsMgr.Restore(util.IpsetConfigFile)

	os.Exit(exitCode)
}
