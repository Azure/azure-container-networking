package npm

import (
	"encoding/json"
	"time"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	k8sversion "k8s.io/apimachinery/pkg/version"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	fakeexec "k8s.io/utils/exec/testing"
)

// CacheEncoder is used only for unit tests to test encoding and decoding Cache.
func CacheEncoder(nodeName string) json.Marshaler {
	noResyncPeriodFunc := func() time.Duration { return 0 }
	kubeclient := k8sfake.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())
	fakeK8sVersion := &k8sversion.Info{
		GitVersion: "v1.20.2",
	}
	exec := &fakeexec.FakeExec{}
	npmVersion := "npm-ut-test"

	npMgr := NewNetworkPolicyManager(npmconfig.DefaultConfig, kubeInformer, &dpmocks.MockGenericDataplane{}, exec, npmVersion, fakeK8sVersion)
	npMgr.NodeName = nodeName
	return npMgr
}
