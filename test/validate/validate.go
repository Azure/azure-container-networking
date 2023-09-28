package validate

import (
	"context"
	"log"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var privilegedDaemonSetPathMap = map[string]string{
	"windows": "../manifests/load/privileged-daemonset-windows.yaml",
	"linux":   "../manifests/load/privileged-daemonset.yaml",
}

var nodeSelectorMap = map[string]string{
	"windows": "kubernetes.io/os=windows",
	"linux":   "kubernetes.io/os=linux",
}

// IPv4 overlay Linux and windows nodes must have this label
var v4OverlayNodeLabels = map[string]string{
	"kubernetes.azure.com/podnetwork-type": "overlay",
}

// dualstack overlay Linux and windows nodes must have these labels
var dualstackOverlayNodeLabels = map[string]string{
	"kubernetes.azure.com/podnetwork-type":   "overlay",
	"kubernetes.azure.com/podv6network-type": "overlay",
}

const (
	privilegedLabelSelector  = "app=privileged-daemonset"
	privilegedNamespace      = "kube-system"
	IPv4ExpectedIPCount      = 1
	DualstackExpectedIPCount = 2
)

type Validator struct {
	clientset   *kubernetes.Clientset
	config      *rest.Config
	checks      []check
	namespace   string
	cni         string
	restartCase bool
	os          string
}

type check struct {
	name             string
	stateFileIps     func([]byte) (map[string]string, error)
	podLabelSelector string
	podNamespace     string
	cmd              []string
}

func CreateValidator(ctx context.Context, clientset *kubernetes.Clientset, config *rest.Config, namespace, cni string, restartCase bool, os string) (*Validator, error) {
	// deploy privileged pod
	privilegedDaemonSet, err := acnk8s.MustParseDaemonSet(privilegedDaemonSetPathMap[os])
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse daemonset")
	}
	daemonsetClient := clientset.AppsV1().DaemonSets(privilegedNamespace)
	if err := acnk8s.MustCreateDaemonset(ctx, daemonsetClient, privilegedDaemonSet); err != nil {
		return nil, errors.Wrap(err, "unable to create daemonset")
	}
	// Ensures that pods have been replaced if test is re-run after failure
	if err := acnk8s.WaitForPodDaemonset(ctx, clientset, privilegedNamespace, privilegedDaemonSet.Name, privilegedLabelSelector); err != nil {
		return nil, errors.Wrap(err, "unable to wait for daemonset")
	}

	var checks []check
	switch os {
	case "windows":
		checks = windowsChecksMap[cni]
	case "linux":
		checks = linuxChecksMap[cni]
	default:
		return nil, errors.Errorf("unsupported os: %s", os)
	}

	return &Validator{
		clientset:   clientset,
		config:      config,
		namespace:   namespace,
		cni:         cni,
		restartCase: restartCase,
		checks:      checks,
		os:          os,
	}, nil
}

func (v *Validator) Validate(ctx context.Context) error {
	log.Printf("Validating State File")
	err := v.ValidateStateFile(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to validate state file")
	}

	if v.os == "linux" {
		// We are restarting the systmemd network and checking that the connectivity works after the restart. For more details: https://github.com/cilium/cilium/issues/18706
		log.Printf("Validating the restart network scenario")
		err = v.ValidateRestartNetwork(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate restart network scenario")
		}
	}
	return nil
}

func (v *Validator) ValidateStateFile(ctx context.Context) error {
	for _, check := range v.checks {
		err := v.validateIPs(ctx, check.stateFileIps, check.cmd, check.name, check.podNamespace, check.podLabelSelector)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) ValidateRestartNetwork(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := acnk8s.GetPodsByNode(ctx, v.clientset, privilegedNamespace, privilegedLabelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}

		privelegedPod := pod.Items[0]
		// exec into the pod to get the state file
		_, err = acnk8s.ExecCmdOnPod(ctx, v.clientset, privilegedNamespace, privelegedPod.Name, restartNetworkCmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod - %s", privelegedPod.Name)
		}
		err = acnk8s.WaitForPodsRunning(ctx, v.clientset, "", "")
		if err != nil {
			return errors.Wrapf(err, "failed to wait for pods running")
		}
	}
	return nil
}

