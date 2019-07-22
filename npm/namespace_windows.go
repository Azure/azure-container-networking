// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/hnsm"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type namespace struct {
	name   string
	setMap map[string]string
	podMap map[types.UID]*corev1.Pod
	npMap  map[string]*networkingv1.NetworkPolicy
	tMgr   *hnsm.TagManager
	aclMgr *hnsm.ACLPolicyManager
}

// newNS constructs a new namespace object.
func newNs(name string) (*namespace, error) {
	ns := &namespace{
		name:   name,
		setMap: make(map[string]string),
		podMap: make(map[types.UID]*corev1.Pod),
		npMap:  make(map[string]*networkingv1.NetworkPolicy),
		tMgr:   hnsm.NewTagManager(),
		aclMgr: hnsm.NewACLPolicyManager(),
	}

	return ns, nil
}

// InitAllNsList syncs all-namespace ipset list.
func (npMgr *NetworkPolicyManager) InitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	for nsName := range npMgr.nsMap {
		if nsName == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.tMgr.AddToNLTag(util.KubeAllNamespacesFlag, nsName); err != nil {
			log.Errorf("Error: failed to add namespace set %s to list %s", nsName, util.KubeAllNamespacesFlag)
			return err
		}
	}

	return nil
}

// UninitAllNsList cleans all-namespace ipset list.
func (npMgr *NetworkPolicyManager) UninitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	for nsName := range npMgr.nsMap {
		if nsName == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.tMgr.DeleteFromNLTag(util.KubeAllNamespacesFlag, nsName); err != nil {
			log.Errorf("Error: failed to delete namespace set %s from list %s", nsName, util.KubeAllNamespacesFlag)
			return err
		}
	}

	return nil
}

// AddNamespace handles adding  namespace to ipset.
func (npMgr *NetworkPolicyManager) AddNamespace(nsObj *corev1.Namespace) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	nsName, nsNs, nsLabel := nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Namespace, nsObj.ObjectMeta.Labels
	log.Printf("NAMESPACE CREATING: [%s/%s/%+v]", nsName, nsNs, nsLabel)

	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
	// Create tag for the namespace.
	if err = tMgr.CreateTag(nsName); err != nil {
		log.Errorf("Error: failed to create tag for namespace %s.", nsName)
		return err
	}

	if err = tMgr.AddToNLTag(util.KubeAllNamespacesFlag, nsName); err != nil {
		log.Errorf("Error: failed to add %s to all-namespace tag.", nsName)
		return err
	}

	// Add the namespace to its label's tag.
	var labelKeys []string
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNsIpsetName(nsLabelKey, nsLabelVal)
		log.Printf("Adding namespace %s to tag %s", nsName, labelKey)
		if err = tMgr.AddToNLTag(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to add namespace %s to tag %s", nsName, labelKey)
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	ns, err := newNs(nsName)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", nsName)
	}
	npMgr.nsMap[nsName] = ns

	return nil
}

// DeleteNamespace handles deleting namespace from ipset.
func (npMgr *NetworkPolicyManager) DeleteNamespace(nsObj *corev1.Namespace) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	nsName, nsNs, nsLabel := nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Namespace, nsObj.ObjectMeta.Labels
	log.Printf("NAMESPACE DELETING: [%s/%s/%+v]", nsName, nsNs, nsLabel)

	_, exists := npMgr.nsMap[nsName]
	if !exists {
		return nil
	}

	// Delete the namespace from its label's tag.
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
	var labelKeys []string
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNsIpsetName(nsLabelKey, nsLabelVal)
		log.Printf("Deleting namespace %s from tag %s", nsName, labelKey)
		if err = tMgr.DeleteFromNLTag(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to delete namespace %s from tag %s", nsName, labelKey)
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	// Delete the namespace from all-namespace tag.
	if err = tMgr.DeleteFromNLTag(util.KubeAllNamespacesFlag, nsName); err != nil {
		log.Errorf("Error: failed to delete namespace %s from tag %s", nsName, util.KubeAllNamespacesFlag)
		return err
	}

	// Delete tag for the namespace.
	if err = tMgr.DeleteTag(nsName); err != nil {
		log.Errorf("Error: failed to delete tag for namespace %s.", nsName)
		return err
	}

	delete(npMgr.nsMap, nsName)

	return nil
}
