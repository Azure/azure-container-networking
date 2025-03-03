package dpshim

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

const cleanEmptySetsInHrs = 24

var ErrChannelUnset = errors.New("channel must be set")

// (TODO) DPShim has commonalities with IPSetManager, we should consider refactoring
// to have a common interface for both.

type DPShim struct {
	OutChannel  chan *protos.Events
	stopChannel <-chan struct{}
	setCache    map[string]*controlplane.ControllerIPSets
	policyCache map[string]*policies.NPMNetworkPolicy
	dirtyCache  *dirtyCache
	mu          *sync.Mutex
}

func NewDPSim(stopChannel <-chan struct{}) (*DPShim, error) {
	return &DPShim{
		OutChannel:  make(chan *protos.Events),
		setCache:    make(map[string]*controlplane.ControllerIPSets),
		policyCache: make(map[string]*policies.NPMNetworkPolicy),
		stopChannel: stopChannel,
		dirtyCache:  newDirtyCache(),
		mu:          &sync.Mutex{},
	}, nil
}

func (dp *DPShim) BootupDataplane() error {
	return nil
}

func (dp *DPShim) FinishBootupPhase() {
	// No-op
}

// HydrateClients is used in DPShim to hydrate a restarted Daemon Client
func (dp *DPShim) HydrateClients() (*protos.Events, error) {
	dp.lock()
	defer dp.unlock()

	if len(dp.setCache) == 0 && len(dp.policyCache) == 0 {
		klog.Infof("HydrateClients: No local cache objects to hydrate daemon client")
		return nil, nil
	}

	goalStates := make(map[string]*protos.GoalState)

	toApplySets, err := dp.hydrateSetCache()
	if err != nil {
		return nil, err
	}
	if toApplySets != nil {
		goalStates[controlplane.IpsetApply] = toApplySets
	}

	toApplyPolicies, err := dp.hydratePolicyCache()
	if err != nil {
		return nil, err
	}
	if toApplyPolicies != nil {
		goalStates[controlplane.PolicyApply] = toApplyPolicies
	}

	if len(goalStates) == 0 {
		klog.Info("HydrateClients: No changes to apply")
		return nil, nil
	}

	return &protos.Events{
		EventType: protos.Events_Hydration,
		Payload:   goalStates,
	}, nil
}

func (dp *DPShim) RunPeriodicTasks() {
	// Here Run periodic task to check if any sets with empty references are present and delete them
	dp.deleteUnusedSets(dp.stopChannel)
}

// GetIPSet is a no-op in DPShim since DPShim does not deal with IPSet object
func (dp *DPShim) GetIPSet(setName string) *ipsets.IPSet {
	return nil
}

func (dp *DPShim) getCachedIPSet(setName string) *controlplane.ControllerIPSets {
	return dp.setCache[setName]
}

func (dp *DPShim) setExists(setName string) bool {
	_, ok := dp.setCache[setName]
	return ok
}

func (dp *DPShim) CreateIPSets(setMetadatas []*ipsets.IPSetMetadata) {
	dp.lock()
	defer dp.unlock()
	for _, set := range setMetadatas {
		dp.createIPSet(set)
	}
}

func (dp *DPShim) createIPSet(set *ipsets.IPSetMetadata) {
	setName := set.GetPrefixName()

	if dp.setExists(setName) {
		return
	}

	dp.setCache[setName] = controlplane.NewControllerIPSets(set)
	dp.dirtyCache.modifyAddorUpdateSets(setName)
}

func (dp *DPShim) DeleteIPSet(setMetadata *ipsets.IPSetMetadata, _ util.DeleteOption) {
	dp.lock()
	defer dp.unlock()
	dp.deleteIPSet(setMetadata)
}

func (dp *DPShim) deleteIPSet(setMetadata *ipsets.IPSetMetadata) {
	setName := setMetadata.GetPrefixName()
	klog.Infof("deleteIPSet: cleaning up %s", setName)
	set, ok := dp.setCache[setName]
	if !ok {
		return
	}

	if set.HasReferences() {
		klog.Infof("deleteIPSet: ignore delete since set: %s has references", setName)
		return
	}

	delete(dp.setCache, setName)
	dp.dirtyCache.modifyDeleteSets(setName)
}

