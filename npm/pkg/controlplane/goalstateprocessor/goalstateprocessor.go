package goalstateprocessor

import (
	"bytes"
	"context"
	"fmt"

	cp "github.com/Azure/azure-container-networking/npm/pkg/controlplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

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
	dp dataplane.GenericDataplane) *GoalStateProcessor {
	klog.Infof("Creating GoalStateProcessor for node %s", nodeID)
	return &GoalStateProcessor{
		ctx:            ctx,
		nodeID:         nodeID,
		podName:        podName,
		dp:             dp,
		inputChannel:   inputChan,
		backoffChannel: make(chan *protos.Events),
	}
}

// Start kicks off the GoalStateProcessor
func (gsp *GoalStateProcessor) Start() {
	klog.Infof("Starting GoalStateProcessor for node %s", gsp.nodeID)
	go gsp.run()
}

// Stop stops the GoalStateProcessor
func (gsp *GoalStateProcessor) Stop() {
	klog.Infof("Stopping GoalStateProcessor for node %s", gsp.nodeID)
	gsp.cancel()
}

func (gsp *GoalStateProcessor) run() {
	klog.Infof("Starting dataplane for node %s", gsp.nodeID)

	for {
		gsp.processNext()
	}
}

func (gsp *GoalStateProcessor) processNext() {
	select {
	case <-gsp.ctx.Done():
		klog.Infof("GoalStateProcessor for node %s stopped", gsp.nodeID)
		return
	case inputEvents := <-gsp.inputChannel:
		// TODO remove this large print later
		klog.Infof("Received event %s", inputEvents)
		gsp.process(inputEvents)
	case backoffEvents := <-gsp.backoffChannel:
		// For now keep it simple. Do not worry about backoff events
		// but if we need to handle them, we can do it here.
		// TODO remove this large print later
		klog.Infof("Received backoff event %s", backoffEvents)
		gsp.process(backoffEvents)
	default:
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

	// Process these individual buckkets in order
	// 1. Apply IPSET
	// 2. Apply POLICY
	// 3. Remove POLICY
	// 4. Remove IPSET

	// TODO need to handle first connect stream of all GoalStates
	payload := inputEvent.GetPayload()

	if !validatePayload(payload) {
		klog.Warningf("Empty payload in event %s", inputEvent)
		return
	}

	if ipsetApplyPayload, ok := payload[cp.IpsetApply]; ok {
		err := gsp.processIPSetsApplyEvent(ipsetApplyPayload)
		if err != nil {
			klog.Errorf("Error processing IPSET apply event %s", err)
		}
	}

	if policyApplyPayload, ok := payload[cp.PolicyApply]; ok {
		err := gsp.processPolicyApplyEvent(policyApplyPayload)
		if err != nil {
			klog.Errorf("Error processing POLICY apply event %s", err)
		}
	}

	if policyRemovePayload, ok := payload[cp.PolicyRemove]; ok {
		err := gsp.processPolicyRemoveEvent(policyRemovePayload)
		if err != nil {
			klog.Errorf("Error processing POLICY remove event %s", err)
		}
	}

	if ipsetRemovePayload, ok := payload[cp.IpsetRemove]; ok {
		err := gsp.processIPSetsRemoveEvent(ipsetRemovePayload)
		if err != nil {
			klog.Errorf("Error processing IPSET remove event %s", err)
		}
	}
}

func (gsp *GoalStateProcessor) processIPSetsApplyEvent(goalState *protos.GoalState) error {
	for _, gs := range goalState.GetData() {
		payload := bytes.NewBuffer(gs)
		ipset, err := cp.DecodeControllerIPSet(payload)
		if err != nil {
			return npmerrors.SimpleErrorWrapper("failed to decode IPSet apply event", err)
		}

		if ipset == nil {
			klog.Warningf("Empty IPSet apply event")
			continue
		}

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
				return err
			}
		case ipsets.ListSet:
			err = gsp.applyLists(ipset, cachedIPSet)
			if err != nil {
				return err
			}
		case ipsets.UnknownKind:
			return npmerrors.SimpleError(
				fmt.Sprintf("failed to decode IPSet apply event: Unknown IPSet kind %s", cachedIPSet.Kind),
			)
		}
	}
	return nil
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
		for podIP, podKey := range cachedIPSet.IPPodKey {
			if _, ok := ipSet.IPPodMetadata[podIP]; !ok {
				err := gsp.dp.RemoveFromSets([]*ipsets.IPSetMetadata{setMetadata}, dataplane.NewPodMetadata(podIP, podKey, ""))
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

func (gsp *GoalStateProcessor) processIPSetsRemoveEvent(goalState *protos.GoalState) error {
	for _, gs := range goalState.GetData() {
		payload := bytes.NewBuffer(gs)
		ipsetName, err := cp.DecodeString(payload)
		if err != nil {
			return npmerrors.SimpleErrorWrapper("failed to decode IPSet remove event", err)
		}
		if ipsetName == "" {
			klog.Warningf("Empty IPSet remove event")
			continue
		}
		klog.Infof("Processing %s IPSET remove event", ipsetName)

		cachedIPSet := gsp.dp.GetIPSet(ipsetName)
		if cachedIPSet == nil {
			klog.Infof("IPSet %s not found in cache, ignoring delete call.", ipsetName)
			return nil
		}

		gsp.dp.DeleteIPSet(ipsets.NewIPSetMetadata(cachedIPSet.Name, cachedIPSet.Type))
	}
	return nil
}

func (gsp *GoalStateProcessor) processPolicyApplyEvent(goalState *protos.GoalState) error {
	for _, gs := range goalState.GetData() {
		payload := bytes.NewBuffer(gs)
		netpol, err := cp.DecodeNPMNetworkPolicy(payload)
		if err != nil {
			return npmerrors.SimpleErrorWrapper("failed to decode Policy apply event", err)
		}

		if netpol == nil {
			klog.Warningf("Empty Policy apply event")
			continue
		}
		klog.Infof("Processing %s Policy ADD event", netpol.Name)

		err = gsp.dp.UpdatePolicy(netpol)
		if err != nil {
			klog.Errorf("Error applying policy %s to dataplane with error: %s", netpol.Name, err.Error())
			return npmerrors.SimpleErrorWrapper("failed update policy event", err)
		}
	}
	return nil
}

func (gsp *GoalStateProcessor) processPolicyRemoveEvent(goalState *protos.GoalState) error {
	for _, gs := range goalState.GetData() {
		payload := bytes.NewBuffer(gs)
		netpolName, err := cp.DecodeString(payload)
		if err != nil {
			return npmerrors.SimpleErrorWrapper("failed to decode Policy remove event", err)
		}
		klog.Infof("Processing %s Policy remove event", netpolName)

		if netpolName == "" {
			klog.Warningf("Empty Policy remove event")
			continue
		}

		err = gsp.dp.RemovePolicy(netpolName)
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
