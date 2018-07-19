// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

// AddNetworkPolicy handles adding network policy to iptables.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	defer func() {
		if err = npMgr.UpdateAndSendReport(err, util.AddNetworkPolicyEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
	}()

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY CREATING: %s/%s\n", npNs, npName)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	if !npMgr.isAzureNpmChainCreated {
		if err = allNs.ipsMgr.CreateSet(util.KubeSystemFlag); err != nil {
			log.Printf("Error initialize kube-system ipset.\n")
			return err
		}

		if err = allNs.iptMgr.InitNpmChains(); err != nil {
			log.Printf("Error initialize azure-npm chains.\n")
			return err
		}

		npMgr.isAzureNpmChainCreated = true
	}

	podSets, nsLists, iptEntries := parsePolicy(npObj)

	ipsMgr := allNs.ipsMgr
	for _, set := range podSets {
		if err = ipsMgr.CreateSet(set); err != nil {
			log.Printf("Error creating ipset %s-%s\n", npNs, set)
			return err
		}
	}

	for _, list := range nsLists {
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

	allNs.npMap[npName] = npObj

	npMgr.clusterState.NwPolicyCount++

	ns, err := newNs(npNs)
	if err != nil {
		log.Printf("Error creating namespace %s\n", npNs)
	}
	npMgr.nsMap[npNs] = ns

	return nil
}

// UpdateNetworkPolicy handles updateing network policy in iptables.
func (npMgr *NetworkPolicyManager) UpdateNetworkPolicy(oldNpObj *networkingv1.NetworkPolicy, newNpObj *networkingv1.NetworkPolicy) error {
	var err error

	defer func() {
		npMgr.Lock()
		if err = npMgr.UpdateAndSendReport(err, util.UpdateNetworkPolicyEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
		npMgr.Unlock()
	}()

	oldNpNs, oldNpName := oldNpObj.ObjectMeta.Namespace, oldNpObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY UPDATING: %s/%s\n", oldNpNs, oldNpName)

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

	var err error

	defer func() {
		if err = npMgr.UpdateAndSendReport(err, util.DeleteNetworkPolicyEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
	}()

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %s/%s\n", npNs, npName)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	_, _, iptEntries := parsePolicy(npObj)

	iptMgr := allNs.iptMgr
	for _, iptEntry := range iptEntries {
		if err = iptMgr.Delete(iptEntry); err != nil {
			log.Printf("Error applying iptables rule.\n Rule: %+v", iptEntry)
			return err
		}
	}

	delete(allNs.npMap, npName)

	npMgr.clusterState.NwPolicyCount--

	if len(allNs.npMap) == 0 {
		if err = iptMgr.UninitNpmChains(); err != nil {
			log.Printf("Error uninitialize azure-npm chains.\n")
			return err
		}
		npMgr.isAzureNpmChainCreated = false
	}

	return nil
}
