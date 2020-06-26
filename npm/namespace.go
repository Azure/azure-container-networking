// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type namespace struct {
	name           string
	setMap         map[string]string
	podMap         map[types.UID]*corev1.Pod
	rawNpMap       map[string]*networkingv1.NetworkPolicy
	processedNpMap map[string]*networkingv1.NetworkPolicy
	ipsMgr         *ipsm.IpsetManager
	iptMgr         *iptm.IptablesManager
}

// newNS constructs a new namespace object.
func newNs(name string) (*namespace, error) {
	ns := &namespace{
		name:           name,
		setMap:         make(map[string]string),
		podMap:         make(map[types.UID]*corev1.Pod),
		rawNpMap:       make(map[string]*networkingv1.NetworkPolicy),
		processedNpMap: make(map[string]*networkingv1.NetworkPolicy),
		ipsMgr:         ipsm.NewIpsetManager(),
		iptMgr:         iptm.NewIptablesManager(),
	}

	return ns, nil
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
	if np, exists := ns.rawNpMap[npObj.ObjectMeta.Name]; exists {
		if isSamePolicy(np, npObj) {
			return true
		}
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

	nsName, nsLabel := "ns-"+nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Labels
	log.Printf("NAMESPACE CREATING: [%s/%v]", nsName, nsLabel)

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
		labelKey := "ns-" + nsLabelKey
		log.Printf("Adding namespace %s to ipset list %s", nsName, labelKey)
		if err = ipsMgr.AddToList(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to add namespace %s to ipset list %s", nsName, labelKey)
			return err
		}

		label := "ns-" + nsLabelKey + ":" + nsLabelVal
		log.Printf("Adding namespace %s to ipset list %s", nsName, label)
		if err = ipsMgr.AddToList(label, nsName); err != nil {
			log.Errorf("Error: failed to add namespace %s to ipset list %s", nsName, label)
			return err
		}
	}

	ns, err := newNs(nsName)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", nsName)
	}
	npMgr.nsMap[nsName] = ns

	return nil
}

// UpdateNamespace handles updating namespace in ipset.
func (npMgr *NetworkPolicyManager) UpdateNamespace(oldNsObj *corev1.Namespace, newNsObj *corev1.Namespace) error {
	if isInvalidNamespaceUpdate(oldNsObj, newNsObj) {
		return nil
	}

	var err error
	oldNsNs, oldNsLabel := "ns-"+oldNsObj.ObjectMeta.Name, oldNsObj.ObjectMeta.Labels
	newNsNs, newNsLabel := "ns-"+newNsObj.ObjectMeta.Name, newNsObj.ObjectMeta.Labels
	log.Printf(
		"NAMESPACE UPDATING:\n old namespace: [%s/%v]\n new namespace: [%s/%v]",
		oldNsNs, oldNsLabel, newNsNs, newNsLabel,
	)

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

// DeleteNamespace handles deleting namespace from ipset.
func (npMgr *NetworkPolicyManager) DeleteNamespace(nsObj *corev1.Namespace) error {
	var err error

	nsName, nsLabel := "ns-"+nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Labels
	log.Printf("NAMESPACE DELETING: [%s/%v]", nsName, nsLabel)

	_, exists := npMgr.nsMap[nsName]
	if !exists {
		return nil
	}

	// Delete the namespace from its label's ipset list.
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	nsLabels := nsObj.ObjectMeta.Labels
	for nsLabelKey, nsLabelVal := range nsLabels {
		labelKey := "ns-" + nsLabelKey
		log.Printf("Deleting namespace %s from ipset list %s", nsName, labelKey)
		if err = ipsMgr.DeleteFromList(labelKey, nsName); err != nil {
			log.Errorf("Error: failed to delete namespace %s from ipset list %s", nsName, labelKey)
			return err
		}

		label := "ns-" + nsLabelKey + ":" + nsLabelVal
		log.Printf("Deleting namespace %s from ipset list %s", nsName, label)
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
