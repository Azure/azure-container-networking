// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

// AddNetworkPolicy handles adding network policy to iptables.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY CREATING: %v", npObj)

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

	podSets, nsLists, iptEntries := parsePolicy(npObj)

	ipsMgr := allNs.ipsMgr
	for _, set := range podSets {
		if err = ipsMgr.CreateSet(set); err != nil {
			log.Errorf("Error: failed to create ipset %s-%s", npNs, set)
			return err
		}
	}

	for _, list := range nsLists {
		if err = ipsMgr.CreateList(list); err != nil {
			log.Errorf("Error: failed to create ipset list %s-%s", npNs, list)
			return err
		}
	}

	if err = npMgr.InitAllNsList(); err != nil {
		log.Errorf("Error: failed to initialize all-namespace ipset list.")
		return err
	}

	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Add(iptEntry); err != nil {
			log.Errorf("Error: failed to apply iptables rule. Rule: %+v", iptEntry)
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

// DeleteNetworkPolicy handles deleting network policy from iptables.
func (npMgr *NetworkPolicyManager) DeleteNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npName := npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %v", npObj)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	_, _, iptEntries := parsePolicy(npObj)

	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Delete(iptEntry); err != nil {
			log.Errorf("Error: failed to apply iptables rule. Rule: %+v", iptEntry)
			return err
		}
	}

	delete(allNs.npMap, npName)

	if len(allNs.npMap) == 0 {
		if err = iptMgr.UninitNpmChains(); err != nil {
			log.Errorf("Error: failed to uninitialize azure-npm chains.")
			return err
		}
		npMgr.isAzureNpmChainCreated = false
	}

	return nil
}
