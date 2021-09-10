package cache

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	k8sversion "k8s.io/apimachinery/pkg/version"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	fakeexec "k8s.io/utils/exec/testing"
)

func NPMEncoder(nodeName string) npm.NetworkPolicyManagerEncoder {
	noResyncPeriodFunc := func() time.Duration { return 0 }
	kubeclient := k8sfake.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())
	fakeK8sVersion := &k8sversion.Info{
		GitVersion: "v1.20.2",
	}
	exec := &fakeexec.FakeExec{}
	npmVersion := "npm-ut-test"

	npMgr := npm.NewNetworkPolicyManager(kubeInformer, exec, npmVersion, fakeK8sVersion)
	npMgr.NodeName = nodeName
	return npMgr
}

func TestDecode(t *testing.T) {
	encodedNPMCacheData := "\"nodename\"\n{}\n{}\n{}\n{}\n"
	reader := strings.NewReader(encodedNPMCacheData)
	decodedNPMCache, err := Decode(reader)
	if err != nil {
		t.Errorf("failed to decode %s to NPMCache", encodedNPMCacheData)
	}

	expected := &NPMCache{
		Nodename: "nodename",
		NsMap:    make(map[string]*npm.Namespace),
		PodMap:   make(map[string]*npm.NpmPod),
		ListMap:  make(map[string]*ipsm.Ipset),
		SetMap:   make(map[string]*ipsm.Ipset),
	}

	if !reflect.DeepEqual(decodedNPMCache, expected) {
		t.Errorf("got '%+v', expected '%+v'", decodedNPMCache, expected)
	}
}

func TestEncode(t *testing.T) {
	nodeName := "nodename"
	npmEncoder := NPMEncoder(nodeName)
	var buf bytes.Buffer
	if err := Encode(&buf, npmEncoder); err != nil {
		t.Errorf("failed to encode NPMCache")
	}
	encodedNPMCache := buf.String()

	expected := "\"nodename\"\n{}\n{}\n{}\n{}\n"
	if encodedNPMCache != expected {
		t.Errorf("got '%+v', expected '%+v'", encodedNPMCache, expected)
	}
}
