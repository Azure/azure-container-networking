package goalstateprocessor

import (
	"bytes"
	"context"
	"fmt"

	cp "github.com/Azure/azure-container-networking/npm/pkg/controlplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

var ErrPodOrNodeNameNil = fmt.Errorf("both pod and node name must be set")

type GoalStateProcessor struct {
	ctx            context.Context
	cancel         context.CancelFunc
	nodeID         string
	podName        string
	dp             dataplane.GenericDataplane
	inputChannel   chan *protos.Events
	backoffChannel chan *protos.Events
}

func NewGoalStateProcessor(
	ctx context.Context,
	nodeID string,
	podName string,
	inputChan chan *protos.Events,
	dp dataplane.GenericDataplane) (*GoalStateProcessor, error) {

	if nodeID == "" || podName == "" {
		return nil, ErrPodOrNodeNameNil
	}

	klog.Infof("Creating GoalStateProcessor for node %s", nodeID)

	return &GoalStateProcessor{
		ctx:            ctx,
		nodeID:         nodeID,
		podName:        podName,
		dp:             dp,
		inputChannel:   inputChan,
		backoffChannel: make(chan *protos.Events),
	}, nil
}

// Start kicks off the GoalStateProcessor
func (gsp *GoalStateProcessor) Start(stopCh <-chan struct{}) {
	klog.Infof("Starting GoalStateProcessor for node %s", gsp.nodeID)
	go gsp.run(stopCh)
}

// Stop stops the GoalStateProcessor
func (gsp *GoalStateProcessor) Stop() {
	klog.Infof("Stopping GoalStateProcessor for node %s", gsp.nodeID)
	gsp.cancel()
}

func (gsp *GoalStateProcessor) run(stopCh <-chan struct{}) {
	klog.Infof("Starting dataplane for node %s", gsp.nodeID)

	for gsp.processNext(stopCh) {
	}
}

func (gsp *GoalStateProcessor) processNext(stopCh <-chan struct{}) bool {
	select {
	// TODO benchmark how many events can stay in pipeline as we work
	// on a previous event
	case inputEvents := <-gsp.inputChannel:
		// TODO remove this large print later
		klog.Infof("Received event %s", inputEvents)
		gsp.process(inputEvents)
		return true
	case backoffEvents := <-gsp.backoffChannel:
		// For now keep it simple. Do not worry about backoff events
		// but if we need to handle them, we can do it here.
		// TODO remove this large print later
		klog.Infof("Received backoff event %s", backoffEvents)
		gsp.process(backoffEvents)
		return true

	case <-gsp.ctx.Done():
		klog.Infof("GoalStateProcessor for node %s received context Done", gsp.nodeID)
		return false
	case <-stopCh:
		klog.Infof("GoalStateProcessor for node %s stopped", gsp.nodeID)
		return false
	}
}

func (gsp *GoalStateProcessor) process(inputEvent *protos.Events) {
	klog.Infof("Processing event")
	// apply dataplane after syncing
	defer func() {
		dperr := gsp.dp.ApplyDataPlane()
		if dperr != nil {
			klog.Errorf("Apply Dataplane failed with %v", dperr)
		}
	}()

	payload := inputEvent.GetPayload()
	if !validatePayload(payload) {
		klog.Warningf("Empty payload in event %s", inputEvent)
		return
	}

	switch inputEvent.GetEventType() {
	case protos.Events_Hydration:
		// in hydration event, any thing in local cache and not in event should be deleted.
		klog.Infof("Received hydration event")
		gsp.processHydrationEvent(payload)
	case protos.Events_GoalState:
		klog.Infof("Received goal state event")
		gsp.processGoalStateEvent(payload)
	default:
		klog.Errorf("Received unknown event type %s", inputEvent.GetEventType())
	}
}

func (gsp *GoalStateProcessor) processHydrationEvent(payload map[string]*protos.GoalState) {
	// Hydration events are sent when the daemon first starts up, or a reconnection to controller happens.
	// In this case, the controller will send a current state of the cache down to daemon.
	// Daemon will need to calculate what updates and deleted have been missed and send them to the dataplane.

	// Sequence of processing will be:
	// Apply IPsets
	// Apply Policies
	// Get all existing IPSets and policies in the dataplane
	// Delete cached Policies not in event
	// Delete cached IPSets (without references) not in the event

	var appendedIPSets map[string]struct{}
	var appendedPolicies map[string]struct{}
	var err error

	if ipsetApplyPayload, ok := payload[cp.IpsetApply]; ok {
		appendedIPSets, err = gsp.processIPSetsApplyEvent(ipsetApplyPayload)
		if err != nil {
			klog.Errorf("Error processing IPSET apply HYDRATION event %s", err)
		}
	}

	if policyApplyPayload, ok := payload[cp.PolicyApply]; ok {
		appendedPolicies, err = gsp.processPolicyApplyEvent(policyApplyPayload)
		if err != nil {
			klog.Errorf("Error processing POLICY apply HYDRATION event %s", err)
		}
	}

	cachedPolicyKeys := gsp.dp.GetAllPolicies()
	toDeletePolicies := make([]string, 0)
	if appendedPolicies == nil {
		toDeletePolicies = cachedPolicyKeys
	} else {
		for _, policy := range cachedPolicyKeys {
			if _, ok := appendedPolicies[policy]; !ok {
				toDeletePolicies = append(toDeletePolicies, policy)
			}
		}
	}

	if len(toDeletePolicies) > 0 {
		klog.Infof("Deleting %d policies", len(toDeletePolicies))
		err = gsp.processPolicyRemoveEvent(toDeletePolicies)
		if err != nil {
			klog.Errorf("Error processing POLICY remove HYDRATION event %s", err)
		}
	}

	cachedIPSetNames := gsp.dp.GetAllIPSets()
	hashedsetnames := make([]string, len(cachedIPSetNames))

	toDeleteIPSets := make([]string, 0)

	i := 0
	for name := range cachedIPSetNames {
		hashedsetnames[i] = name
		i++
	}

	if appendedIPSets == nil {
		toDeleteIPSets = hashedsetnames
	} else {
		for _, ipset := range cachedIPSetNames {
			if _, ok := appendedIPSets[ipset]; !ok {
				toDeleteIPSets = append(toDeleteIPSets, ipset)
			}
		}
	}

	if len(toDeleteIPSets) > 0 {
		klog.Infof("Deleting %d ipsets", len(toDeleteIPSets))
		gsp.processIPSetsRemoveEvent(toDeleteIPSets, util.ForceDelete)
	}
}

func (gsp *GoalStateProcessor) processGoalStateEvent(payload map[string]*protos.GoalState) {
	// Process these individual buckets in order
	// 1. Apply IPSET
	// 2. Apply POLICY
	// 3. Remove POLICY
	// 4. Remove IPSET
	if ipsetApplyPayload, ok := payload[cp.IpsetApply]; ok {
		_, err := gsp.processIPSetsApplyEvent(ipsetApplyPayload)
		if err != nil {
			klog.Errorf("Error processing IPSET apply event %s", err)
		}
	}

	if policyApplyPayload, ok := payload[cp.PolicyApply]; ok {
		_, err := gsp.processPolicyApplyEvent(policyApplyPayload)
		if err != nil {
			klog.Errorf("Error processing POLICY apply event %s", err)
		}
	}

	if policyRemovePayload, ok := payload[cp.PolicyRemove]; ok {
		payload := bytes.NewBuffer(policyRemovePayload.GetData())
		netpolNames, err := cp.DecodeStrings(payload)
		if err != nil {
			klog.Errorf("Error processing POLICY remove event, failed to decode Policy remove event %s", err)
		}
		err = gsp.processPolicyRemoveEvent(netpolNames)
		if err != nil {
			klog.Errorf("Error processing POLICY remove event %s", err)
		}
	}

	if ipsetRemovePayload, ok := payload[cp.IpsetRemove]; ok {
		payload := bytes.NewBuffer(ipsetRemovePayload.GetData())
		ipsetNames, err := cp.DecodeStrings(payload)
		if err != nil {
			klog.Errorf("Error processing IPSET remove event, failed to decode IPSet remove event: %s", err)
		}
		gsp.processIPSetsRemoveEvent(ipsetNames, util.SoftDelete)
	}
}

func (gsp *GoalStateProcessor) processIPSetsApplyEvent(goalState *protos.GoalState) (map[string]struct{}, error) {
	payload := bytes.NewBuffer(goalState.GetData())
	payloadIPSets, err := cp.DecodeControllerIPSets(payload)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode IPSet apply event", err)
	}

	klog.Infof("Processing IPSet apply event %v", payloadIPSets)
	appendedIPSets := make(map[string]struct{}, len(payloadIPSets))
	for _, ipset := range payloadIPSets {
		if ipset == nil {
			klog.Warningf("Empty IPSet apply event")
			continue
		}

		klog.Infof("ipset: %v", ipset)

		ipsetName := ipset.GetPrefixName()
		klog.Infof("Processing %s IPSET apply event", ipsetName)

		cachedIPSet := gsp.dp.GetIPSet(ipsetName)
		if cachedIPSet == nil {
			klog.Infof("IPSet %s not found in cache, adding to cache", ipsetName)
		}

		switch ipset.GetSetKind() {
		case ipsets.HashSet:
			err = gsp.applySets(ipset, cachedIPSet)
			if err != nil {
				return nil, err
			}
		case ipsets.ListSet:
			err = gsp.applyLists(ipset, cachedIPSet)
			if err != nil {
				return nil, err
			}
		case ipsets.UnknownKind:
			return nil, npmerrors.SimpleError(
				fmt.Sprintf("failed to decode IPSet apply event: Unknown IPSet kind %s", cachedIPSet.Kind),
			)
		}
		appendedIPSets[ipsetName] = struct{}{}
	}
	return appendedIPSets, nil
}

