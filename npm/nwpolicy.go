// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

func (npMgr *NetworkPolicyManager) canCleanUpNpmChains() bool {
	for _, ns := range npMgr.nsMap {
		if len(ns.rawNpMap) > 0 {
			return false
		}
	}

	return true
}

// AddNetworkPolicy handles adding network policy to iptables.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var (
		err error
		ns  *namespace
	)

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY CREATING: %v", npObj)

	var exists bool
	if ns, exists = npMgr.nsMap[npNs]; !exists {
		ns, err = newNs(npNs)
		if err != nil {
			log.Printf("Error creating namespace %s\n", npNs)
		}
		npMgr.nsMap[npNs] = ns
	}

	if ns.policyExists(npObj) {
		return nil
	}

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	if !npMgr.isAzureNpmChainCreated {
		if err = allNs.ipsMgr.CreateSet(util.KubeSystemFlag); err != nil {
			log.Errorf("Error: failed to initialize kube-system ipset.")
			return err
		}

		if err = allNs.iptMgr.InitNpmChains(); err != nil {
			log.Errorf("Error: failed to initialize azure-npm chains.")
			return err
		}

		npMgr.isAzureNpmChainCreated = true
	}

	labels, newPolicies := splitPolicy(npObj)
	var (
		addedPolicy   *networkingv1.NetworkPolicy
		oldPolicies   []*networkingv1.NetworkPolicy
		addedPolicies []*networkingv1.NetworkPolicy
	)
	for i := range newPolicies {
		label, newPolicy := labels[i], newPolicies[i]
		if oldPolicy, exists := ns.processedNpMap[label]; exists {
			addedPolicy, err = addPolicy(oldPolicy, newPolicy)
			oldPolicies = append(oldPolicies, oldPolicy)
			addedPolicies = append(addedPolicies, addedPolicy)
		} else {
			ns.processedNpMap[label] = newPolicy
			addedPolicies = append(addedPolicies, newPolicy)
		}
	}

	npMgr.Unlock()
	for _, oldPolicy := range oldPolicies {
		npMgr.DeleteNetworkPolicy(oldPolicy)
	}
	npMgr.Lock()

	for _, addedPolicy = range addedPolicies {
		sets, lists, iptEntries := translatePolicy(addedPolicy)

		ipsMgr := allNs.ipsMgr
		for _, set := range sets {
			if err = ipsMgr.CreateSet(set); err != nil {
				log.Printf("Error creating ipset %s-%s\n", npNs, set)
				return err
			}
		}

		for _, list := range lists {
			if err = ipsMgr.CreateList(list); err != nil {
				log.Printf("Error creating ipset list %s-%s\n", npNs, list)
				return err
			}
		}

		if err = npMgr.InitAllNsList(); err != nil {
			log.Printf("Error initializing all-namespace ipset list.\n")
			return err
		}

		iptMgr := allNs.iptMgr
		for _, iptEntry := range iptEntries {
			if err = iptMgr.Add(iptEntry); err != nil {
				log.Printf("Error applying iptables rule\n. Rule: %+v", iptEntry)
				return err
			}
		}

	}

	ns.rawNpMap[npName] = npObj

	return nil
}

// UpdateNetworkPolicy handles updateing network policy in iptables.
func (npMgr *NetworkPolicyManager) UpdateNetworkPolicy(oldNpObj *networkingv1.NetworkPolicy, newNpObj *networkingv1.NetworkPolicy) error {
	var err error

	log.Printf("NETWORK POLICY UPDATING:\n old policy:[%v]\n new policy:[%v]", oldNpObj, newNpObj)

	if err = npMgr.DeleteNetworkPolicy(oldNpObj); err != nil {
		return err
	}

	if newNpObj.ObjectMeta.DeletionTimestamp == nil && newNpObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		if err = npMgr.AddNetworkPolicy(newNpObj); err != nil {
			return err
		}
	}

	return nil
}

// DeleteNetworkPolicy handles deleting network policy from iptables.
func (npMgr *NetworkPolicyManager) DeleteNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var (
		err error
		ns  *namespace
	)

	npName := npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %v", npObj)

	var exists bool
	if ns, exists = npMgr.nsMap[npName]; !exists {
		ns, err = newNs(npName)
		if err != nil {
			log.Printf("Error creating namespace %s\n", npName)
		}
		npMgr.nsMap[npName] = ns
	}

	if !ns.policyExists(npObj) {
		return nil
	}

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	_, _, iptEntries := translatePolicy(npObj)

	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Delete(iptEntry); err != nil {
			log.Errorf("Error: failed to apply iptables rule. Rule: %+v", iptEntry)
			return err
		}
	}

	delete(ns.rawNpMap, npName)

	if npMgr.canCleanUpNpmChains() {
		if err = iptMgr.UninitNpmChains(); err != nil {
			log.Errorf("Error: failed to uninitialize azure-npm chains.")
			return err
		}
		npMgr.isAzureNpmChainCreated = false
	}

	return nil
}
