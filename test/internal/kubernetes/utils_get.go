package kubernetes

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetNodeList(ctx context.Context, clientset *kubernetes.Clientset) (*corev1.NodeList, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes")
	}

	return nodes, nil
}

func GetNodeListByLabelSelector(ctx context.Context, clientset *kubernetes.Clientset, labelSelector string) (*corev1.NodeList, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get nodes with labelselector: %s", labelSelector)
	}

	return nodes, nil
}

func GetPodsByNode(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector, nodeName string) (*corev1.PodList, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
		LabelSelector: labelselector,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pods by node %s", nodeName)
	}
	return pods, nil
}

func GetPodsIpsByNode(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector, nodeName string) ([]string, error) {
	pods, err := GetPodsByNode(ctx, clientset, namespace, labelselector, nodeName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pods by node")
	}
	ips := make([]string, 0, len(pods.Items)*2) //nolint
	for index := range pods.Items {
		for _, podIP := range pods.Items[index].Status.PodIPs {
			ips = append(ips, podIP.IP)
		}
	}
	return ips, nil
}
