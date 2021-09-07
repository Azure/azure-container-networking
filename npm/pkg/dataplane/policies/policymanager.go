package policies

import "sync"

type PolicyMap struct {
	sync.RWMutex
	cache map[string]*NPMNetworkPolicy
}

type PolicyManager struct {
	policyMap *PolicyMap
}

func NewPolicyManager() PolicyManager {
	return PolicyManager{
		policyMap: &PolicyMap{
			cache: make(map[string]*NPMNetworkPolicy),
		},
	}
}

func (pMgr *PolicyManager) GetPolicy(name string) (*NPMNetworkPolicy, error) {
	pMgr.policyMap.RLock()
	defer pMgr.policyMap.RUnlock()

	if policy, ok := pMgr.policyMap.cache[name]; ok {
		return policy, nil
	}

	return nil, nil
}

func (pMgr *PolicyManager) AddPolicies(policy *NPMNetworkPolicy) error {
	pMgr.policyMap.Lock()
	defer pMgr.policyMap.Unlock()

	// Call actual dataplane function to apply changes
	err := pMgr.addPolicies(policy)
	if err != nil {
		return err
	}

	pMgr.policyMap.cache[policy.Name] = policy
	return nil
}

func (pMgr *PolicyManager) RemovePolicies(name string) error {
	pMgr.policyMap.Lock()
	defer pMgr.policyMap.Unlock()

	// Call actual dataplane function to apply changes
	err := pMgr.removePolicies(name)
	if err != nil {
		return err
	}

	delete(pMgr.policyMap.cache, name)

	return nil
}

func (pMgr *PolicyManager) UpdatePolicies(policy *NPMNetworkPolicy) error {
	pMgr.policyMap.Lock()
	defer pMgr.policyMap.Unlock()

	// check and update
	// Call actual dataplane function to apply changes
	err := pMgr.updatePolicies(policy)
	if err != nil {
		return err
	}

	return nil
}
