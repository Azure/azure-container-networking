// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
)

type Namespace struct {
	name            string
	LabelsMap       map[string]string // NameSpace labels
	SetMap          map[string]string
	IpsMgr          *ipsm.IpsetManager
	iptMgr          *iptm.IptablesManager
	resourceVersion uint64 // NameSpace ResourceVersion
}

// newNS constructs a new namespace object.
func newNs(name string) (*Namespace, error) {
	ns := &Namespace{
		name:      name,
		LabelsMap: make(map[string]string),
		SetMap:    make(map[string]string),
		IpsMgr:    ipsm.NewIpsetManager(),
		iptMgr:    iptm.NewIptablesManager(),
		// resource version is converted to uint64
		// so make sure it is initialized to "0"
		resourceVersion: 0,
	}

	return ns, nil
}

// setResourceVersion setter func for RV
func setResourceVersion(nsObj *Namespace, rv string) {
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

// InitAllNsList syncs all-namespace ipset list.
func (npMgr *NetworkPolicyManager) InitAllNsList() error {
	allNs := npMgr.NsMap[util.KubeAllNamespacesFlag]
	for ns := range npMgr.NsMap {
		if ns == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.IpsMgr.AddToList(util.KubeAllNamespacesFlag, ns); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[InitAllNsList] Error: failed to add namespace set %s to ipset list %s with err: %v", ns, util.KubeAllNamespacesFlag, err)
			return err
		}
	}

	return nil
}

// UninitAllNsList cleans all-namespace ipset list.
func (npMgr *NetworkPolicyManager) UninitAllNsList() error {
	allNs := npMgr.NsMap[util.KubeAllNamespacesFlag]
	for ns := range npMgr.NsMap {
		if ns == util.KubeAllNamespacesFlag {
			continue
		}

		if err := allNs.IpsMgr.DeleteFromList(util.KubeAllNamespacesFlag, ns); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UninitAllNsList] Error: failed to delete namespace set %s from list %s with err: %v", ns, util.KubeAllNamespacesFlag, err)
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

	ipsMgr := npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	// Create ipset for the namespace.
	if err = ipsMgr.CreateSet(nsName, append([]string{util.IpsetNetHashFlag})); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to create ipset for namespace %s with err: %v", nsName, err)
		return err
	}

	if err = ipsMgr.AddToList(util.KubeAllNamespacesFlag, nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add %s to all-namespace ipset list with err: %v", nsName, err)
		return err
	}

	// Add the namespace to its label's ipset list.
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabelKey)
		log.Logf("Adding namespace %s to ipset list %s", nsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", nsName, labelKey, err)
			return err
		}

		label := util.GetNSNameWithPrefix(nsLabelKey + ":" + nsLabelVal)
		log.Logf("Adding namespace %s to ipset list %s", nsName, label)
		if err = ipsMgr.AddToList(label, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", nsName, label, err)
			return err
		}
	}

	ns, err := newNs(nsName)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[AddNamespace] Error: failed to create namespace %s with err: %v", nsName, err)
		return err
	}
	setResourceVersion(ns, nsObj.GetObjectMeta().GetResourceVersion())

	// Append all labels to the cache NS obj
	ns.LabelsMap = util.AppendMap(ns.LabelsMap, nsLabel)
	npMgr.NsMap[nsName] = ns

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
	curNsObj, exists := npMgr.NsMap[newNsNs]
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
	if reflect.DeepEqual(curNsObj.LabelsMap, newNsLabel) {
		log.Logf(
			"NAMESPACE UPDATING: nothing to delete or add. namespace: [%s/%v]",
			newNsNs, newNsLabel,
		)
		return nil
	}

	//If the Namespace is not deleted, delete removed labels and create new labels
	addToIPSets, deleteFromIPSets := util.GetIPSetListCompareLabels(curNsObj.LabelsMap, newNsLabel)

	// Delete the namespace from its label's ipset list.
	ipsMgr := npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	for _, nsLabelVal := range deleteFromIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Deleting namespace %s from ipset list %s", oldNsNs, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, oldNsNs); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", oldNsNs, labelKey, err)
			return err
		}
	}

	// Add the namespace to its label's ipset list.
	for _, nsLabelVal := range addToIPSets {
		labelKey := util.GetNSNameWithPrefix(nsLabelVal)
		log.Logf("Adding namespace %s to ipset list %s", oldNsNs, labelKey)
		if err = ipsMgr.AddToList(labelKey, oldNsNs); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[UpdateNamespace] Error: failed to add namespace %s to ipset list %s with err: %v", oldNsNs, labelKey, err)
			return err
		}
	}

	// Append all labels to the cache NS obj
	curNsObj.LabelsMap = util.ClearAndAppendMap(curNsObj.LabelsMap, newNsLabel)
	setResourceVersion(curNsObj, newNsObj.GetObjectMeta().GetResourceVersion())
	npMgr.NsMap[newNsNs] = curNsObj

	return nil
}

// DeleteNamespace handles deleting namespace from ipset.
func (npMgr *NetworkPolicyManager) DeleteNamespace(nsObj *corev1.Namespace) error {
	var err error

	nsName, nsLabel := util.GetNSNameWithPrefix(nsObj.ObjectMeta.Name), nsObj.ObjectMeta.Labels
	log.Logf("NAMESPACE DELETING: [%s/%v]", nsName, nsLabel)

	cachedNsObj, exists := npMgr.NsMap[nsName]
	if !exists {
		return nil
	}

	log.Logf("NAMESPACE DELETING cached labels: [%s/%v]", nsName, cachedNsObj.LabelsMap)
	// Delete the namespace from its label's ipset list.
	ipsMgr := npMgr.NsMap[util.KubeAllNamespacesFlag].IpsMgr
	nsLabels := cachedNsObj.LabelsMap
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := util.GetNSNameWithPrefix(nsLabelKey)
		log.Logf("Deleting namespace %s from ipset list %s", nsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", nsName, labelKey, err)
			return err
		}

		label := util.GetNSNameWithPrefix(nsLabelKey + ":" + nsLabelVal)
		log.Logf("Deleting namespace %s from ipset list %s", nsName, label)
		if err = ipsMgr.DeleteFromList(label, nsName); err != nil {
			metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", nsName, label, err)
			return err
		}
	}

	// Delete the namespace from all-namespace ipset list.
	if err = ipsMgr.DeleteFromList(util.KubeAllNamespacesFlag, nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete namespace %s from ipset list %s with err: %v", nsName, util.KubeAllNamespacesFlag, err)
		return err
	}

	// Delete ipset for the namespace.
	if err = ipsMgr.DeleteSet(nsName); err != nil {
		metrics.SendErrorLogAndMetric(util.NSID, "[DeleteNamespace] Error: failed to delete ipset for namespace %s with err: %v", nsName, err)
		return err
	}

	delete(npMgr.NsMap, nsName)

	return nil
}
