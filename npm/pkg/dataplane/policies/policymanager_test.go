package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	testutils "github.com/Azure/azure-container-networking/test/utils"
)

var (
	// below epList is no-op for linux
	epList        = map[string]string{"10.0.0.1": "test123", "10.0.0.2": "test456"}
	testNSSet     = ipsets.NewIPSetMetadata("test-ns-set", ipsets.Namespace)
	testKeyPodSet = ipsets.NewIPSetMetadata("test-keyPod-set", ipsets.KeyLabelOfPod)
	testNetPol    = NPMNetworkPolicy{
		Name: "test/test-netpol",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet,
			},
			{
				Metadata: testKeyPodSet,
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet,
			},
			{
				Metadata: testKeyPodSet,
			},
		},
		ACLs: []*ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    Dropped,
				Direction: Ingress,
			},
			{
				PolicyID:  "azure-acl-234",
				Target:    Allowed,
				Direction: Ingress,
				SrcList: []SetInfo{
					{
						IPSet:     testNSSet,
						Included:  true,
						MatchType: "src",
					},
					{
						IPSet:     testKeyPodSet,
						Included:  true,
						MatchType: "src",
					},
				},
			},
		},
		PodEndpoints: map[string]string{
			"10.0.0.1": "1234",
		},
	}
)

func TestAddPolicy(t *testing.T) {
	pMgr := NewPolicyManager(common.NewMockIOShim([]testutils.TestCmd{}))

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicy(&netpol, epList)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}

	err = pMgr.AddPolicy(&testNetPol, epList)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}
}

func TestGetPolicy(t *testing.T) {
	pMgr := NewPolicyManager(common.NewMockIOShim([]testutils.TestCmd{}))
	netpol := NPMNetworkPolicy{
		Name: "test",
		ACLs: []*ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    Dropped,
				Direction: Ingress,
			},
		},
	}

	err := pMgr.AddPolicy(&netpol, epList)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}

	ok := pMgr.PolicyExists("test")
	if !ok {
		t.Error("PolicyExists() returned false")
	}

	policy, ok := pMgr.GetPolicy("test")
	if !ok {
		t.Error("GetPolicy() returned false")
	} else if policy.Name != "test" {
		t.Errorf("GetPolicy() returned wrong policy %s", policy.Name)
	}

}

func TestRemovePolicy(t *testing.T) {
	pMgr := NewPolicyManager(common.NewMockIOShim([]testutils.TestCmd{}))

	err := pMgr.AddPolicy(&testNetPol, epList)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}

	err = pMgr.RemovePolicy("test", epList)
	if err != nil {
		t.Errorf("RemovePolicy() returned error %s", err.Error())
	}
	err = pMgr.RemovePolicy("test/test-netpol", nil)
	if err != nil {
		t.Errorf("RemovePolicy() returned error %s", err.Error())
	}
}
