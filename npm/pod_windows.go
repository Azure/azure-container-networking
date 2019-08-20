// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"strings"

	"github.com/Microsoft/hcsshim"
	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/kalebmorris/azure-container-networking/npm/vfpm"
	corev1 "k8s.io/api/core/v1"
)

// getHNSEndpointByIP gets the endpoint corresponding to the given ip.
func getHNSEndpointByIP(endpointIP string) (*hcsshim.HNSEndpoint, error) {
	hnsResponse, err := hcsshim.HNSListEndpointRequest()
	if err != nil {
		return nil, err
	}
	for _, hnsEndpoint := range hnsResponse {
		if hnsEndpoint.IPAddress.String() == endpointIP {
			return &hnsEndpoint, nil
		}
	}
	return nil, hcsshim.EndpointNotFoundError{EndpointName: endpointIP}
}

// findPort retrieves the name of the VFP port associated with the given pod IP.
func findPort(podIP string) (string, error) {
	endpoint, err := getHNSEndpointByIP(podIP)
	if err != nil {
		log.Errorf("Error: failed to retrieve endpoint corresponding to pod ip %s.", podIP)
		return "", err
	}

	portName, err := vfpm.GetPortByMAC(endpoint.MacAddress)
	if err != nil {
		log.Errorf("Error: failed to find port for MAC %s.", endpoint.MacAddress)
		return "", err
	}

	return portName, nil
}

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
	podPort, err := findPort(podIP)
	if err != nil {
		return err
	}
	npMgr.ipPortMap[podIP] = podPort

	log.Printf("POD CREATING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	// Add the pod to tags.
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr

	for _, portName := range ports {
		// Add the pod to its namespace's tag.
		if err = tMgr.AddToTag(podNs, podIP, portName); err != nil {
			log.Errorf("Error: failed to add pod %s to tag %s on port %s.", podIP, podNs, portName)
			return err
		}

		// Add the pod to its labels' tags.
		for podLabelKey, podLabelVal := range podLabels {
			// Ignore pod-template-hash label.
			if strings.Contains(podLabelKey, util.KubePodTemplateHashFlag) {
				continue
			}

			labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
			if err = tMgr.AddToTag(labelKey, podIP, portName); err != nil {
				log.Errorf("Error: failed to add pod %s to tag %s on port %s.", podIP, labelKey, portName)
				return err
			}

			labelKey := podNs + "-" + podLabelKey + ":" + podLabelVal
			if err = tMgr.AddToTag(labelKey, podIP, portName); err != nil {
				log.Errorf("Error: failed to add pod %s to tag %s on port %s.", podIP, labelKey, portName)
				return err
			}
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
	delete(npMgr.ipPortMap, podIP)

	log.Printf("POD DELETING: [%s/%s/%s%+v%s]", podNs, podName, podNodeName, podLabels, podIP)

	ports, err := vfpm.GetPorts()
	if err != nil {
		log.Errorf("Error: failed to retrieve ports.")
		return err
	}

	// Delete pod from tags.
	tMgr := npMgr.nsMap[util.KubeAllNamespacesFlag].tMgr

	for _, portName := range ports {
		// Delete the pod from its namespace's tag.
		if err = tMgr.DeleteFromTag(podNs, podIP, portName); err != nil {
			log.Errorf("Error: failed to delete pod %s from tag %s on port %s.", podIP, podNs, portName)
			return err
		}
		// Delete the pod from its labels' tags.
		for podLabelKey, podLabelVal := range podLabels {
			//Ignore pod-template-hash label.
			if strings.Contains(podLabelKey, "pod-template-hash") {
				continue
			}

			labelKey := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
			if err = tMgr.DeleteFromTag(labelKey, podIP, portName); err != nil {
				log.Errorf("Error: failed to delete pod %s from tag %s on port %s.", podIP, labelKey, portName)
				return err
			}

			labelKey := podNs + "-" + podLabelKey + ":" + podLabelVal
			if err = tMgr.DeleteFromTag(labelKey, podIP, portName); err != nil {
				log.Errorf("Error: failed to delete pod %s from tag %s on port %s.", podIP, labelKey, portName)
				return err
			}
		}
	}

	return nil
}
