// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type namespace struct {
	name            string
	labelsMap       map[string]string // NameSpace labels
	setMap          map[string]string
	podMap          map[string]*npmPod // Key is PodUID
	rawNpMap        map[string]*networkingv1.NetworkPolicy
	processedNpMap  map[string]*networkingv1.NetworkPolicy
	ipsMgr          *ipsm.IpsetManager
	iptMgr          *iptm.IptablesManager
	resourceVersion uint64 // NameSpace ResourceVersion
}

// newNS constructs a new namespace object.
func newNs(name string) (*namespace, error) {
	ns := &namespace{
		name:           name,
		labelsMap:      make(map[string]string),
		setMap:         make(map[string]string),
		podMap:         make(map[string]*npmPod),
		rawNpMap:       make(map[string]*networkingv1.NetworkPolicy),
		processedNpMap: make(map[string]*networkingv1.NetworkPolicy),
		ipsMgr:         ipsm.NewIpsetManager(),
		iptMgr:         iptm.NewIptablesManager(),
		// resource version is converted to uint64
		// so make sure it is initialized to "0"
		resourceVersion: 0,
	}

	return ns, nil
}

// setResourceVersion setter func for RV
func setResourceVersion(nsObj *namespace, rv string) {
	nsObj.resourceVersion = util.ParseResourceVersion(rv)
}

func isSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}

func isInvalidNamespaceUpdate(oldNsObj, newNsObj *corev1.Namespace) (isInvalidUpdate bool) {
	isInvalidUpdate = oldNsObj.ObjectMeta.Name == newNsObj.ObjectMeta.Name &&
		newNsObj.ObjectMeta.DeletionTimestamp == nil &&
		newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil
	isInvalidUpdate = isInvalidUpdate && reflect.DeepEqual(oldNsObj.ObjectMeta.Labels, newNsObj.ObjectMeta.Labels)

	return
}

func (ns *namespace) policyExists(npObj *networkingv1.NetworkPolicy) bool {
	np, exists := ns.rawNpMap[npObj.ObjectMeta.Name]
	if !exists {
		return false
	}

	if !util.CompareResourceVersions(np.ObjectMeta.ResourceVersion, npObj.ObjectMeta.ResourceVersion) {
		log.Logf("Cached Network Policy has larger ResourceVersion number than new Obj. Name: %s Cached RV: %d New RV: %d\n",
			npObj.ObjectMeta.Name,
			np.ObjectMeta.ResourceVersion,
			npObj.ObjectMeta.ResourceVersion,
		)
		return true
	}

	if isSamePolicy(np, npObj) {
		return true
	}

	return false
}

// InitAllNsList syncs all-namespace ipset list.
func (npMgr *NetworkPolicyManager) InitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	for ns := range npMgr.nsMap {
		if ns == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.ipsMgr.AddToList(util.KubeAllNamespacesFlag, ns); err != nil {
			log.Errorf("Error: failed to add namespace set %s to ipset list %s", ns, util.KubeAllNamespacesFlag)
			return err
		}
	}

	return nil
}

// UninitAllNsList cleans all-namespace ipset list.
func (npMgr *NetworkPolicyManager) UninitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	for ns := range npMgr.nsMap {
		if ns == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.ipsMgr.DeleteFromList(util.KubeAllNamespacesFlag, ns); err != nil {
			log.Errorf("Error: failed to delete namespace set %s from list %s", ns, util.KubeAllNamespacesFlag)
			return err
		}
	}

	return nil
}

// AddNamespace handles adding namespace to ipset.
func (npMgr *NetworkPolicyManager) AddNamespace(nsObj *corev1.Namespace) error {
	var err error

	nsName, nsLabel := util.GetNSNameWithPrefix(nsObj.ObjectMeta.Name), nsObj.ObjectMeta.Labels
	log.Logf("NAMESPACE CREATING: [%s/%v]", nsName, nsLabel)

	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	// Create ipset for the namespace.
	if err = ipsMgr.CreateSet(nsName, append([]string{util.IpsetNetHashFlag})); err != nil {
		log.Errorf("Error: failed to create ipset for namespace %s.", nsName)
		return err
	}

	if err = ipsMgr.AddToList(util.KubeAllNamespacesFlag, nsName); err != nil {
		log.Errorf("Error: failed to add %s to all-namespace ipset list.", nsName)
		return err
	}

	// Add the namespace to its label's ipset list.
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabelKey)
		log.Logf("Adding namespace %s to ipset list %s", nsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to add namespace %s to ipset list %s", nsName, labelKey)
			return err
		}

		label := util.GetNSNameWithPrefix(nsLabelKey + ":" + nsLabelVal)
		log.Logf("Adding namespace %s to ipset list %s", nsName, label)
		if err = ipsMgr.AddToList(label, nsName); err != nil {
			log.Errorf("Error: failed to add namespace %s to ipset list %s", nsName, label)
			return err
		}
	}

	ns, err := newNs(nsName)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", nsName)
	}
	setResourceVersion(ns, nsObj.GetObjectMeta().GetResourceVersion())

	// Append all labels to the cache NS obj
	ns.labelsMap = util.AppendMap(ns.labelsMap, nsLabel)
	npMgr.nsMap[nsName] = ns

	return nil
}

