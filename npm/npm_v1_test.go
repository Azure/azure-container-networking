package npm

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/exec"
)

// To indicate the object is needed to be DeletedFinalStateUnknown Object
type IsDeletedFinalStateUnknownObject bool

const (
	DeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject = true
	DeletedFinalStateknownObject   IsDeletedFinalStateUnknownObject = false
)

func TestMarshalJSONForNilValues(t *testing.T) {
	npMgr := &NetworkPolicyManager{}
	npMgr.ipsMgr = ipsm.NewIpsetManager(exec.New())
	npmCacheRaw, err := npMgr.MarshalJSON()
	assert.NoError(t, err)

	expect := []byte(`{"ListMap":{},"NodeName":"","NsMap":null,"PodMap":null,"SetMap":{}}`)
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

	decodedNPMCache := common.Cache{}
	if err := json.Unmarshal(npmCacheRaw, &decodedNPMCache); err != nil {
		t.Errorf("failed to decode %s to NPMCache", npmCacheRaw)
	}

	expected := common.Cache{
		NodeName: nodeName,
		NsMap:    make(map[string]*common.Namespace),
		PodMap:   make(map[string]*common.NpmPod),
		SetMap:   make(map[string]string),
		ListMap:  make(map[string]string),
	}

	if !reflect.DeepEqual(decodedNPMCache, expected) {
		t.Errorf("got '%+v', expected '%+v'", decodedNPMCache, expected)
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()
	ex := exec.New()
	iptMgr := iptm.NewIptablesManager(ex, iptm.NewFakeIptOperationShim(), npmconfig.DefaultConfig.Toggles.PlaceAzureChainFirst)
	_ = iptMgr.UninitNpmChains()

	ipsMgr := ipsm.NewIpsetManager(ex)
	// Do not check returned error here to proceed all UTs.
	// TODO(jungukcho): are there any side effect?
	_ = ipsMgr.DestroyNpmIpsets()

	exitCode := m.Run()
	os.Exit(exitCode)
}
