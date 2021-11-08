package npm

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/exec"
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

func TestMarshalJSONForNilValues(t *testing.T) {
	npMgr := &NetworkPolicyManager{}
	npmCacheRaw, err := npMgr.MarshalJSON()
	assert.NoError(t, err)

	expect := []byte(`{"NodeName":"","NsMap":null,"PodMap":null}`)
	assert.ElementsMatch(t, expect, npmCacheRaw)
}

func TestMarshalJSON(t *testing.T) {
	nodeName := "test-nodename"
	npmCacheEncoder := CacheEncoder(nodeName)
	npmCacheRaw, err := npmCacheEncoder.MarshalJSON()
	assert.NoError(t, err)

	// "test-nodename" in "NodeName" should be the same as nodeName variable.
	expect := []byte(`{"ListMap":{},"NodeName":"test-nodename","NsMap":{},"PodMap":{},"SetMap":{}}`)
	assert.ElementsMatch(t, expect, npmCacheRaw)
}

func TestMarshalUnMarshalJSON(t *testing.T) {
	nodeName := "test-nodename"
	npmCacheEncoder := CacheEncoder(nodeName)

	npmCacheRaw, err := npmCacheEncoder.MarshalJSON()
	assert.NoError(t, err)

	decodedNPMCache := controllersv1.Cache{}
	if err := json.Unmarshal(npmCacheRaw, &decodedNPMCache); err != nil {
		t.Errorf("failed to decode %s to NPMCache", npmCacheRaw)
	}

	expected := controllersv1.Cache{
		ListMap:  make(map[string]*ipsm.Ipset),
		NodeName: nodeName,
		NsMap:    make(map[string]*controllersv1.Namespace),
		PodMap:   make(map[string]*controllersv1.NpmPod),
		SetMap:   make(map[string]*ipsm.Ipset),
	}

	if !reflect.DeepEqual(decodedNPMCache, expected) {
		t.Errorf("got '%+v', expected '%+v'", decodedNPMCache, expected)
	}
}

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
