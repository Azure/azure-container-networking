package policies

import "testing"

func TestAddPolicy(t *testing.T) {
	pMgr := NewPolicyManager()

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicy(&netpol)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}
}

func TestRemovePolicy(t *testing.T) {
	pMgr := NewPolicyManager()

	err := pMgr.RemovePolicy("test")
	if err != nil {
		t.Errorf("RemovePolicy() returned error %s", err.Error())
	}
}

func TestUpdatePolicy(t *testing.T) {
	pMgr := NewPolicyManager()

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicy(&netpol)
	if err != nil {
		t.Errorf("UpdatePolicy() returned error %s", err.Error())
	}

	err = pMgr.UpdatePolicy(&netpol)
	if err != nil {
		t.Errorf("UpdatePolicy() returned error %s", err.Error())
	}
}
