package common

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
)

type NpmPod struct {
	Name           string
	Namespace      string
	PodIP          string
	Labels         map[string]string
	ContainerPorts []corev1.ContainerPort
	Phase          corev1.PodPhase
}

type LabelAppendOperation bool

const (
	ClearExistingLabels    LabelAppendOperation = true
	AppendToExistingLabels LabelAppendOperation = false
)

func (n *NpmPod) IP() string {
	return n.PodIP
}

func (n *NpmPod) NamespaceString() string {
	return n.Namespace
}

func NewNpmPod(podObj *corev1.Pod) *NpmPod {
	return &NpmPod{
		Name:           podObj.ObjectMeta.Name,
		Namespace:      podObj.ObjectMeta.Namespace,
		PodIP:          podObj.Status.PodIP,
		Labels:         make(map[string]string),
		ContainerPorts: []corev1.ContainerPort{},
		Phase:          podObj.Status.Phase,
	}
}

func (nPod *NpmPod) AppendLabels(new map[string]string, clear LabelAppendOperation) {
	if clear {
		nPod.Labels = make(map[string]string)
	}
	for k, v := range new {
		nPod.Labels[k] = v
	}
}

func (nPod *NpmPod) RemoveLabelsWithKey(key string) {
	delete(nPod.Labels, key)
}

func (nPod *NpmPod) AppendContainerPorts(podObj *corev1.Pod) {
	nPod.ContainerPorts = GetContainerPortList(podObj)
}

func (nPod *NpmPod) RemoveContainerPorts() {
	nPod.ContainerPorts = []corev1.ContainerPort{}
}

// This function can be expanded to other attribs if needed
func (nPod *NpmPod) UpdateNpmPodAttributes(podObj *corev1.Pod) {
	if nPod.Phase != podObj.Status.Phase {
		nPod.Phase = podObj.Status.Phase
	}
}

// noUpdate evaluates whether NpmPod is required to be update given podObj.
func (nPod *NpmPod) NoUpdate(podObj *corev1.Pod) bool {
	return nPod.Namespace == podObj.ObjectMeta.Namespace &&
		nPod.Name == podObj.ObjectMeta.Name &&
		nPod.Phase == podObj.Status.Phase &&
		nPod.PodIP == podObj.Status.PodIP &&
		k8slabels.Equals(nPod.Labels, podObj.ObjectMeta.Labels) &&
		// TODO(jungukcho) to avoid using DeepEqual for ContainerPorts,
		// it needs a precise sorting. Will optimize it later if needed.
		reflect.DeepEqual(nPod.ContainerPorts, GetContainerPortList(podObj))
}

func GetContainerPortList(podObj *corev1.Pod) []corev1.ContainerPort {
	portList := []corev1.ContainerPort{}
	for _, container := range podObj.Spec.Containers {
		portList = append(portList, container.Ports...)
	}
	return portList
}
