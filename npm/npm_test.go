package npm

import (
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/client-go/tools/cache"
	utilexec "k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"
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
	iptMgr := iptm.NewIptablesManager()
	iptMgr.Save(util.IptablesConfigFile)

	var calls = []struct {
		cmd []string
		err error
	}{
		{cmd: []string{"ipset", "save", "-file", "/var/log/ipset.conf"}, err: nil},
		{cmd: []string{"ipset", "restore", "-file", "/var/log/ipset.conf"}, err: nil},
	}

	fcmd := fakeexec.FakeCmd{CombinedOutputScript: []fakeexec.FakeAction{}}
	fexec := fakeexec.FakeExec{CommandScript: []fakeexec.FakeCommandAction{}}

	// expect happy path, each call returns no errors
	for _, call := range calls {
		fcmd.CombinedOutputScript = append(fcmd.CombinedOutputScript, func() ([]byte, []byte, error) { return nil, nil, call.err })
		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) utilexec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) })
	}
	ipsMgr := ipsm.NewIpsetManager(&fexec)
	ipsMgr.Save(util.IpsetConfigFile)

	exitCode := m.Run()

	iptMgr.Restore(util.IptablesConfigFile)
	ipsMgr.Restore(util.IpsetConfigFile)

	if fcmd.CombinedOutputCalls != len(calls) {
		log.Fatalf("Mismatched calls, expected %v, actual %v", len(calls), fcmd.CombinedOutputCalls)
	}

	for i, call := range calls {
		if !reflect.DeepEqual(call, fcmd.CombinedOutputLog[i]) {
			log.Fatalf("Mismatched call, expected %v, actual %v", call, fcmd.CombinedOutputLog[i])
		}
	}

	os.Exit(exitCode)
}
