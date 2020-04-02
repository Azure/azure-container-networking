// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
)

func isValidPod(podObj *corev1.Pod) bool {
	return len(podObj.Status.PodIP) > 0
}

func isSystemPod(podObj *corev1.Pod) bool {
	return podObj.ObjectMeta.Namespace == util.KubeSystemFlag
}

// AddPod handles adding pod ip to its label's ipset.
func (npMgr *NetworkPolicyManager) AddPod(podObj *corev1.Pod) error {
	if !isValidPod(podObj) {
		return nil
	}

	var err error

	podNs := "ns-" + podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP
	log.Printf("POD CREATING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Add the pod to ipset
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr

	// Add pod namespace if it doesn't exist
	if _, exists := npMgr.nsMap[podNs]; !exists {
		log.Printf("Creating set: %v, hashedSet: %v", podNs, util.GetHashedName(podNs))
		if err = ipsMgr.CreateSet(podNs); err != nil {
			log.Printf("Error creating ipset %s", podNs)
			return err
		}
	}

	// Add the pod to its namespace's ipset.
	log.Printf("Adding pod %s to ipset %s", podIP, podNs)
	if err = ipsMgr.AddToSet(podNs, podIP); err != nil {
		log.Errorf("Error: failed to add pod to namespace ipset.")
		return err
	}

	// Add the pod to its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		log.Printf("Adding pod %s to ipset %s", podIP, podLabelKey)
		if err = ipsMgr.AddToSet(podLabelKey, podIP); err != nil {
			log.Errorf("Error: failed to add pod to label ipset.")
			return err
		}

		label := podLabelKey + ":" + podLabelVal
		log.Printf("Adding pod %s to ipset %s", podIP, label)
		if err = ipsMgr.AddToSet(label, podIP); err != nil {
			log.Errorf("Error: failed to add pod to label ipset.")
			return err
		}
	}

	return nil
}

// UpdatePod handles updating pod ip in its label's ipset.
func (npMgr *NetworkPolicyManager) UpdatePod(oldPodObj, newPodObj *corev1.Pod) error {
	if !isValidPod(newPodObj) {
		return nil
	}

	var err error

	oldPodObjNs := oldPodObj.ObjectMeta.Namespace
	oldPodObjName := oldPodObj.ObjectMeta.Name
	oldPodObjLabel := oldPodObj.ObjectMeta.Labels
	oldPodObjPhase := oldPodObj.Status.Phase
	oldPodObjIP := oldPodObj.Status.PodIP
	newPodObjNs := newPodObj.ObjectMeta.Namespace
	newPodObjName := newPodObj.ObjectMeta.Name
	newPodObjLabel := newPodObj.ObjectMeta.Labels
	newPodObjPhase := newPodObj.Status.Phase
	newPodObjIP := newPodObj.Status.PodIP

	log.Printf(
		"POD UPDATING:\n old pod: [%s/%s/%+v/%s/%s]\n new pod: [%s/%s/%+v/%s/%s]",
		oldPodObjNs, oldPodObjName, oldPodObjLabel, oldPodObjPhase, oldPodObjIP,
		newPodObjNs, newPodObjName, newPodObjLabel, newPodObjPhase, newPodObjIP,
	)

	if err = npMgr.DeletePod(oldPodObj); err != nil {
		return err
	}

	if newPodObj.ObjectMeta.DeletionTimestamp == nil && newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		if err = npMgr.AddPod(newPodObj); err != nil {
			return err
		}
	}

	return nil
}

// DeletePod handles deleting pod from its label's ipset.
func (npMgr *NetworkPolicyManager) DeletePod(podObj *corev1.Pod) error {
	if !isValidPod(podObj) {
		return nil
	}

	var err error

	podNs := "ns-" + podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP
	log.Printf("POD DELETING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Delete pod from ipset
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	// Delete the pod from its namespace's ipset.
	if err = ipsMgr.DeleteFromSet(podNs, podIP); err != nil {
		log.Errorf("Error: failed to delete pod from namespace ipset.")
		return err
	}
	// Delete the pod from its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		log.Printf("Deleting pod %s from ipset %s", podIP, podLabelKey)
		if err = ipsMgr.DeleteFromSet(podLabelKey, podIP); err != nil {
			log.Errorf("Error: failed to delete pod from label ipset.")
			return err
		}

		label := podLabelKey + ":" + podLabelVal
		log.Printf("Deleting pod %s from ipset %s", podIP, label)
		if err = ipsMgr.DeleteFromSet(label, podIP); err != nil {
			log.Errorf("Error: failed to delete pod from label ipset.")
			return err
		}
	}

	return nil
}
