package policies

import "testing"

func TestAddPolicy(t *testing.T) {
	pMgr := NewPolicyManager()

	netpol := NPMNetworkPolicy{}

	err := pMgr.AddPolicy(&netpol, nil)
	if err != nil {
		t.Errorf("AddPolicy() returned error %s", err.Error())
	}
}

func TestGetPolicy(t *testing.T) {
	pMgr := NewPolicyManager()
	netpol := NPMNetworkPolicy{
		Name: "test",
	}

	err := pMgr.AddPolicy(&netpol, nil)
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
	pMgr := NewPolicyManager()

	err := pMgr.RemovePolicy("test", nil)
	if err != nil {
		t.Errorf("RemovePolicy() returned error %s", err.Error())
	}
}