func (gsp *GoalStateProcessor) applySets(ipSet *cp.ControllerIPSets, cachedIPSet *ipsets.IPSet) error {
	if len(ipSet.IPPodMetadata) == 0 {
		gsp.dp.CreateIPSets([]*ipsets.IPSetMetadata{ipSet.GetMetadata()})
		return nil
	}

	setMetadata := ipSet.GetMetadata()
	for _, podMetadata := range ipSet.IPPodMetadata {
		err := gsp.dp.AddToSets([]*ipsets.IPSetMetadata{setMetadata}, podMetadata)
		if err != nil {
			return npmerrors.SimpleErrorWrapper("IPSet apply event, failed at AddToSet.", err)
		}
	}

	if cachedIPSet != nil {
		for podIP, cachedPodKey := range cachedIPSet.IPPodKey {
			if _, ok := ipSet.IPPodMetadata[podIP]; !ok {
				err := gsp.dp.RemoveFromSets([]*ipsets.IPSetMetadata{setMetadata}, dataplane.NewPodMetadata(podIP, cachedPodKey, ""))
				if err != nil {
					return npmerrors.SimpleErrorWrapper("IPSet apply event, failed at RemoveFromSets.", err)
				}
			}
		}
	}
	return nil
}

func (gsp *GoalStateProcessor) applyLists(ipSet *cp.ControllerIPSets, cachedIPSet *ipsets.IPSet) error {
	if len(ipSet.MemberIPSets) == 0 {
		gsp.dp.CreateIPSets([]*ipsets.IPSetMetadata{ipSet.GetMetadata()})
		return nil
	}

	setMetadata := ipSet.GetMetadata()
	membersToAdd := make([]*ipsets.IPSetMetadata, len(ipSet.MemberIPSets))
	idx := 0
	for _, memberIPSet := range ipSet.MemberIPSets {
		membersToAdd[idx] = memberIPSet
		idx++
	}
	err := gsp.dp.AddToLists([]*ipsets.IPSetMetadata{setMetadata}, membersToAdd)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("IPSet apply event, failed at AddToLists.", err)
	}

	if cachedIPSet != nil {
		toDeleteMembers := make([]*ipsets.IPSetMetadata, 0)
		for _, memberSet := range cachedIPSet.MemberIPSets {
			if _, ok := ipSet.MemberIPSets[memberSet.Name]; !ok {
				toDeleteMembers = append(toDeleteMembers, memberSet.GetSetMetadata())
			}
		}

		if len(toDeleteMembers) > 0 {
			err := gsp.dp.RemoveFromList(setMetadata, toDeleteMembers)
			if err != nil {
				return npmerrors.SimpleErrorWrapper("IPSet apply event, failed at RemoveFromList.", err)
			}
		}
	}
	return nil
}

