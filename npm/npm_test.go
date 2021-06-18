package npm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/exec"
	utilexec "k8s.io/utils/exec"
)

// To indicate the object is needed to be DeletedFinalStateUnknown Object
type IsDeletedFinalStateUnknownObject bool

const (
	DeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject = true
	DeletedFinalStateknownObject   IsDeletedFinalStateUnknownObject = false
)

func getKey(obj interface{}, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		t.Errorf("Unexpected error getting key for obj %v: %v", obj, err)
		return ""
	}
	return key
}

func newNPMgr(t *testing.T, exec utilexec.Interface) *NetworkPolicyManager {
	npMgr := &NetworkPolicyManager{
		Exec:             exec,
		NsMap:            make(map[string]*Namespace),
		PodMap:           make(map[string]*NpmPod),
		TelemetryEnabled: false,
	}

	// This initialization important as without this NPM will panic
	allNs, _ := newNs(util.KubeAllNamespacesFlag, npMgr.Exec)
	npMgr.NsMap[util.KubeAllNamespacesFlag] = allNs
	return npMgr
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	exec := exec.New()
	iptMgr := iptm.NewIptablesManager(exec, iptm.NewFakeIptOperationShim())
	iptMgr.UninitNpmChains()

	ipsMgr := ipsm.NewIpsetManager(exec)
	ipsMgr.Destroy()

	exitCode := m.Run()

	os.Exit(exitCode)
}
