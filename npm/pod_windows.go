// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"strings"

	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
)

// AddPod handles adding pod ip to its label's tag.
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
	log.Printf("POD CREATING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Add the pod to tag
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
	// Add the pod to its namespace's tag.
	log.Printf("Adding pod %s to tag %s", podIP, podNs)
	if err = tMgr.AddToTag(podNs, podIP); err != nil {
		log.Errorf("Error: failed to add pod to namespace tag.")
		return err
	}

	// Add the pod to its label's tag.
	var labelKeys []string
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, util.KubePodTemplateHashFlag) {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		log.Printf("Adding pod %s to tag %s", podIP, labelKey)
		if err = tMgr.AddToTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to add pod to label tag.")
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	ns, err := newNs(podNs)
	if err != nil {
		log.Errorf("Error: failed to create namespace %s", podNs)
		return err
	}
	npMgr.nsMap[podNs] = ns

	return nil
}

// DeletePod handles deleting pod from its label's tag.
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

	// Delete pod from tag
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr
	// Delete the pod from its namespace's tag.
	if err = tMgr.DeleteFromTag(podNs, podIP); err != nil {
		log.Errorf("Error: failed to delete pod from namespace tag.")
		return err
	}
	// Delete the pod from its label's tag.
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, "pod-template-hash") {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		if err = tMgr.DeleteFromTag(labelKey, podIP); err != nil {
			log.Errorf("Error: failed to delete pod from label tag.")
			return err
		}
	}

	return nil
}
