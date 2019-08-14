// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/kalebmorris/azure-container-networking/npm/vfpm"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type namespace struct {
	name   string
	setMap map[string]string
	podMap map[types.UID]*corev1.Pod
	npMap  map[string]*networkingv1.NetworkPolicy
	tMgr   *vfpm.TagManager
	rMgr   *vfpm.RuleManager
}

// newNS constructs a new namespace object.
func newNs(name string) (*namespace, error) {
	ns := &namespace{
		name:   name,
		setMap: make(map[string]string),
		podMap: make(map[types.UID]*corev1.Pod),
		npMap:  make(map[string]*networkingv1.NetworkPolicy),
		tMgr:   vfpm.NewTagManager(),
		rMgr:   vfpm.NewRuleManager(),
	}

	return ns, nil
}

// InitAllNsList syncs all-namespace tag.
func (npMgr *NetworkPolicyManager) InitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	for nsName := range npMgr.nsMap {
		if nsName == util.KubeAllNamespacesFlag {
			continue
		}

		for _, portName := range ports {
			if err := allNs.tMgr.AddToNLTag(util.KubeAllNamespacesFlag, nsName, portName); err != nil {
				log.Errorf("Error: failed to add Tag %s to NLTag %s on port %s", nsName, util.KubeAllNamespacesFlag, portName)
				return err
			}
		}
	}

	return nil
}

// UninitAllNsList cleans all-namespace tag.
func (npMgr *NetworkPolicyManager) UninitAllNsList() error {
	allNs := npMgr.nsMap[util.KubeAllNamespacesFlag]
	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	for nsName := range npMgr.nsMap {
		if nsName == util.KubeAllNamespacesFlag {
			continue
		}

		for _, portName := range ports {
			if err := allNs.tMgr.DeleteFromNLTag(util.KubeAllNamespacesFlag, nsName, portName); err != nil {
				log.Errorf("Error: failed to delete Tag %s from NLTag %s on port %s", nsName, util.KubeAllNamespacesFlag, portName)
				return err
			}
		}
	}

	return nil
}

// AddNamespace handles adding namespace to tag.
func (npMgr *NetworkPolicyManager) AddNamespace(nsObj *corev1.Namespace) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	var err error

	nsName, nsNs, nsLabel := nsObj.ObjectMeta.Name, nsObj.ObjectMeta.Namespace, nsObj.ObjectMeta.Labels
	log.Printf("NAMESPACE CREATING: [%s/%s/%+v]", nsName, nsNs, nsLabel)

	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr

	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	for _, portName := range ports {
		// Create tag for the namespace.
		if err = tMgr.CreateTag(nsName, portName); err != nil {
			log.Errorf("Error: failed to create tag for namespace %s on port %s.", nsName)
			return err
		}

		if err = tMgr.AddToNLTag(util.KubeAllNamespacesFlag, nsName, portName); err != nil {
			log.Errorf("Error: failed to add %s to all-namespace tag on port %s.", nsName, portName)
			return err
		}

		// Add the namespace to its label's tag.
		nsLabels := nsObj.ObjectMeta.Labels
		for nsLabelKey, nsLabelVal := range nsLabels {
			labelKey := util.GetNsIpsetName(nsLabelKey, nsLabelVal)
			if err = tMgr.AddToNLTag(labelKey, nsName, portName); err != nil {
				log.Errorf("Error: failed to add namespace %s to tag %s on port %s.", nsName, labelKey, portName)
				return err
			}
		}
	}

	ns, err := newNs(nsName)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", nsName)
	}
	npMgr.nsMap[nsName] = ns

	return nil
}

// DeleteNamespace handles deleting namespace from tag.
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

	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	for _, portName := range ports {
		// Delete the namespace from its label's tag.
		tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
		nsLabels := nsObj.ObjectMeta.Labels
		for nsLabelKey, nsLabelVal := range nsLabels {
			labelKey := util.GetNsIpsetName(nsLabelKey, nsLabelVal)
			log.Printf("Deleting namespace %s from tag %s on port %s.", nsName, labelKey, portName)
			if err = tMgr.DeleteFromNLTag(labelKey, nsName, portName); err != nil {
				log.Errorf("Error: failed to delete namespace %s from tag %s on port %s.", nsName, labelKey, portName)
				return err
			}
		}

		// Delete the namespace from all-namespace tag.
		if err = tMgr.DeleteFromNLTag(util.KubeAllNamespacesFlag, nsName, portName); err != nil {
			log.Errorf("Error: failed to delete namespace %s from tag %s on port %s.", nsName, util.KubeAllNamespacesFlag, portName)
			return err
		}

		// Delete tag for the namespace.
		if err = tMgr.DeleteTag(nsName, portName); err != nil {
			log.Errorf("Error: failed to delete tag for namespace %s on port %s.", nsName, portName)
			return err
		}
	}

	delete(npMgr.nsMap, nsName)

	return nil
}
