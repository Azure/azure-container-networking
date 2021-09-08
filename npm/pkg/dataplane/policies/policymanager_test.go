package policies

import "testing"

func TestAddPolicy(t *testing.T) {
	pMgr := NewPolicyManager()

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicies(&netpol)
	if err != nil {
		t.Errorf("AddPolicies() returned error %s", err.Error())
	}
}

func TestRemovePolicies(t *testing.T) {
	pMgr := NewPolicyManager()

	err := pMgr.RemovePolicies("test")
	if err != nil {
		t.Errorf("RemovePolicies() returned error %s", err.Error())
	}
}

func TestUpdatePolicies(t *testing.T) {
	pMgr := NewPolicyManager()

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicies(&netpol)
	if err != nil {
		t.Errorf("UpdatePolicies() returned error %s", err.Error())
	}

	err = pMgr.UpdatePolicies(&netpol)
	if err != nil {
		t.Errorf("UpdatePolicies() returned error %s", err.Error())
	}
}