// UpdateNamespace handles updating namespace in ipset.
func (npMgr *NetworkPolicyManager) UpdateNamespace(oldNsObj *corev1.Namespace, newNsObj *corev1.Namespace) error {
	if isInvalidNamespaceUpdate(oldNsObj, newNsObj) {
		return nil
	}

	var err error
	oldNsNs, oldNsLabel := util.GetNSNameWithPrefix(oldNsObj.ObjectMeta.Name), oldNsObj.ObjectMeta.Labels
	newNsNs, newNsLabel := util.GetNSNameWithPrefix(newNsObj.ObjectMeta.Name), newNsObj.ObjectMeta.Labels
	log.Logf(
		"NAMESPACE UPDATING:\n old namespace: [%s/%v]\n new namespace: [%s/%v]",
		oldNsNs, oldNsLabel, newNsNs, newNsLabel,
	)

	if oldNsNs != newNsNs {
		if err = npMgr.DeleteNamespace(oldNsObj); err != nil {
			return err
		}

		if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
			if err = npMgr.AddNamespace(newNsObj); err != nil {
				return err
			}
		}

		return nil
	}

	// If orignal AddNamespace failed for some reason, then NS will not be found
	// in nsMap, resulting in retry of ADD.
	curNsObj, exists := npMgr.nsMap[newNsNs]
	if !exists {
		if newNsObj.ObjectMeta.DeletionTimestamp == nil && newNsObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
			if err = npMgr.AddNamespace(newNsObj); err != nil {
				return err
			}
		}

		return nil
	}

	newRv := util.ParseResourceVersion(newNsObj.ObjectMeta.ResourceVersion)
	if !util.CompareUintResourceVersions(curNsObj.resourceVersion, newRv) {
		log.Logf("Cached NameSpace has larger ResourceVersion number than new Obj. NameSpace: %s Cached RV: %d New RV:\n",
			oldNsNs,
			curNsObj.resourceVersion,
			newRv,
		)
		return nil
	}

	//if no change in labels then return
	if reflect.DeepEqual(curNsObj.labelsMap, newNsLabel) {
		log.Logf(
			"NAMESPACE UPDATING:\n nothing to delete or add. old namespace: [%s/%v]\n cache namespace: [%s/%v] new namespace: [%s/%v]",
			oldNsNs, oldNsLabel, curNsObj.name, curNsObj.labelsMap, newNsNs, newNsLabel,
		)
		return nil
	}

	//If the Namespace is not deleted, delete removed labels and create new labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(curNsObj.labelsMap, newNsLabel)

	// Delete the namespace from its label's ipset list.
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	for _, nsLabelVal := range deleteFromIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Deleting namespace %s from ipset list %s", oldNsNs, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, oldNsNs); err != nil {
			log.Errorf("Error: failed to delete namespace %s from ipset list %s", oldNsNs, labelKey)
			return err
		}
	}

	// Add the namespace to its label's ipset list.
	for _, nsLabelVal := range addToIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Adding namespace %s to ipset list %s", oldNsNs, labelKey)
		if err = ipsMgr.AddToList(labelKey, oldNsNs); err != nil {
			log.Errorf("Error: failed to add namespace %s to ipset list %s", oldNsNs, labelKey)
			return err
		}
	}

	// Append all labels to the cache NS obj
	curNsObj.labelsMap = util.ClearAndAppendMap(curNsObj.labelsMap, newNsLabel)
	setResourceVersion(curNsObj, newNsObj.GetObjectMeta().GetResourceVersion())
	npMgr.nsMap[newNsNs] = curNsObj

	return nil
}

// DeleteNamespace handles deleting namespace from ipset.
func (npMgr *NetworkPolicyManager) DeleteNamespace(nsObj *corev1.Namespace) error {
	var err error

	nsName, nsLabel := util.GetNSNameWithPrefix(nsObj.ObjectMeta.Name), nsObj.ObjectMeta.Labels
	log.Logf("NAMESPACE DELETING: [%s/%v]", nsName, nsLabel)

	cachedNsObj, exists := npMgr.nsMap[nsName]
	if !exists {
		return nil
	}

	log.Logf("NAMESPACE DELETING cached labels: [%s/%v]", nsName, cachedNsObj.labelsMap)
	// Delete the namespace from its label's ipset list.
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	nsLabels := cachedNsObj.labelsMap
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabelKey)
		log.Logf("Deleting namespace %s from ipset list %s", nsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to delete namespace %s from ipset list %s", nsName, labelKey)
			return err
		}

		label := util.GetNSNameWithPrefix(nsLabelKey + ":" + nsLabelVal)
		log.Logf("Deleting namespace %s from ipset list %s", nsName, label)
		if err = ipsMgr.DeleteFromList(label, nsName); err != nil {
			log.Errorf("Error: failed to delete namespace %s from ipset list %s", nsName, label)
			return err
		}
	}

	// Delete the namespace from all-namespace ipset list.
	if err = ipsMgr.DeleteFromList(util.KubeAllNamespacesFlag, nsName); err != nil {
		log.Errorf("Error: failed to delete namespace %s from ipset list %s", nsName, util.KubeAllNamespacesFlag)
		return err
	}

	// Delete ipset for the namespace.
	if err = ipsMgr.DeleteSet(nsName); err != nil {
		log.Errorf("Error: failed to delete ipset for namespace %s.", nsName)
		return err
	}

	delete(npMgr.nsMap, nsName)

	return nil
}
