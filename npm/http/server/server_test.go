package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/cache"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/stretchr/testify/assert"

	"github.com/Azure/azure-container-networking/npm"
	k8sversion "k8s.io/apimachinery/pkg/version"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	fakeexec "k8s.io/utils/exec/testing"
)

func NPMEncoder() npm.NetworkPolicyManagerEncoder {
	noResyncPeriodFunc := func() time.Duration { return 0 }
	kubeclient := k8sfake.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())
	fakeK8sVersion := &k8sversion.Info{
		GitVersion: "v1.20.2",
	}
	exec := &fakeexec.FakeExec{}
	npmVersion := "npm-ut-test"

	npmEncoder := npm.NewNetworkPolicyManager(kubeclient, kubeInformer, exec, npmVersion, fakeK8sVersion)
	return npmEncoder
}

func TestGetNPMCacheHandler(t *testing.T) {
	assert := assert.New(t)

	npmEncoder := NPMEncoder()
	n := &NPMRestServer{}
	handler := n.npmCacheHandler(npmEncoder)

	req, err := http.NewRequest(http.MethodGet, api.NPMMgrPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	var actual *cache.NPMCache
	actual, err = cache.Decode(rr.Body)
	if err != nil {
		t.Fatal(err)
	}

	expected := &cache.NPMCache{
		Nodename: os.Getenv("HOSTNAME"),
		NsMap:    make(map[string]*npm.Namespace),
		PodMap:   make(map[string]*npm.NpmPod),
		ListMap:  make(map[string]*ipsm.Ipset),
		SetMap:   make(map[string]*ipsm.Ipset),
	}

	assert.Exactly(expected, actual)
}
