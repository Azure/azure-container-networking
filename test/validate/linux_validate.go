package validate

import (
	"context"
	"encoding/json"

	"github.com/Azure/azure-container-networking/cns"
	restserver "github.com/Azure/azure-container-networking/cns/restserver"
	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

const (
	validatorPod            = "k8s-app=azure-cns"
	ciliumLabelSelector     = "k8s-app=cilium"
	overlayClusterLabelName = "overlay"
)

var (
	restartNetworkCmd           = []string{"bash", "-c", "systemctl restart systemd-networkd"}
	cnsManagedStateFileCmd      = []string{"bash", "-c", "cat /var/run/azure-cns/azure-endpoints.json"}
	azureVnetStateFileCmd       = []string{"bash", "-c", "cat /var/run/azure-vnet.json"}
	azureVnetIpamStateCmd       = []string{"bash", "-c", "cat /var/run/azure-vnet-ipam.json"}
	ciliumStateFileCmd          = []string{"cilium", "endpoint", "list", "-o", "json"}
	cnsCachedAssignedIPStateCmd = []string{"curl", "localhost:10090/debug/ipaddresses", "-d", "{\"IPConfigStateFilter\":[\"Assigned\"]}"}
)

type stateFileIpsFunc func([]byte) (map[string]string, error)

var linuxChecksMap = map[string][]check{
	"cilium": {
		{
			name:             "cns",
			stateFileIPs:     cnsManagedStateFileIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsManagedStateFileCmd,
		}, // cns configmap "ManageEndpointState": true, | Endpoints managed in CNS State File
		{
			name:             "cilium",
			stateFileIPs:     ciliumStateFileIps,
			podLabelSelector: ciliumLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              ciliumStateFileCmd,
			containerName:    "cilium-agent",
		},
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsCachedAssignedIPStateCmd,
		},
	},
	"cniv1": {
		{
			name:             "azure-vnet",
			stateFileIPs:     azureVnetStateIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetStateFileCmd,
		},
		{
			name:             "azure-vnet-ipam",
			stateFileIPs:     azureVnetIpamStateIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetIpamStateCmd,
		},
	},
	"cniv2": {
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsCachedAssignedIPStateCmd,
		},
		{
			name:             "azure-vnet",
			stateFileIPs:     azureVnetStateIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetStateFileCmd,
		}, // cns configmap "ManageEndpointState": false, | Endpoints managed in CNI State File
	},
	"dualstack": {
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsCachedAssignedIPStateCmd,
		},
		{
			name:             "azure dualstackoverlay",
			stateFileIPs:     azureVnetStateIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetStateFileCmd,
		},
	},
	"cilium_dualstack": {
		{
			name:             "cns dualstack",
			stateFileIPs:     cnsManagedStateFileDualStackIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsManagedStateFileCmd,
		}, // cns configmap "ManageEndpointState": true, | Endpoints managed in CNS State File
		{
			name:             "cilium",
			stateFileIPs:     ciliumStateFileDualStackIps,
			podLabelSelector: ciliumLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              ciliumStateFileCmd,
			containerName:    "cilium-agent",
		},
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: validatorPod,
			podNamespace:     privilegedNamespace,
			containerName:    "debug",
			cmd:              cnsCachedAssignedIPStateCmd,
		},
	},
}

type CnsManagedState struct {
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
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
}

// parse azure-vnet.json
// azure cni manages endpoint state
type AzureCniState struct {
	AzureCniState AzureVnetNetwork `json:"Network"`
}

type AzureVnetNetwork struct {
	Version            string                   `json:"Version"`
	TimeStamp          string                   `json:"TimeStamp"`
	ExternalInterfaces map[string]InterfaceInfo `json:"ExternalInterfaces"`
}

type InterfaceInfo struct {
	Name     string                          `json:"Name"`
	Networks map[string]AzureVnetNetworkInfo `json:"Networks"` // key: networkName, value: AzureVnetNetworkInfo
}

type AzureVnetInfo struct {
	Name     string
	Networks map[string]AzureVnetNetworkInfo // key: network name, value: NetworkInfo
}

