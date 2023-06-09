package validate

import (
	"context"

	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func compareIPs(expected map[string]string, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}

	for _, ip := range actual {
		if _, ok := expected[ip]; !ok {
			return false
		}
	}

	return true
}

// func to get the pods ip without the node ip (ie. host network as false)
func getPodIPsWithoutNodeIP(ctx context.Context, clientset *kubernetes.Clientset, node corev1.Node) []string {
	podsIpsWithoutNodeIP := []string{}
	podIPs, err := k8sutils.GetPodsIpsByNode(ctx, clientset, "", "", node.Name)
	if err != nil {
		return podsIpsWithoutNodeIP
	}
	nodeIP := node.Status.Addresses[0].Address
	for _, podIP := range podIPs {
		if podIP != nodeIP {
			podsIpsWithoutNodeIP = append(podsIpsWithoutNodeIP, podIP)
		}
	}
	return podsIpsWithoutNodeIP
}
