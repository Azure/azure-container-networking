// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"

	corev1 "k8s.io/api/core/v1"
)

func isValidPod(podObj *corev1.Pod) bool {
	return podObj.Status.Phase != corev1.PodPhase(util.KubePodStatusFailedFlag) &&
		podObj.Status.Phase != corev1.PodPhase(util.KubePodStatusSucceededFlag) &&
		podObj.Status.Phase != corev1.PodPhase(util.KubePodStatusUnknownFlag) &&
		len(podObj.Status.PodIP) > 0
}

func isSystemPod(podObj *corev1.Pod) bool {
	return podObj.ObjectMeta.Namespace == util.KubeSystemFlag
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