func (dp *DPShim) AddToSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *dataplane.PodMetadata) error {
	if len(setMetadatas) == 0 {
		return nil
	}

	dp.lock()
	defer dp.unlock()

	for _, set := range setMetadatas {
		klog.Infof("AddToSets: Adding pod IP: %s, Key: %s,  to set %s", podMetadata.PodIP, podMetadata.PodKey, set.GetPrefixName())
		prefixedSetName := set.GetPrefixName()
		if !dp.setExists(prefixedSetName) {
			dp.createIPSet(set)
		}

		set := dp.setCache[prefixedSetName]
		if set.IPSetMetadata.GetSetKind() != ipsets.HashSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set", prefixedSetName))
		}

		cachedPodMetadata, ok := set.IPPodMetadata[podMetadata.PodIP]
		if ok && cachedPodMetadata.PodKey == podMetadata.PodKey {
			continue
		}
		set.IPPodMetadata[podMetadata.PodIP] = podMetadata
		dp.dirtyCache.modifyAddorUpdateSets(prefixedSetName)
	}

	return nil
}

func (dp *DPShim) RemoveFromSets(setMetadatas []*ipsets.IPSetMetadata, podMetadata *dataplane.PodMetadata) error {
	if len(setMetadatas) == 0 {
		return nil
	}

	dp.lock()
	defer dp.unlock()

	for _, set := range setMetadatas {
		klog.Infof("RemoveFromSets: removing pod ip: %s, podkey: %s,  from set %s ", podMetadata.PodIP, podMetadata.PodKey, set.GetPrefixName())
		prefixedSetName := set.GetPrefixName()
		if !dp.setExists(prefixedSetName) {
			continue
		}

		set := dp.setCache[prefixedSetName]
		if set.IPSetMetadata.GetSetKind() != ipsets.HashSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("RemoveFromSets, ipset %s is not a hash set", prefixedSetName))
		}

		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPod, exists := set.IPPodMetadata[podMetadata.PodIP]
		if !exists {
			continue
		}
		if cachedPod.PodKey != podMetadata.PodKey {
			klog.Infof("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				cachedPod.PodIP, prefixedSetName, cachedPod.PodKey, podMetadata.PodKey)
			continue
		}

		// update the IP ownership with podkey
		delete(set.IPPodMetadata, podMetadata.PodIP)
		dp.dirtyCache.modifyAddorUpdateSets(prefixedSetName)
	}
	return nil
}

func (dp *DPShim) AddToLists(listMetadatas, setMetadatas []*ipsets.IPSetMetadata) error {
	if len(listMetadatas) == 0 || len(setMetadatas) == 0 {
		return nil
	}

	dp.lock()
	defer dp.unlock()

	for _, setMetadata := range setMetadatas {
		setName := setMetadata.GetPrefixName()
		if !dp.setExists(setName) {
			dp.createIPSet(setMetadata)
		}

		set := dp.setCache[setName]
		if set.IPSetMetadata.GetSetKind() != ipsets.HashSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", setName))
		}
	}

	for _, listMetadata := range listMetadatas {
		listName := listMetadata.GetPrefixName()
		if !dp.setExists(listName) {
			dp.createIPSet(listMetadata)
		}

		list := dp.setCache[listName]

		if list.IPSetMetadata.GetSetKind() != ipsets.ListSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
		}

		modified := false
		for _, setMetadata := range setMetadatas {
			setName := setMetadata.GetPrefixName()
			if _, ok := list.MemberIPSets[setName]; ok {
				continue
			}

			set := dp.setCache[setName]
			list.MemberIPSets[setName] = set.IPSetMetadata
			set.AddReference(listName, controlplane.ListReference)
			dp.dirtyCache.modifyAddorUpdateSets(setName)
			modified = true
		}

		if modified {
			dp.dirtyCache.modifyAddorUpdateSets(listName)
		}
	}

	return nil
}

func (dp *DPShim) RemoveFromList(listMetadata *ipsets.IPSetMetadata, setMetadatas []*ipsets.IPSetMetadata) error {
	if len(setMetadatas) == 0 || listMetadata == nil {
		return nil
	}

	dp.lock()
	defer dp.unlock()

	listName := listMetadata.GetPrefixName()
	list, exists := dp.setCache[listName]
	if !exists {
		return nil
	}

	if list.IPSetMetadata.GetSetKind() != ipsets.ListSet {
		return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
	}

	modified := false
	for _, setMetadata := range setMetadatas {
		setName := setMetadata.GetPrefixName()
		if !dp.setExists(setName) {
			continue
		}

		set := dp.setCache[setName]
		if set.IPSetMetadata.GetSetKind() != ipsets.HashSet {
			if modified {
				dp.dirtyCache.modifyAddorUpdateSets(listName)
			}
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", setName))
		}

		if _, ok := list.MemberIPSets[setName]; !ok {
			continue
		}

		delete(list.MemberIPSets, setName)
		set.DeleteReference(listName, controlplane.ListReference)
		modified = true
	}
	if modified {
		dp.dirtyCache.modifyAddorUpdateSets(listName)
	}

	return nil
}

