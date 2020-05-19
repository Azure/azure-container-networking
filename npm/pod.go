// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

func isValidPod(podObj *corev1.Pod) bool {
	return len(podObj.Status.PodIP) > 0
}

func isSystemPod(podObj *corev1.Pod) bool {
	return podObj.ObjectMeta.Namespace == util.KubeSystemFlag
}

func isInvalidPodUpdate(oldPodObj, newPodObj *corev1.Pod) (isInvalidUpdate bool) {
	isInvalidUpdate = oldPodObj.ObjectMeta.Namespace == newPodObj.ObjectMeta.Namespace &&
		oldPodObj.ObjectMeta.Name == newPodObj.ObjectMeta.Name &&
		oldPodObj.Status.Phase == newPodObj.Status.Phase &&
		oldPodObj.Status.PodIP == newPodObj.Status.PodIP &&
		newPodObj.ObjectMeta.DeletionTimestamp == nil &&
		newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil
	isInvalidUpdate = isInvalidUpdate && reflect.DeepEqual(oldPodObj.ObjectMeta.Labels, newPodObj.ObjectMeta.Labels)

	return
}

// AddPod handles adding pod ip to its label's ipset.
func (npMgr *NetworkPolicyManager) AddPod(podObj *corev1.Pod) error {
	if !isValidPod(podObj) {
		return nil
	}

	var (
		err           error
		podNs         = "ns-" + podObj.ObjectMeta.Namespace
		podName       = podObj.ObjectMeta.Name
		podNodeName   = podObj.Spec.NodeName
		podLabels     = podObj.ObjectMeta.Labels
		podIP         = podObj.Status.PodIP
		podContainers = podObj.Spec.Containers
		ipsMgr        = npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	)

	log.Printf("POD CREATING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Add pod namespace if it doesn't exist
	if _, exists := npMgr.nsMap[podNs]; !exists {
		log.Printf("Creating set: %v, hashedSet: %v", podNs, util.GetHashedName(podNs))
		if err = ipsMgr.CreateSet(podNs, util.IpsetNetHashFlag); err != nil {
			log.Printf("Error creating ipset %s", podNs)
		}
	}

	// Add the pod to its namespace's ipset.
	log.Printf("Adding pod %s to ipset %s", podIP, podNs)
	if err = ipsMgr.AddToSet(podNs, podIP, util.IpsetNetHashFlag); err != nil {
		log.Errorf("Error: failed to add pod to namespace ipset.")
	}

	// Add the pod to its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		log.Printf("Adding pod %s to ipset %s", podIP, podLabelKey)
		if err = ipsMgr.AddToSet(podLabelKey, podIP, util.IpsetNetHashFlag); err != nil {
			log.Errorf("Error: failed to add pod to label ipset.")
		}

		label := podLabelKey + ":" + podLabelVal
		log.Printf("Adding pod %s to ipset %s", podIP, label)
		if err = ipsMgr.AddToSet(label, podIP, util.IpsetNetHashFlag); err != nil {
			log.Errorf("Error: failed to add pod to label ipset.")
		}
	}

	// Add the pod's named ports its ipset.
	for _, container := range podContainers {
		for _, port := range container.Ports {
			if port.Name != "" {
				protocol := ""
				switch port.Protocol {
				case v1.ProtocolUDP:
					protocol = util.IpsetUDPFlag
				case v1.ProtocolSCTP:
					protocol = util.IpsetSCTPFlag
				}
				ipsMgr.AddToSet(port.Name, fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort), util.IpsetIPPortHashFlag)
			}
		}
	}

	npMgr.podMap[podNs+podName] = true

	return nil
}

