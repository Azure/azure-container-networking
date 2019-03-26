// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
)

func isValidPod(podObj *corev1.Pod) bool {
	return podObj.Status.Phase != "Failed" &&
		podObj.Status.Phase != "Succeeded" &&
		podObj.Status.Phase != "Unknown" &&
		len(podObj.Status.PodIP) > 0
}

func isSystemPod(podObj *corev1.Pod) bool {
	return podObj.ObjectMeta.Namespace == util.KubeSystemFlag
}

// AddPod handles adding pod ip to its label's ipset.
func (npMgr *NetworkPolicyManager) AddPod(podObj *corev1.Pod) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	if !isValidPod(podObj) {
		return nil
	}

	var err error

	defer func() {
		if err = npMgr.UpdateAndSendReport(err, util.AddPodEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
	}()

	podNs := podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP
	log.Printf("POD CREATING: %s/%s/%s%+v%s\n", podNs, podName, podNodeName, podLabels, podIP)

	// Add the pod to ipset
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	// Add the pod to its namespace's ipset.
	log.Printf("Adding pod %s to ipset %s\n", podIP, podNs)
	if err = ipsMgr.AddToSet(podNs, podIP); err != nil {
		log.Printf("Error adding pod to namespace ipset.\n")
		return err
	}

	// Add the pod to its label's ipset.
	var labelKeys []string
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, util.KubePodTemplateHashFlag) {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		log.Printf("Adding pod %s to ipset %s\n", podIP, labelKey)
		if err = ipsMgr.AddToSet(labelKey, podIP); err != nil {
			log.Printf("Error adding pod to label ipset.\n")
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	ns, err := newNs(podNs)
	if err != nil {
		log.Printf("Error creating namespace %s\n", podNs)
		return err
	}
	npMgr.nsMap[podNs] = ns

	return nil
}

// UpdatePod handles updating pod ip in its label's ipset.
func (npMgr *NetworkPolicyManager) UpdatePod(oldPodObj, newPodObj *corev1.Pod) error {
	if !isValidPod(newPodObj) {
		return nil
	}

	var err error

	defer func() {
		npMgr.Lock()
		if err = npMgr.UpdateAndSendReport(err, util.UpdateNamespaceEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
		npMgr.Unlock()
	}()

	oldPodObjNs := oldPodObj.ObjectMeta.Namespace
	oldPodObjName := oldPodObj.ObjectMeta.Name
	oldPodObjPhase := oldPodObj.Status.Phase
	oldPodObjIP := oldPodObj.Status.PodIP
	newPodObjNs := newPodObj.ObjectMeta.Namespace
	newPodObjName := newPodObj.ObjectMeta.Name
	newPodObjPhase := newPodObj.Status.Phase
	newPodObjIP := newPodObj.Status.PodIP

	log.Printf(
		"POD UPDATING. %s %s %s %s %s %s %s %s\n",
		oldPodObjNs, oldPodObjName, oldPodObjPhase, oldPodObjIP, newPodObjNs, newPodObjName, newPodObjPhase, newPodObjIP,
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
	npMgr.Lock()
	defer npMgr.Unlock()

	if !isValidPod(podObj) {
		return nil
	}

	var err error

	defer func() {
		if err = npMgr.UpdateAndSendReport(err, util.DeletePodEvent); err != nil {
			log.Printf("Error sending NPM telemetry report")
		}
	}()

	podNs := podObj.ObjectMeta.Namespace
	podName := podObj.ObjectMeta.Name
	podNodeName := podObj.Spec.NodeName
	podLabels := podObj.ObjectMeta.Labels
	podIP := podObj.Status.PodIP
	log.Printf("POD DELETING: %s/%s/%s\n", podNs, podName, podNodeName)

	// Delete pod from ipset
	ipsMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	// Delete the pod from its namespace's ipset.
	if err = ipsMgr.DeleteFromSet(podNs, podIP); err != nil {
		log.Printf("Error deleting pod from namespace ipset.\n")
		return err
	}
	// Delete the pod from its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, "pod-template-hash") {
			continue
		}

		labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		if err = ipsMgr.DeleteFromSet(labelKey, podIP); err != nil {
			log.Printf("Error deleting pod from label ipset.\n")
			return err
		}
	}

	return nil
}