func (dp *DPShim) AddPolicy(networkpolicies *policies.NPMNetworkPolicy) error {
	var err error
	// apply dataplane after syncing
	defer func() {
		dperr := dp.ApplyDataPlane()
		if dperr != nil {
			err = fmt.Errorf("failed with error %w, apply failed with %v", err, dperr)
		}
	}()

	// Here refers work in LIFO, DP gets unlocked first, then ApplyDataPlane will acquire lock again
	dp.lock()
	defer dp.unlock()

	if dp.policyExists(networkpolicies.PolicyKey) {
		return nil
	}

	policies.NormalizePolicy(networkpolicies)
	if vErr := policies.ValidatePolicy(networkpolicies); vErr != nil {
		return npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("couldn't add malformed policy: %s", vErr.Error()))
	}
	dp.policyCache[networkpolicies.PolicyKey] = networkpolicies
	dp.dirtyCache.modifyAddorUpdatePolicies(networkpolicies.PolicyKey)

	return err
}

func (dp *DPShim) RemovePolicy(policyKey string) error {
	var err error
	// apply dataplane after syncing
	defer func() {
		dperr := dp.ApplyDataPlane()
		if dperr != nil {
			err = fmt.Errorf("failed with error %w, apply failed with %v", err, dperr)
		}
	}()

	// Here refers work in LIFO, DP gets unlocked first, then ApplyDataPlane will acquire lock again
	dp.lock()
	defer dp.unlock()
	// keeping err different so we can catch the defer func err
	delete(dp.policyCache, policyKey)
	dp.dirtyCache.modifyDeletePolicies(policyKey)

	return err
}

func (dp *DPShim) UpdatePolicy(networkpolicies *policies.NPMNetworkPolicy) error {
	var err error
	// apply dataplane after syncing
	defer func() {
		dperr := dp.ApplyDataPlane()
		if dperr != nil {
			err = fmt.Errorf("failed with error %w, apply failed with %v", err, dperr)
		}
	}()

	// Here refers work in LIFO, DP gets unlocked first, then ApplyDataPlane will acquire lock again
	dp.lock()
	defer dp.unlock()

	// For simplicity, we will not be adding references of netpols to ipsets.
	// DP in daemon will take care of tracking the references.

	dp.policyCache[networkpolicies.PolicyKey] = networkpolicies
	dp.dirtyCache.modifyAddorUpdatePolicies(networkpolicies.PolicyKey)

	return err
}

func (dp *DPShim) ApplyDataPlane() error {
	dp.lock()
	defer dp.unlock()

	// check dirty cache contents
	if !dp.dirtyCache.hasContents() {
		klog.Info("ApplyDataPlane: No changes to apply")
		return nil
	}

	dp.dirtyCache.printContents()

	goalStates := make(map[string]*protos.GoalState)

	toApplySets, err := dp.processIPSetsApply()
	if err != nil {
		return err
	}
	if toApplySets != nil {
		goalStates[controlplane.IpsetApply] = toApplySets
	}

	toDeleteSets, err := dp.processIPSetsDelete()
	if err != nil {
		return err
	}
	if toDeleteSets != nil {
		goalStates[controlplane.IpsetRemove] = toDeleteSets
	}

	toApplyPolicies, err := dp.processPoliciesApply()
	if err != nil {
		return err
	}
	if toApplyPolicies != nil {
		goalStates[controlplane.PolicyApply] = toApplyPolicies
	}

	toDeletePolicies, err := dp.processPoliciesRemove()
	if err != nil {
		return err
	}
	if toDeletePolicies != nil {
		goalStates[controlplane.PolicyRemove] = toDeletePolicies
	}

	if len(goalStates) == 0 {
		klog.Info("ApplyDataPlane: No changes to apply")
		return nil
	}

	go func() {
		dp.OutChannel <- &protos.Events{
			EventType: protos.Events_GoalState,
			Payload:   goalStates,
		}
	}()

	dp.dirtyCache.clearCache()
	return nil
}

func (dp *DPShim) GetAllIPSets() map[string]string {
	return nil
}

func (dp *DPShim) GetAllPolicies() []string {
	return nil
}

func (dp *DPShim) lock() {
	dp.mu.Lock()
}

func (dp *DPShim) unlock() {
	dp.mu.Unlock()
}

func (dp *DPShim) policyExists(policyKey string) bool {
	_, ok := dp.policyCache[policyKey]
	return ok
}

