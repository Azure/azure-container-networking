// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"strconv"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

func (npMgr *NetworkPolicyManager) canCleanUpNpmChains() bool {
	if !npMgr.isSafeToCleanUpAzureNpmChain {
		return false
	}

	for _, ns := range npMgr.nsMap {
		if len(ns.processedNpMap) > 0 {
			return false
		}
	}

	return true
}

// AddNetworkPolicy handles adding network policy to iptables.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	var (
		err    error
		ns     *namespace
		exists bool
		npNs   = "ns-" + npObj.ObjectMeta.Namespace
		npName = npObj.ObjectMeta.Name
		allNs  = npMgr.nsMap[util.KubeAllNamespacesFlag]
	)

	log.Printf("NETWORK POLICY CREATING: %v", npObj)

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

	if !npMgr.isAzureNpmChainCreated {
		if err = allNs.ipsMgr.CreateSet(util.KubeSystemFlag, append([]string{util.IpsetNetHashFlag})); err != nil {
			log.Errorf("Error: failed to initialize kube-system ipset.")
			return err
		}

		if err = allNs.iptMgr.InitNpmChains(); err != nil {
			log.Errorf("Error: failed to initialize azure-npm chains.")
			return err
		}

		npMgr.isAzureNpmChainCreated = true
	}

	var (
		hashedSelector                   = HashSelector(&npObj.Spec.PodSelector)
		addedPolicy                      *networkingv1.NetworkPolicy
		sets, namedPorts, lists          []string
		ingressIPCidrs, egressIPCidrs    [][]string
		iptEntries                       []*iptm.IptEntry
		ipsMgr                           = allNs.ipsMgr
	)

	// Remove the existing policy from processed (merged) network policy map
	if oldPolicy, oldPolicyExists := ns.rawNpMap[npObj.ObjectMeta.Name]; oldPolicyExists {
		npMgr.isSafeToCleanUpAzureNpmChain = false
		npMgr.DeleteNetworkPolicy(oldPolicy)
		npMgr.isSafeToCleanUpAzureNpmChain = true
	}

	// Add (merge) the new policy with others who apply to the same pods
	if oldPolicy, oldPolicyExists := ns.processedNpMap[hashedSelector]; oldPolicyExists {
		addedPolicy, err = addPolicy(oldPolicy, npObj)
		if err != nil {
			log.Printf("Error adding policy %s to %s", npName, oldPolicy.ObjectMeta.Name)
		}
	}

	if addedPolicy != nil {
		ns.processedNpMap[hashedSelector] = addedPolicy
	} else {
		ns.processedNpMap[hashedSelector] = npObj
	}

	ns.rawNpMap[npObj.ObjectMeta.Name] = npObj

	sets, namedPorts, lists, ingressIPCidrs, egressIPCidrs, iptEntries = translatePolicy(npObj)
	for _, set := range sets {
		log.Printf("Creating set: %v, hashedSet: %v", set, util.GetHashedName(set))
		if err = ipsMgr.CreateSet(set, append([]string{util.IpsetNetHashFlag})); err != nil {
			log.Printf("Error creating ipset %s", set)
		}
	}
	for _, set := range namedPorts {
		log.Printf("Creating set: %v, hashedSet: %v", set, util.GetHashedName(set))
		if err = ipsMgr.CreateSet(set, append([]string{util.IpsetIPPortHashFlag})); err != nil {
			log.Printf("Error creating ipset named port %s", set)
		}
	}
	for _, list := range lists {
		if err = ipsMgr.CreateList(list); err != nil {
			log.Printf("Error creating ipset list %s", list)
		}
	}
	if err = npMgr.InitAllNsList(); err != nil {
		log.Printf("Error initializing all-namespace ipset list.")
	}
	createCidrsRule("in", npObj.ObjectMeta.Name, npObj.ObjectMeta.Namespace, ingressIPCidrs, ipsMgr)
	createCidrsRule("out", npObj.ObjectMeta.Name, npObj.ObjectMeta.Namespace, egressIPCidrs, ipsMgr)
	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Add(iptEntry); err != nil {
			log.Errorf("Error: failed to apply iptables rule. Rule: %+v", iptEntry)
		}
	}

	return nil
}

