package validate

import (
	"context"
	"log"
	"reflect"
	"strings"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const bashCommand = "bash"

func compareIPs(expected map[string]string, actual []string) error {
	expectedLen := len(expected)

	for _, ip := range actual {
		if _, ok := expected[ip]; !ok {
			return errors.Errorf("actual ip %s is unexpected, expected: %+v, actual: %+v", ip, expected, actual)
		}
		delete(expected, ip)
	}
	if expectedLen != len(actual) {
		return errors.Errorf("len of expected IPs != len of actual IPs, expected: %+v, actual: %+v | Remaining, potentially leaked, IP(s) on state file - %v", expectedLen, len(actual), expected)
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

// getCiliumInternalEndpointIPs execs into the cilium agent pod on the given node
// and runs `cilium endpoint list` to find IPs of reserved:ingress endpoints.
// These are not real Kubernetes pods but still have IPs allocated from CNS.
// Returns nil when Cilium is not installed or the exec fails.
func getCiliumInternalEndpointIPs(ctx context.Context, clientset *kubernetes.Clientset, config *rest.Config, nodeName string) []string {
	pods, err := acnk8s.GetPodsByNode(ctx, clientset, "kube-system", "k8s-app=cilium", nodeName)
	if err != nil || len(pods.Items) == 0 {
		return nil
	}

	cmd := []string{bashCommand, "-c", "cilium endpoint list | grep 'reserved:ingress' | grep -oE '[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+' || true"}
	result, _, err := acnk8s.ExecCmdOnPod(ctx, clientset, "kube-system", pods.Items[0].Name, "cilium-agent", cmd, config, true)
	if err != nil {
		return nil
	}

	return parseCiliumIngressIPs(result)
}

// parseCiliumIngressIPs parses the output of the grep pipeline that extracts
// reserved:ingress endpoint IPs from `cilium endpoint list`.
func parseCiliumIngressIPs(output []byte) []string {
	var ips []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if ip := strings.TrimSpace(line); ip != "" {
			ips = append(ips, ip)
		}
	}
	if len(ips) > 0 {
		log.Printf("Parsed Cilium internal endpoint IPs: %v", ips)
	} else {
		log.Printf("No Cilium internal endpoint IPs found in output")
	}
	return ips
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