func (v *Validator) validateIPs(ctx context.Context, stateFileIps stateFileIpsFunc, cmd []string, checkType, namespace, labelSelector string) error {
	log.Printf("Validating %s state file", checkType)
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := acnk8s.GetPodsByNode(ctx, v.clientset, namespace, labelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}
		podName := pod.Items[0].Name
		// exec into the pod to get the state file
		result, err := acnk8s.ExecCmdOnPod(ctx, v.clientset, namespace, podName, cmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod - %s", podName)
		}
		filePodIps, err := stateFileIps(result)
		if err != nil {
			return errors.Wrapf(err, "failed to get pod ips from state file")
		}
		if len(filePodIps) == 0 && v.restartCase {
			log.Printf("No pods found on node %s", nodes.Items[index].Name)
			continue
		}
		// get the pod ips
		podIps := getPodIPsWithoutNodeIP(ctx, v.clientset, nodes.Items[index])

		if err := compareIPs(filePodIps, podIps); err != nil {
			return errors.Wrapf(errors.New("State file validation failed"), "for %s on node %s", checkType, nodes.Items[index].Name)
		}
	}
	log.Printf("State file validation for %s passed", checkType)
	return nil
}

func validateNodeProperties(nodes *corev1.NodeList, labels map[string]string, expectedIPCount int) error {
	log.Print("Validating Node properties")

	for index := range nodes.Items {
		nodeName := nodes.Items[index].ObjectMeta.Name
		// check nodes status;
		// nodes status should be ready after cluster is created
		nodeConditions := nodes.Items[index].Status.Conditions
		if nodeConditions[len(nodeConditions)-1].Type != corev1.NodeReady {
			return errors.Errorf("node %s status is not ready", nodeName)
		}

		// get node labels
		nodeLabels := nodes.Items[index].ObjectMeta.GetLabels()
		for key := range nodeLabels {
			if label, ok := labels[key]; ok {
				log.Printf("label %s is correctly shown on the node %+v", key, nodeName)
				if label != overlayClusterLabelName {
					return errors.Errorf("node %s overlay label name is wrong; expected label:%s but actual label:%s", nodeName, overlayClusterLabelName, label)
				}
			}
		}

		// check if node has correct number of internal IPs
		internalIPCount := 0
		for _, address := range nodes.Items[index].Status.Addresses {
			if address.Type == "InternalIP" {
				internalIPCount++
			}
		}
		if internalIPCount != expectedIPCount {
			return errors.Errorf("number of node internal IPs: %d does not match expected number of IPs %d", internalIPCount, expectedIPCount)
		}
	}
	return nil
}

func (v *Validator) ValidateV4OverlayControlPlane(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrap(err, "failed to get node list")
	}

	if err := validateNodeProperties(nodes, v4OverlayNodeLabels, IPv4ExpectedIPCount); err != nil {
		return errors.Wrap(err, "failed to validate IPv4 overlay node properties")
	}

	if v.os == "windows" {
		if err := validateHNSNetworkState(ctx, nodes, v.clientset, v.config); err != nil {
			return errors.Wrap(err, "failed to validate IPv4 overlay HNS network state")
		}
	}

	return nil
}

func (v *Validator) ValidateDualStackControlPlane(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrap(err, "failed to get node list")
	}

	if err := validateNodeProperties(nodes, dualstackOverlayNodeLabels, DualstackExpectedIPCount); err != nil {
		return errors.Wrap(err, "failed to validate dualstack overlay node properties")
	}

	if v.os == "windows" {
		if err := validateHNSNetworkState(ctx, nodes, v.clientset, v.config); err != nil {
			return errors.Wrap(err, "failed to validate dualstack overlay HNS network state")
		}
	}

	return nil
}

func (v *Validator) RestartKubeProxyService(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		node := nodes.Items[index]
		if node.Status.NodeInfo.OperatingSystem != string(corev1.Windows) {
			continue
		}
		// get the privileged pod
		pod, err := acnk8s.GetPodsByNode(ctx, v.clientset, privilegedNamespace, privilegedLabelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}

		privelegedPod := pod.Items[0]
		// exec into the pod and restart kubeproxy
		_, err = acnk8s.ExecCmdOnPod(ctx, v.clientset, privilegedNamespace, privelegedPod.Name, restartKubeProxyCmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod - %s", privelegedPod.Name)
		}
	}
	return nil
}

func (v *Validator) Cleanup(ctx context.Context) error {
	// deploy privileged pod
	privilegedDaemonSet, err := acnk8s.MustParseDaemonSet(privilegedDaemonSetPathMap[v.os])
	if err != nil {
		return errors.Wrap(err, "unable to parse daemonset")
	}
	daemonsetClient := v.clientset.AppsV1().DaemonSets(privilegedNamespace)
	if err := acnk8s.MustDeleteDaemonset(ctx, daemonsetClient, privilegedDaemonSet); err != nil {
		return errors.Wrap(err, "unable to delete daemonset")
	}
	return nil
}