// UpdateNetworkPolicy handles updateing network policy in iptables.
func (npMgr *NetworkPolicyManager) UpdateNetworkPolicy(oldNpObj *networkingv1.NetworkPolicy, newNpObj *networkingv1.NetworkPolicy) error {
	if newNpObj.ObjectMeta.DeletionTimestamp == nil && newNpObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		log.Printf("NETWORK POLICY UPDATING:\n old policy:[%v]\n new policy:[%v]", oldNpObj, newNpObj)
		return npMgr.AddNetworkPolicy(newNpObj)
	}

	return nil
}

// DeleteNetworkPolicy handles deleting network policy from iptables.
func (npMgr *NetworkPolicyManager) DeleteNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	var (
		err   error
		ns    *namespace
		allNs = npMgr.nsMap[util.KubeAllNamespacesFlag]
	)

	npNs, npName := "ns-"+npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %v", npObj)

	var exists bool
	if ns, exists = npMgr.nsMap[npNs]; !exists {
		ns, err = newNs(npName)
		if err != nil {
			log.Printf("Error creating namespace %s", npNs)
		}
		npMgr.nsMap[npNs] = ns
	}

	_, _, _, ingressIPCidrs, egressIPCidrs, iptEntries := translatePolicy(npObj)

	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Delete(iptEntry); err != nil {
			log.Errorf("Error: failed to apply iptables rule. Rule: %+v", iptEntry)
		}
	}

	removeCidrsRule("in", npObj.ObjectMeta.Name, npObj.ObjectMeta.Namespace, ingressIPCidrs, allNs.ipsMgr)
	removeCidrsRule("out", npObj.ObjectMeta.Name, npObj.ObjectMeta.Namespace, egressIPCidrs, allNs.ipsMgr)

	delete(ns.rawNpMap, npObj.ObjectMeta.Name)

	hashedSelector := HashSelector(&npObj.Spec.PodSelector)
	if oldPolicy, oldPolicyExists := ns.processedNpMap[hashedSelector]; oldPolicyExists {
		deductedPolicy, err := deductPolicy(oldPolicy, npObj)
		if err != nil {
			log.Printf("Error deducting policy %s from %s", npName, oldPolicy.ObjectMeta.Name)
		}

		if deductedPolicy == nil {
			delete(ns.processedNpMap, hashedSelector)
		} else {
			ns.processedNpMap[hashedSelector] = deductedPolicy
		}
	}

	if npMgr.canCleanUpNpmChains() {
		npMgr.isAzureNpmChainCreated = false
		if err = iptMgr.UninitNpmChains(); err != nil {
			log.Errorf("Error: failed to uninitialize azure-npm chains.")
			return err
		}
	}

	return nil
}

func createCidrsRule(ingressOrEgress, policyName, ns string, ipsetEntries [][]string, ipsMgr *ipsm.IpsetManager) {
	spec := append([]string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum})
	for i, ipCidrSet := range ipsetEntries {
		if ipCidrSet == nil || len(ipCidrSet) == 0 {
			continue
		}
		setName := policyName + "-in-ns-" + ns + "-" + strconv.Itoa(i) + ingressOrEgress
		log.Printf("Creating set: %v, hashedSet: %v", setName, util.GetHashedName(setName))
		if err := ipsMgr.CreateSet(setName, spec); err != nil {
			log.Printf("Error creating ipset %s", ipCidrSet)
		}
		for _, ipCidrEntry := range util.DropEmptyFields(ipCidrSet) {
			if err := ipsMgr.AddToSet(setName, ipCidrEntry, util.IpsetNetHashFlag); err != nil {
				log.Printf("Error adding ip cidrs %s into ipset %s", ipCidrEntry, ipCidrSet)
			}
		}
	}
}

func removeCidrsRule(ingressOrEgress, policyName, ns string, ipsetEntries [][]string, ipsMgr *ipsm.IpsetManager) {
	for i, ipCidrSet := range ipsetEntries {
		if ipCidrSet == nil || len(ipCidrSet) == 0 {
			continue
		}
		setName := policyName + "-in-ns-" + ns + "-" + strconv.Itoa(i) + ingressOrEgress
		log.Printf("Delete set: %v, hashedSet: %v", setName, util.GetHashedName(setName))
		if err := ipsMgr.DeleteSet(setName); err != nil {
			log.Printf("Error deleting ipset %s", ipCidrSet)
		}
	}
}