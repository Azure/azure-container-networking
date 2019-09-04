// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"strings"

	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
)

// AddPod handles adding pod ip to its labels' tags.
func (npMgr *NetworkPolicyManager) AddPod(podObj *corev1.Pod) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	if !isValidPod(podObj) {
		return nil
	}

	var err error

	podNs := podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
	rMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].rMgr

	if podNodeName == npMgr.nodeName {
		err = tMgr.ApplyTags(podObj)
		if err != nil {
			log.Errorf("Error: failed to apply existing tags to new pod %s.", podName)
			return err
		}
	}

	log.Printf("POD CREATING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Add the pod to its namespace's tag.
	if err = tMgr.AddToTag(podNs, podIP); err != nil {
		log.Errorf("Error: failed to add pod %s to tag %s.", podIP, podNs)
		return err
	}

	// Add the pod to its labels' tags.
	for podLabelKey, podLabelVal := range podLabels {
		// Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, util.KubePodTemplateHashFlag) {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		if err = tMgr.AddToTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to add pod %s to tag %s.", podIP, labelKey)
			return err
		}

		labelKey = podNs + "-" + podLabelKey + ":" + podLabelVal
		if err = tMgr.AddToTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to add pod %s to tag %s.", podIP, labelKey)
			return err
		}
	}

	// Apply existing rules to pod.
	if podNodeName == npMgr.nodeName {
		err = rMgr.ApplyRules(podObj, tMgr)
		if err != nil {
			log.Errorf("Error: failed to apply existing vfp rules to new pod %s.", podName)
		}
	}

	ns, err := newNs(podNs)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", podNs)
		return err
	}
	npMgr.nsMap[podNs] = ns

	return nil
}

// DeletePod handles deleting pod from its labels' tags.
func (npMgr *NetworkPolicyManager) DeletePod(podObj *corev1.Pod) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	if !isValidPod(podObj) {
		return nil
	}

	var err error

	podNs := podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP

	log.Printf("POD DELETING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Delete pod from tags.
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr

	// Delete the pod from its namespace's tag.
	if err = tMgr.DeleteFromTag(podNs, podIP); err != nil {
		log.Errorf("Error: failed to delete pod %s from tag %s.", podIP, podNs)
		return err
	}
	// Delete the pod from its labels' tags.
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, "pod-template-hash") {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		if err = tMgr.DeleteFromTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to delete pod %s from tag %s.", podIP, labelKey)
			return err
		}

		labelKey = podNs + "-" + podLabelKey + ":" + podLabelVal
		if err = tMgr.DeleteFromTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to delete pod %s from tag %s.", podIP, labelKey)
			return err
		}
	}

	// Update rMgr state.
	npMgr.nsMap[util.KubeAllNamespacesFlag].rMgr.HandlePodDeletion(podObj)

	return nil
}
