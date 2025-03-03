package dpshim

type dirtyCache struct {
	toAddorUpdateSets     map[string]struct{}
	toDeleteSets          map[string]struct{}
	toAddorUpdatePolicies map[string]struct{}
	toDeletePolicies      map[string]struct{}
}

func newDirtyCache() *dirtyCache {
	return &dirtyCache{
		toAddorUpdateSets:     make(map[string]struct{}),
		toDeleteSets:          make(map[string]struct{}),
		toAddorUpdatePolicies: make(map[string]struct{}),
		toDeletePolicies:      make(map[string]struct{}),
	}
}

func (dc *dirtyCache) clearCache() {
	dc.toAddorUpdateSets = make(map[string]struct{})
	dc.toDeleteSets = make(map[string]struct{})
	dc.toAddorUpdatePolicies = make(map[string]struct{})
	dc.toDeletePolicies = make(map[string]struct{})
}

func (dc *dirtyCache) modifyAddorUpdateSets(setName string) {
	delete(dc.toDeleteSets, setName)
	dc.toAddorUpdateSets[setName] = struct{}{}
}

func (dc *dirtyCache) modifyDeleteSets(setName string) {
	delete(dc.toAddorUpdateSets, setName)
	dc.toDeleteSets[setName] = struct{}{}
}

func (dc *dirtyCache) modifyAddorUpdatePolicies(policyName string) {
	delete(dc.toDeletePolicies, policyName)
	dc.toAddorUpdatePolicies[policyName] = struct{}{}
}

func (dc *dirtyCache) modifyDeletePolicies(policyName string) {
	delete(dc.toAddorUpdatePolicies, policyName)
	dc.toDeletePolicies[policyName] = struct{}{}
}

func (dc *dirtyCache) hasContents() bool {
	return len(dc.toAddorUpdateSets) > 0 || len(dc.toDeleteSets) > 0 ||
		len(dc.toAddorUpdatePolicies) > 0 || len(dc.toDeletePolicies) > 0
}