func (dp *DPShim) processIPSetsApply() (*protos.GoalState, error) {
	if len(dp.dirtyCache.toAddorUpdateSets) == 0 {
		return nil, nil
	}

	toApplySets := make([]*controlplane.ControllerIPSets, len(dp.dirtyCache.toAddorUpdateSets))
	idx := 0

	for setName := range dp.dirtyCache.toAddorUpdateSets {
		set := dp.getCachedIPSet(setName)
		if set == nil {
			klog.Errorf("processIPSetsApply: set %s not found", setName)
			return nil, npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s not found", setName))
		}

		toApplySets[idx] = set
		idx++
	}

	payload, err := controlplane.EncodeControllerIPSets(toApplySets)
	if err != nil {
		klog.Errorf("processIPSetsApply: failed to encode sets %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.AppendIPSet, false, "processIPSetsApply: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) processIPSetsDelete() (*protos.GoalState, error) {
	if len(dp.dirtyCache.toDeleteSets) == 0 {
		return nil, nil
	}

	toDeleteSets := make([]string, len(dp.dirtyCache.toDeleteSets))
	idx := 0

	for setName := range dp.dirtyCache.toDeleteSets {
		toDeleteSets[idx] = setName
		idx++
	}

	payload, err := controlplane.EncodeStrings(toDeleteSets)
	if err != nil {
		klog.Errorf("processIPSetsDelete: failed to encode sets %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.DeleteIPSet, false, "processIPSetsDelete: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) processPoliciesApply() (*protos.GoalState, error) {
	if len(dp.dirtyCache.toAddorUpdatePolicies) == 0 {
		return nil, nil
	}

	toApplyPolicies := make([]*policies.NPMNetworkPolicy, len(dp.dirtyCache.toAddorUpdatePolicies))
	idx := 0

	for policyKey := range dp.dirtyCache.toAddorUpdatePolicies {
		if !dp.policyExists(policyKey) {
			return nil, npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("policy %s not found", policyKey))
		}

		policy := dp.policyCache[policyKey]
		toApplyPolicies[idx] = policy
		idx++
	}

	payload, err := controlplane.EncodeNPMNetworkPolicies(toApplyPolicies)
	if err != nil {
		klog.Errorf("processPoliciesApply: failed to encode policies %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.AddPolicy, false, "processPoliciesApply: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) processPoliciesRemove() (*protos.GoalState, error) {
	if len(dp.dirtyCache.toDeletePolicies) == 0 {
		return nil, nil
	}

	toDeletePolicies := make([]string, len(dp.dirtyCache.toDeletePolicies))
	idx := 0

	for policyKey := range dp.dirtyCache.toDeletePolicies {
		toDeletePolicies[idx] = policyKey
		idx++
	}

	payload, err := controlplane.EncodeStrings(toDeletePolicies)
	if err != nil {
		klog.Errorf("processPoliciesRemove: failed to encode policies %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.RemovePolicy, false, "processPoliciesRemove: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) hydrateSetCache() (*protos.GoalState, error) {
	if len(dp.setCache) == 0 {
		return nil, nil
	}
	toApplySets := make([]*controlplane.ControllerIPSets, len(dp.setCache))
	idx := 0

	for _, set := range dp.setCache {
		toApplySets[idx] = set
		idx++
	}

	payload, err := controlplane.EncodeControllerIPSets(toApplySets)
	if err != nil {
		klog.Errorf("processIPSetsApply: failed to encode sets %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.AppendIPSet, false, "processIPSetsApply: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) hydratePolicyCache() (*protos.GoalState, error) {
	if len(dp.policyCache) == 0 {
		return nil, nil
	}

	toApplyPolicies := make([]*policies.NPMNetworkPolicy, len(dp.policyCache))
	idx := 0

	for _, policy := range dp.policyCache {
		toApplyPolicies[idx] = policy
		idx++
	}

	payload, err := controlplane.EncodeNPMNetworkPolicies(toApplyPolicies)
	if err != nil {
		klog.Errorf("processPoliciesApply: failed to encode policies %v", err)
		return nil, npmerrors.ErrorWrapper(npmerrors.AddPolicy, false, "processPoliciesApply: failed to encode sets", err)
	}

	return getGoalStateFromBuffer(payload), nil
}

func (dp *DPShim) deleteUnusedSets(stopChannel <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(time.Hour * time.Duration(cleanEmptySetsInHrs))
		defer ticker.Stop()

		for {
			select {
			case <-stopChannel:
				return
			case <-ticker.C:
				klog.Info("deleteUnusedSets: cleaning up unused sets")
				dp.checkSetReferences()
				err := dp.ApplyDataPlane()
				if err != nil {
					klog.Errorf("deleteUnusedSets: failed to apply dataplane %v", err)
				}
			}
		}
	}()
}

func (dp *DPShim) checkSetReferences() {
	for _, set := range dp.setCache {
		if !set.CanDelete() {
			continue
		}

		dp.deleteIPSet(set.IPSetMetadata)
	}
}

func getGoalStateFromBuffer(payload *bytes.Buffer) *protos.GoalState {
	return &protos.GoalState{
		Data: payload.Bytes(),
	}
}