// UpdatePod handles updating pod ip in its label's ipset.
func (npMgr *NetworkPolicyManager) UpdatePod(oldPodObj, newPodObj *corev1.Pod) error {
	if !isValidPod(newPodObj) {
		return nil
	}

	if isInvalidPodUpdate(oldPodObj, newPodObj) {
		return nil
	}

	var (
		err            error
		oldPodObjNs    = oldPodObj.ObjectMeta.Namespace
		oldPodObjName  = oldPodObj.ObjectMeta.Name
		oldPodObjLabel = oldPodObj.ObjectMeta.Labels
		oldPodObjPhase = oldPodObj.Status.Phase
		oldPodObjIP    = oldPodObj.Status.PodIP
		newPodObjNs    = newPodObj.ObjectMeta.Namespace
		newPodObjName  = newPodObj.ObjectMeta.Name
		newPodObjLabel = newPodObj.ObjectMeta.Labels
		newPodObjPhase = newPodObj.Status.Phase
		newPodObjIP    = newPodObj.Status.PodIP
	)

	log.Printf(
		"POD UPDATING:\n old pod: [%s/%s/%+v/%s/%s]\n new pod: [%s/%s/%+v/%s/%s]",
		oldPodObjNs, oldPodObjName, oldPodObjLabel, oldPodObjPhase, oldPodObjIP,
		newPodObjNs, newPodObjName, newPodObjLabel, newPodObjPhase, newPodObjIP,
	)

	if err = npMgr.DeletePod(oldPodObj); err != nil {
		log.Errorf("Error: failed to delete pod during update with error %+v", err)
		return err
	}

	// Assume that the pod IP will be released when pod moves to succeeded or failed state.
	// If the pod transitions back to an active state, then add operation will re establish the updated pod info.
	if newPodObj.ObjectMeta.DeletionTimestamp == nil && newPodObj.ObjectMeta.DeletionGracePeriodSeconds == nil && newPodObjPhase != v1.PodSucceeded && newPodObjPhase != v1.PodFailed {
		if err = npMgr.AddPod(newPodObj); err != nil {
			log.Errorf("Error: failed to add pod during update with error %+v", err)
		}
	}

	return nil
}

// DeletePod handles deleting pod from its label's ipset.
func (npMgr *NetworkPolicyManager) DeletePod(podObj *corev1.Pod) error {
	if !isValidPod(podObj) {
		return nil
	}

	var (
		err           error
		podNs         = "ns-" + podObj.ObjectMeta.Namespace
		podName       = podObj.ObjectMeta.Name
		podNodeName   = podObj.Spec.NodeName
		podLabels     = podObj.ObjectMeta.Labels
		podIP         = podObj.Status.PodIP
		podContainers = podObj.Spec.Containers
		ipsMgr        = npMgr.nsMap[util.KubeAllNamespacesFlag].ipsMgr
	)

	_, exists := npMgr.podMap[podNs+podName]
	if !exists {
		return nil
	}

	log.Printf("POD DELETING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	// Delete the pod from its namespace's ipset.
	if err = ipsMgr.DeleteFromSet(podNs, podIP); err != nil {
		log.Errorf("Error: failed to delete pod from namespace ipset.")
	}

	// Delete the pod from its label's ipset.
	for podLabelKey, podLabelVal := range podLabels {
		log.Printf("Deleting pod %s from ipset %s", podIP, podLabelKey)
		if err = ipsMgr.DeleteFromSet(podLabelKey, podIP); err != nil {
			log.Errorf("Error: failed to delete pod from label ipset.")
		}

		label := podLabelKey + ":" + podLabelVal
		log.Printf("Deleting pod %s from ipset %s", podIP, label)
		if err = ipsMgr.DeleteFromSet(label, podIP); err != nil {
			log.Errorf("Error: failed to delete pod from label ipset.")
		}
	}

	// Delete pod's named ports from its ipset.
	for _, container := range podContainers {
		for _, port := range container.Ports {
			if port.Name != "" {
				protocol := ""
				switch port.Protocol {
				case v1.ProtocolUDP:
					protocol = util.IpsetUDPFlag
				case v1.ProtocolSCTP:
					protocol = util.IpsetSCTPFlag
				}
				ipsMgr.DeleteFromSet(port.Name, fmt.Sprintf("%s,%s%d", podIP, protocol, port.ContainerPort))
			}
		}
	}

	delete(npMgr.podMap, podNs+podName)

	return nil
}
