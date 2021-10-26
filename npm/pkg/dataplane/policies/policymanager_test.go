package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
)

func TestAddPolicy(t *testing.T) {
	netpol := &NPMNetworkPolicy{}

	calls := getAddPolicyTestCalls(netpol)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))

	err := pMgr.AddPolicy(netpol, nil)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}
}

func TestGetPolicy(t *testing.T) {
	netpol := &NPMNetworkPolicy{
		Name: "test",
	}

	calls := getAddPolicyTestCalls(netpol)
	calls = append(calls, getRemovePolicyTestCalls(netpol)...)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))

	err := pMgr.AddPolicy(netpol, nil)
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
	pMgr := NewPolicyManager(common.NewMockIOShim(nil))
	err := pMgr.RemovePolicy("test", nil)
	if err != nil {
		t.Errorf("RemovePolicy() returned error %s", err.Error())
	}
}
