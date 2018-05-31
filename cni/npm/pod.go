package npm

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/cni/npm/util"

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

// AddPod handles add pod.
func (npMgr *NetworkPolicyManager) AddPod(podObj *corev1.Pod) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	// Don't deal with system pods.
	if isSystemPod(podObj) {
		return nil
	}

	if !isValidPod(podObj) {
		return nil
	}

	podNs, podName, podNodeName, podLabels, podIP := podObj.ObjectMeta.Namespace, podObj.ObjectMeta.Name, podObj.Spec.NodeName, podObj.ObjectMeta.Labels, podObj.Status.PodIP
	fmt.Printf("POD CREATED: %s/%s/%s%+v%s\n", podNs, podName, podNodeName, podLabels, podIP)

	ns, exists := npMgr.nsMap[podNs]
	if !exists {
		newns, err := newNs(podNs)
		if err != nil {
			return err
		}
		npMgr.nsMap[podNs] = newns
		ns = newns
	}
	ns.podMap[podObj.ObjectMeta.UID] = podObj

	// Add the pod to ipset
	ipsMgr := ns.ipsMgr
	// Add the pod to its namespace's ipset.
	fmt.Printf("Adding pod %s to ipset %s\n", podIP, podNs)
	if err := ipsMgr.AddToSet(podNs, podIP); err != nil {
		fmt.Printf("Error adding pod to namespace ipset.\n")
	}

	// Add the pod to its label's ipset.
	var labelKeys []string
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, util.KubePodTemplateHashFlag) {
			continue
		}

		labelKey := podNs + "-" + podLabelKey + ":" + podLabelVal
		fmt.Printf("Adding pod %s to ipset %s\n", podIP, labelKey)
		if err := ipsMgr.AddToSet(labelKey, podIP); err != nil {
			fmt.Printf("Error adding pod to label ipset.\n")
			return err
		}
		labelKeys = append(labelKeys, labelKey)
	}

	return nil
}

// UpdatePod handles update pod.
func (npMgr *NetworkPolicyManager) UpdatePod(oldPodObj, newPodObj *corev1.Pod) error {
	npMgr.Lock()

	// Ignore system pods.
	if isSystemPod(newPodObj) {
		npMgr.Unlock()
		return nil
	}

	if !isValidPod(newPodObj) {
		npMgr.Unlock()
		return nil
	}

	oldPodObjNs, oldPodObjName, oldPodObjPhase, oldPodObjIP, newPodObjNs, newPodObjName, newPodObjPhase, newPodObjIP := oldPodObj.ObjectMeta.Namespace, oldPodObj.ObjectMeta.Name, oldPodObj.Status.Phase, oldPodObj.Status.PodIP, newPodObj.ObjectMeta.Namespace, newPodObj.ObjectMeta.Name, newPodObj.Status.Phase, newPodObj.Status.PodIP

	fmt.Printf(
		"POD UPDATED. %s %s %s %s %s %s %s %s\n",
		oldPodObjNs, oldPodObjName, oldPodObjPhase, oldPodObjIP, newPodObjNs, newPodObjName, newPodObjPhase, newPodObjIP,
	)

	npMgr.Unlock()
	npMgr.DeletePod(oldPodObj)

	if newPodObj.ObjectMeta.DeletionTimestamp == nil && newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil {
		npMgr.AddPod(newPodObj)
	}

	return nil
}

// DeletePod handles delete pod.
func (npMgr *NetworkPolicyManager) DeletePod(podObj *corev1.Pod) error {
	npMgr.Lock()
	defer npMgr.Unlock()

	// Don't deal with system pods.
	if isSystemPod(podObj) {
		return nil
	}

	if !isValidPod(podObj) {
		return nil
	}

	podNs, podName, podNodeName, podLabels := podObj.ObjectMeta.Namespace, podObj.ObjectMeta.Name, podObj.Spec.NodeName, podObj.ObjectMeta.Labels
	fmt.Printf("POD DELETED: %s/%s/%s\n", podNs, podName, podNodeName)

	ns, exists := npMgr.nsMap[podNs]
	if !exists {
		newns, err := newNs(podNs)
		if err != nil {
			return err
		}
		npMgr.nsMap[podNs] = newns
		ns = newns
	}
	delete(ns.podMap, podObj.ObjectMeta.UID)

	// Delete pod from ipset
	podIP := podObj.Status.PodIP
	ipsMgr := ns.ipsMgr
	// Delete the pod from its namespace's ipset.
	if err := ipsMgr.DeleteFromSet(podNs, podIP); err != nil {
		fmt.Printf("Error deleting pod from namespace ipset.\n")
		return err
	}
	// Delete the pod from its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		//Ignore pod-template-hash label.
		if strings.Contains(podLabelKey, "pod-template-hash") {
			continue
		}

		labelKey := podNs + "-" + podLabelKey + ":" + podLabelVal
		if err := ipsMgr.DeleteFromSet(labelKey, podIP); err != nil {
			fmt.Printf("Error deleting pod from label ipset.\n")
			return err
		}
	}

	if len(ns.npMap) == 0 {
		if err := ipsMgr.Clean(); err != nil {
			fmt.Printf("Error cleaning ipset\n")
			return err
		}
	}

	return nil
}
