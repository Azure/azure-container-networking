// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

// AddNetworkPolicy handles adding network policy to hcn.
func (npMgr *NetworkPolicyManager) AddNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npNs, npName := npObj.ObjectMeta.Namespace, npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY CREATING: %v", npObj)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	if err = allNs.tMgr.CreateTag(util.KubeSystemFlag); err != nil {
		log.Errorf("Error: failed to initialize kube-system tag.")
		return err
	}

	podTags, nsLists, aclPolicies := parsePolicy(npObj)

	tMgr := allNs.tMgr
	for _, tag := range podTags {
		if err = tMgr.CreateTag(tag); err != nil {
			log.Errorf("Error: failed to create tag %s-%s", npNs, tag)
			return err
		}
	}

	for _, nlTag := range nsLists {
		if err = tMgr.CreateNLTag(nlTag); err != nil {
			log.Errorf("Error: failed to create NLTag %s-%s", npNs, nlTag)
			return err
		}
	}

	if err = npMgr.InitAllNsList(); err != nil {
		log.Errorf("Error: failed to initialize all-namespace NLTag.")
		return err
	}

	aclMgr := allNs.aclMgr
	for _, aclPolicy := range aclPolicies {
		if err = aclMgr.Add(aclPolicy); err != nil {
			log.Errorf("Error: failed to apply ACL policy. Policy: %+v", aclPolicy)
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

// DeleteNetworkPolicy handles deleting network policy from hcn.
func (npMgr *NetworkPolicyManager) DeleteNetworkPolicy(npObj *networkingv1.NetworkPolicy) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	npName := npObj.ObjectMeta.Name
	log.Printf("NETWORK POLICY DELETING: %v", npObj)

	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]

	_, _, aclPolicies := parsePolicy(npObj)

	aclMgr := allNs.aclMgr
	for _, aclPolicy := range aclPolicies {
		if err = aclMgr.Delete(aclPolicy); err != nil {
			log.Errorf("Error: failed to delete ACL policy. Policy: %+v", aclPolicy)
			return err
		}
	}

	delete(allNs.npMap, npName)

	return nil
}
