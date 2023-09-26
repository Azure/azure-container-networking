package validate

import (
	"context"
	"reflect"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func compareIPs(expected map[string]string, actual []string) error {
	if len(expected) != len(actual) {
		return errors.Errorf("len of expected IPs != len of actual IPs, expected: %+v, actual: %+v", expected, actual)
	}

	for _, ip := range actual {
		if _, ok := expected[ip]; !ok {
			return errors.Errorf("actual ip %s is unexpected, expected: %+v, actual: %+v", ip, expected, actual)
		}
	}

	return nil
}

// func to get the pods ip without the node ip (ie. host network as false)
func getPodIPsWithoutNodeIP(ctx context.Context, clientset *kubernetes.Clientset, node corev1.Node) []string {
	podsIpsWithoutNodeIP := []string{}
	podIPs, err := acnk8s.GetPodsIpsByNode(ctx, clientset, "", "", node.Name)
	if err != nil {
		return podsIpsWithoutNodeIP
	}
	nodeIPs := make([]string, 0)
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			nodeIPs = append(nodeIPs, address.Address)
		}
	}

	for _, podIP := range podIPs {
		if !contain(podIP, nodeIPs) {
			podsIpsWithoutNodeIP = append(podsIpsWithoutNodeIP, podIP)
		}
	}
	return podsIpsWithoutNodeIP
}

func contain(obj, target interface{}) bool {
	targetValue := reflect.ValueOf(target)
	switch reflect.TypeOf(target).Kind() { //nolint
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	}
	return false
}