type AzureVnetNetworkInfo struct {
	ID        string
	Mode      string
	Subnets   []Subnet
	Endpoints map[string]AzureVnetEndpointInfo // key: azure endpoint name, value: AzureVnetEndpointInfo
	PodName   string
}

type Subnet struct {
	Family    int
	Prefix    Prefix
	Gateway   string
	PrimaryIP string
}

type Prefix struct {
	IP   string
	Mask string
}

type AzureVnetEndpointInfo struct {
	IfName      string
	MacAddress  string
	IPAddresses []Prefix
	PodName     string
}

func cnsManagedStateFileDualStackIps(result []byte) (map[string]string, error) {
	var cnsResult CnsManagedState
	err := json.Unmarshal(result, &cnsResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cns endpoint list")
	}

	cnsPodIps := make(map[string]string)
	for _, v := range cnsResult.Endpoints {
		for ifName, ip := range v.IfnameToIPMap {
			if ifName == "eth0" {
				cnsPodIps[ip.IPv4[0].IP.String()] = v.PodName
				cnsPodIps[ip.IPv6[0].IP.String()] = v.PodName
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
			if addr.IPv4 != "" {
				ciliumPodIps[addr.IPv4] = v.Status.Networking.InterfaceName
			}
		}
	}
	return ciliumPodIps, nil
}

func ciliumStateFileDualStackIps(result []byte) (map[string]string, error) {
	var ciliumResult []CiliumEndpointStatus
	err := json.Unmarshal(result, &ciliumResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cilium endpoint list")
	}

	ciliumPodIps := make(map[string]string)
	for _, v := range ciliumResult {
		for _, addr := range v.Status.Networking.Addresses {
			if addr.IPv4 != "" && addr.IPv6 != "" {
				ciliumPodIps[addr.IPv4] = v.Status.Networking.InterfaceName
				ciliumPodIps[addr.IPv6] = v.Status.Networking.InterfaceName
			}
		}
	}
	return ciliumPodIps, nil
}

func azureVnetStateIps(result []byte) (map[string]string, error) {
	var azureVnetResult AzureCniState
	err := json.Unmarshal(result, &azureVnetResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal azure vnet")
	}

	azureVnetPodIps := make(map[string]string)
	for _, v := range azureVnetResult.AzureCniState.ExternalInterfaces {
		for _, v := range v.Networks {
			for _, e := range v.Endpoints {
				for _, v := range e.IPAddresses {
					// collect both ipv4 and ipv6 addresses
					azureVnetPodIps[v.IP] = e.IfName
				}
			}
		}
	}
	return azureVnetPodIps, nil
}

func azureVnetIpamStateIps(result []byte) (map[string]string, error) {
	var azureVnetIpamResult AzureVnetIpam
	err := json.Unmarshal(result, &azureVnetIpamResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal azure vnet ipam")
	}

	azureVnetIpamPodIps := make(map[string]string)

	for _, v := range azureVnetIpamResult.IPAM.AddrSpaces {
		for _, v := range v.Pools {
			for _, v := range v.Addresses {
				if v.InUse {
					azureVnetIpamPodIps[v.Addr.String()] = v.Addr.String()
				}
			}
		}
	}
	return azureVnetIpamPodIps, nil
}

// Linux only function
func (v *Validator) validateRestartNetwork(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeList(ctx, v.clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		node := nodes.Items[index]
		if node.Status.NodeInfo.OperatingSystem != string(corev1.Linux) {
			continue
		}
		// get the privileged pod
		pod, err := acnk8s.GetPodsByNode(ctx, v.clientset, privilegedNamespace, privilegedLabelSelector, node.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod")
		}
		if len(pod.Items) == 0 {
			return errors.Errorf("there are no privileged pods on node - %v", node.Name)
		}
		privilegedPod := pod.Items[0]
		// exec into the pod to get the state file
		_, _, err = acnk8s.ExecCmdOnPod(ctx, v.clientset, privilegedNamespace, privilegedPod.Name, "", restartNetworkCmd, v.config, true)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod %s on node %s", privilegedPod.Name, node.Name)
		}
		err = acnk8s.WaitForPodsRunning(ctx, v.clientset, "", "")
		if err != nil {
			return errors.Wrapf(err, "failed to wait for pods running")
		}
	}
	return nil
}