func (gsp *GoalStateProcessor) processIPSetsRemoveEvent(ipsetNames []string, forceDelete util.DeleteOption) {
	for _, ipsetName := range ipsetNames {
		if ipsetName == "" {
			klog.Warningf("Empty IPSet remove event")
			continue
		}
		klog.Infof("Processing %s IPSET remove event", ipsetName)

		cachedIPSet := gsp.dp.GetIPSet(ipsetName)
		if cachedIPSet == nil {
			klog.Infof("IPSet %s not found in cache, ignoring delete call.", ipsetName)
			continue
		}

		gsp.dp.DeleteIPSet(ipsets.NewIPSetMetadata(cachedIPSet.Name, cachedIPSet.Type), forceDelete)
	}
}

func (gsp *GoalStateProcessor) processPolicyApplyEvent(goalState *protos.GoalState) (map[string]struct{}, error) {
	payload := bytes.NewBuffer(goalState.GetData())
	netpols, err := cp.DecodeNPMNetworkPolicies(payload)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode Policy apply event", err)
	}

	appendedPolicies := make(map[string]struct{}, len(netpols))
	for _, netpol := range netpols {
		if netpol == nil {
			klog.Warningf("Empty Policy apply event")
			continue
		}
		klog.Infof("Processing %s Policy ADD event", netpol.PolicyKey)
		klog.Infof("Netpol: %v", netpol)

		err = gsp.dp.UpdatePolicy(netpol)
		if err != nil {
			klog.Errorf("Error applying policy %s to dataplane with error: %s", netpol.PolicyKey, err.Error())
			return nil, npmerrors.SimpleErrorWrapper("failed update policy event", err)
		}
		appendedPolicies[netpol.PolicyKey] = struct{}{}
	}
	return appendedPolicies, nil
}

func (gsp *GoalStateProcessor) processPolicyRemoveEvent(netpolNames []string) error {
	for _, netpolName := range netpolNames {
		klog.Infof("Processing %s Policy remove event", netpolName)

		if netpolName == "" {
			klog.Warningf("Empty Policy remove event")
			continue
		}

		err := gsp.dp.RemovePolicy(netpolName)
		if err != nil {
			klog.Errorf("Error removing policy %s from dataplane with error: %s", netpolName, err.Error())
			return npmerrors.SimpleErrorWrapper("failed remove policy event", err)
		}
	}
	return nil
}

func validatePayload(payload map[string]*protos.GoalState) bool {
	for _, v := range payload {
		if len(v.GetData()) != 0 {
			return true
		}
	}
	return false
}
