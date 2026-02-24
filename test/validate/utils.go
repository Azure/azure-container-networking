package validate

import (
	"context"
	"reflect"
	"sort"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type ipComparisonResult struct {
	ExpectedCount int      `json:"expectedCount"`
	ActualCount   int      `json:"actualCount"`
	MissingIPs    []string `json:"missingIPs,omitempty"`
	UnexpectedIPs []string `json:"unexpectedIPs,omitempty"`
	DuplicateIPs  []string `json:"duplicateIPs,omitempty"`
}

func (r ipComparisonResult) HasMismatch() bool {
	return len(r.MissingIPs) > 0 || len(r.UnexpectedIPs) > 0 || len(r.DuplicateIPs) > 0
}

func compareIPsDetailed(expected map[string]string, actual []string) ipComparisonResult {
	result := ipComparisonResult{
		ExpectedCount: len(expected),
		ActualCount:   len(actual),
	}

	seen := make(map[string]struct{}, len(actual))
	for _, ip := range actual {
		if _, found := seen[ip]; found {
			result.DuplicateIPs = append(result.DuplicateIPs, ip)
			continue
		}
		seen[ip] = struct{}{}

		if _, ok := expected[ip]; !ok {
			result.UnexpectedIPs = append(result.UnexpectedIPs, ip)
		}
	}

	for ip := range expected {
		if _, ok := seen[ip]; !ok {
			result.MissingIPs = append(result.MissingIPs, ip)
		}
	}

	sort.Strings(result.MissingIPs)
	sort.Strings(result.UnexpectedIPs)
	sort.Strings(result.DuplicateIPs)
	return result
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
