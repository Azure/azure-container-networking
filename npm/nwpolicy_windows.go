// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

// AddNetworkPolicy handles adding network policy to vfp.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY CREATING: %v", npObj)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	if !npMgr.isAzureNPMLayerCreated {
		if err = allNs.tMgr.CreateTag(util.KubeSystemFlag); err != nil {
			log.Errorf("Error: failed to initialize kube-system tag.")
			return err
		}

		if err = allNs.rMgr.InitAzureNPMLayer(); err != nil {
			log.Errorf("Error: failed to initialize Azure NPM VFP layer.")
			return err
		}
		npMgr.isAzureNPMLayerCreated = true
	}

	podTags, nsLists, rules := parsePolicy(npObj, allNs.tMgr)

	tMgr := allNs.tMgr
	rMgr := allNs.rMgr

	for _, tag := range podTags {
		if err = tMgr.CreateTag(tag); err != nil {
			log.Errorf("Error: failed to create tag %s-%s.", npNs, tag)
			return err
		}
	}

	for _, nlTag := range nsLists {
		if err = tMgr.CreateNLTag(nlTag); err != nil {
			log.Errorf("Error: failed to create NLTag %s-%s.", npNs, nlTag)
			return err
		}
	}

	if err = npMgr.InitAllNsList(); err != nil {
		log.Errorf("Error: failed to initialize all-namespace NLTag.")
		return err
	}

	for _, rule := range rules {
		if err = rMgr.Add(rule, tMgr); err != nil {
			log.Errorf("Error: failed to apply rule. Rule: %+v", rule)
			return err
		}
	}

	allNs.npMap[npName] = npObj

	ns, err := newNs(npNs)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", npNs)
	}
	npMgr.nsMap[npNs] = ns

	return nil
}

// DeleteNetworkPolicy handles deleting network policy from vfp.
func (npMgr *NetworkPolicyManager) DeleteNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npName := npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %v", npObj)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	_, _, rules := parsePolicy(npObj, allNs.tMgr)

	tMgr := allNs.tMgr
	rMgr := allNs.rMgr

	for _, rule := range rules {
		if err = rMgr.Delete(rule, tMgr); err != nil {
			log.Errorf("Error: failed to delete rule. Rule: %+v", rule)
			return err
		}
	}

	delete(allNs.npMap, npName)

	if len(allNs.npMap) == 0 {
		if err = rMgr.UnInitAzureNPMLayer(); err != nil {
			log.Errorf("Error: failed to uninitialize azure-npm vfp layer.")
			return err
		}
		npMgr.isAzureNPMLayerCreated = false
	}

	return nil
}
