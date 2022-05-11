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

func (n *NpmPod) AppendLabels(newPod map[string]string, clear LabelAppendOperation) {
	if clear {
		n.Labels = make(map[string]string)
	}
	for k, v := range newPod {
		n.Labels[k] = v
	}
}

func (n *NpmPod) RemoveLabelsWithKey(key string) {
	delete(n.Labels, key)
}

func (n *NpmPod) AppendContainerPorts(podObj *corev1.Pod) {
	n.ContainerPorts = GetContainerPortList(podObj)
}

func (n *NpmPod) RemoveContainerPorts() {
	n.ContainerPorts = []corev1.ContainerPort{}
}

// This function can be expanded to other attribs if needed
func (n *NpmPod) UpdateNpmPodAttributes(podObj *corev1.Pod) {
	if n.Phase != podObj.Status.Phase {
		n.Phase = podObj.Status.Phase
	}
}

// noUpdate evaluates whether NpmPod is required to be update given podObj.
func (n *NpmPod) NoUpdate(podObj *corev1.Pod) bool {
	return n.Namespace == podObj.ObjectMeta.Namespace &&
		n.Name == podObj.ObjectMeta.Name &&
		n.Phase == podObj.Status.Phase &&
		n.PodIP == podObj.Status.PodIP &&
		k8slabels.Equals(n.Labels, podObj.ObjectMeta.Labels) &&
		// TODO(jungukcho) to avoid using DeepEqual for ContainerPorts,
		// it needs a precise sorting. Will optimize it later if needed.
		reflect.DeepEqual(n.ContainerPorts, GetContainerPortList(podObj))
}

func GetContainerPortList(podObj *corev1.Pod) []corev1.ContainerPort {
	portList := []corev1.ContainerPort{}
	for _, container := range podObj.Spec.Containers { //nolint:gocritic // intentionally copying full struct :(
		portList = append(portList, container.Ports...) //nolint:gocritic // intentionally copying full struct :(
	}
	return portList
}
