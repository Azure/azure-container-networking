package validate

import (
	"context"
	"encoding/json"
	"log"

	"github.com/Azure/azure-container-networking/cns"
	restserver "github.com/Azure/azure-container-networking/cns/restserver"
	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	privilegedDaemonSetPath = "../manifests/load/privileged-daemonset.yaml"
	privilegedLabelSelector = "app=privileged-daemonset"
	privilegedNamespace     = "kube-system"

	cnsLabelSelector    = "k8s-app=azure-cns"
	ciliumLabelSelector = "k8s-app=cilium"
)

var (
	restartNetworkCmd  = []string{"bash", "-c", "chroot /host /bin/bash -c 'systemctl restart systemd-networkd'"}
	cnsStateFileCmd    = []string{"bash", "-c", "cat /var/run/azure-cns/azure-endpoints.json"}
	ciliumStateFileCmd = []string{"bash", "-c", "cilium endpoint list -o json"}
	cnsLocalCacheCmd   = []string{"curl", "localhost:10090/debug/ipaddresses", "-d", "{\"IPConfigStateFilter\":[\"Assigned\"]}"}
)

type stateFileIpsFunc func([]byte) (map[string]string, error)

type LinuxValidator struct {
	clientset   *kubernetes.Clientset
	config      *rest.Config
	namespace   string
	cni         string
	restartCase bool
}

type CnsState struct {
	Endpoints map[string]restserver.EndpointInfo `json:"Endpoints"`
}

type CNSLocalCache struct {
	IPConfigurationStatus []cns.IPConfigurationStatus `json:"IPConfigurationStatus"`
}

type CiliumEndpointStatus struct {
	Status NetworkingStatus `json:"status"`
}

type NetworkingStatus struct {
	Networking NetworkingAddressing `json:"networking"`
}

type NetworkingAddressing struct {
	Addresses     []Address `json:"addressing"`
	InterfaceName string    `json:"interface-name"`
}

type Address struct {
	Addr string `json:"ipv4"`
}

func CreateLinuxValidator(ctx context.Context, clienset *kubernetes.Clientset, config *rest.Config, namespace, cni string, restartCase bool) (*LinuxValidator, error) {
	// deploy privileged pod
	privilegedDaemonSet, err := k8sutils.MustParseDaemonSet(privilegedDaemonSetPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse daemonset")
	}
	daemonsetClient := clienset.AppsV1().DaemonSets(privilegedNamespace)
	if err := k8sutils.MustCreateDaemonset(ctx, daemonsetClient, privilegedDaemonSet); err != nil {
		return nil, errors.Wrap(err, "unable to create daemonset")
	}
	if err := k8sutils.WaitForPodsRunning(ctx, clienset, privilegedNamespace, privilegedLabelSelector); err != nil {
		return nil, errors.Wrap(err, "error while waiting for pods to be running")
	}

	return &LinuxValidator{
		clientset:   clienset,
		config:      config,
		namespace:   namespace,
		cni:         cni,
		restartCase: restartCase,
	}, nil
}

// Todo: Based on cni version validate different state files
func (v *LinuxValidator) ValidateStateFile(ctx context.Context) error {
	checkSet := make(map[string][]check) // key is cni type, value is a list of check
	// TODO: add cniv1 when adding Linux related test cases
	checkSet["cilium"] = []check{
		{"cns", cnsStateFileIps, cnsLabelSelector, privilegedNamespace, cnsStateFileCmd},
		{"cilium", ciliumStateFileIps, ciliumLabelSelector, privilegedNamespace, ciliumStateFileCmd},
		{"cns cache", cnsCacheStateFileIps, cnsLabelSelector, privilegedNamespace, cnsLocalCacheCmd},
	}

	checkSet["cniv2"] = []check{
		{"cns cache", cnsCacheStateFileIps, cnsLabelSelector, privilegedNamespace, cnsLocalCacheCmd},
	}

	for _, check := range checkSet[v.cni] {
		err := v.validateIPs(ctx, check.stateFileIps, check.cmd, check.name, check.podNamespace, check.podLabelSelector)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *LinuxValidator) ValidateRestartNetwork(ctx context.Context) error {
	nodes, err := k8sutils.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := k8sutils.GetPodsByNode(ctx, v.clientset, privilegedNamespace, privilegedLabelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}

		privelegedPod := pod.Items[0]
		// exec into the pod to get the state file
		_, err = k8sutils.ExecCmdOnPod(ctx, v.clientset, privilegedNamespace, privelegedPod.Name, restartNetworkCmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod")
		}
		err = k8sutils.WaitForPodsRunning(ctx, v.clientset, "", "")
		if err != nil {
			return errors.Wrapf(err, "failed to wait for pods running")
		}
	}
	return nil
}

func cnsStateFileIps(result []byte) (map[string]string, error) {
	var cnsResult CnsState
	err := json.Unmarshal(result, &cnsResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cns endpoint list")
	}

	cnsPodIps := make(map[string]string)
	for _, v := range cnsResult.Endpoints {
		for ifName, ip := range v.IfnameToIPMap {
			if ifName == "eth0" {
				ip := ip.IPv4[0].IP.String()
				cnsPodIps[ip] = v.PodName
			}
		}
	}
	return cnsPodIps, nil
}

func ciliumStateFileIps(result []byte) (map[string]string, error) {
	var ciliumResult []CiliumEndpointStatus
	err := json.Unmarshal(result, &ciliumResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cilium endpoint list")
	}

	ciliumPodIps := make(map[string]string)
	for _, v := range ciliumResult {
		for _, addr := range v.Status.Networking.Addresses {
			if addr.Addr != "" {
				ciliumPodIps[addr.Addr] = v.Status.Networking.InterfaceName
			}
		}
	}
	return ciliumPodIps, nil
}

func cnsCacheStateFileIps(result []byte) (map[string]string, error) {
	var cnsLocalCache CNSLocalCache

	err := json.Unmarshal(result, &cnsLocalCache)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cns local cache")
	}

	cnsPodIps := make(map[string]string)
	for index := range cnsLocalCache.IPConfigurationStatus {
		cnsPodIps[cnsLocalCache.IPConfigurationStatus[index].IPAddress] = cnsLocalCache.IPConfigurationStatus[index].PodInfo.Name()
	}
	return cnsPodIps, nil
}

func (v *LinuxValidator) validateIPs(ctx context.Context, stateFileIps stateFileIpsFunc, cmd []string, checkType, namespace, labelSelector string) error {
	log.Printf("Validating %s state file", checkType)
	nodes, err := k8sutils.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		// get the privileged pod
		pod, err := k8sutils.GetPodsByNode(ctx, v.clientset, namespace, labelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}
		podName := pod.Items[0].Name
		// exec into the pod to get the state file
		result, err := k8sutils.ExecCmdOnPod(ctx, v.clientset, namespace, podName, cmd, v.config)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod")
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

		check := compareIPs(filePodIps, podIps)

		if !check {
			return errors.Wrapf(errors.New("State file validation failed"), "for %s on node %s", checkType, nodes.Items[index].Name)
		}
	}
	log.Printf("State file validation for %s passed", checkType)
	return nil
}
