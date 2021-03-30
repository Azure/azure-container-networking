package npm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
)

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
