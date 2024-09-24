package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	cnsWinLabelSelector = "k8s-app=azure-cns-win"
)

var (
	hnsEndPointCmd                 = []string{"powershell", "-c", "Get-HnsEndpoint | ConvertTo-Json"}
	hnsNetworkCmd                  = []string{"powershell", "-c", "Get-HnsNetwork | ConvertTo-Json"}
	azureVnetCmd                   = []string{"powershell", "-c", "cat ../../k/azure-vnet.json"}
	azureVnetIpamCmd               = []string{"powershell", "-c", "cat ../../k/azure-vnet-ipam.json"}
	cnsWinManagedStateFileCmd      = []string{"powershell", "-c", "cat ../../k/azurecns/azure-endpoints.json"}
	cnsWinCachedAssignedIPStateCmd = []string{
		"powershell", "Invoke-WebRequest -Uri 127.0.0.1:10090/debug/ipaddresses",
		"-Method Post -ContentType application/x-www-form-urlencoded",
		"-Body \"{`\"IPConfigStateFilter`\":[`\"Assigned`\"]}\"",
		"-UseBasicParsing | Select-Object -Expand Content",
	}
)

var windowsChecksMap = map[string][]check{
	"cniv1": {
		{
			name:             "hns",
			stateFileIPs:     hnsStateFileIPs,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              hnsEndPointCmd,
		},
		{
			name:             "azure-vnet",
			stateFileIPs:     azureVnetIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetCmd,
		},
		{
			name:             "azure-vnet-ipam",
			stateFileIPs:     azureVnetIpamIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetIpamCmd,
		},
	},
	"cniv2": {
		{
			name:             "hns",
			stateFileIPs:     hnsStateFileIPs,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              hnsEndPointCmd,
		},
		{
			name:             "azure-vnet",
			stateFileIPs:     azureVnetIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              azureVnetCmd,
		},
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: cnsWinLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              cnsWinCachedAssignedIPStateCmd,
		},
	},
	"stateless": {
		{
			name:             "hns",
			stateFileIPs:     hnsStateFileIPs,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              hnsEndPointCmd,
		},
		{
			name:             "cns",
			stateFileIPs:     cnsManagedStateFileIps,
			podLabelSelector: privilegedLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              cnsWinManagedStateFileCmd,
		}, // cns configmap "ManageEndpointState": true, | Endpoints managed in CNS State File
		{
			name:             "cns cache",
			stateFileIPs:     cnsCacheStateFileIps,
			podLabelSelector: cnsWinLabelSelector,
			podNamespace:     privilegedNamespace,
			cmd:              cnsWinCachedAssignedIPStateCmd,
		},
	},
}

type HNSEndpoint struct {
	MacAddress       string `json:"MacAddress"`
	IPAddress        net.IP `json:"IPAddress"`
	IPv6Address      net.IP `json:"IPv6Address"`
	IsRemoteEndpoint bool   `json:",omitempty"`
}

type HNSNetwork struct {
	Name           string `json:"Name"`
	IPv6           bool   `json:"IPv6"`
	ManagementIP   string `json:"ManagementIP"`
	ManagementIPv6 string `json:"ManagementIPv6"`
	State          int    `json:"State"`
}

type AzureVnet struct {
	NetworkInfo NetworkInfo `json:"Network"`
}

type NetworkInfo struct {
	ExternalInterfaces map[string]ExternalInterface `json:"ExternalInterfaces"`
}

type ExternalInterface struct {
	Networks map[string]Network `json:"Networks"`
}

type Network struct {
	Endpoints map[string]Endpoint `json:"Endpoints"`
}

type Endpoint struct {
	IPAddresses []net.IPNet `json:"IPAddresses"`
	IfName      string      `json:"IfName"`
}

type AzureVnetIpam struct {
	IPAM AddressSpaces `json:"IPAM"`
}

type AddressSpaces struct {
	AddrSpaces map[string]AddressSpace `json:"AddressSpaces"`
}

type AddressSpace struct {
	Pools map[string]AddressPool `json:"Pools"`
}

type AddressPool struct {
	Addresses map[string]AddressRecord `json:"Addresses"`
}

type AddressRecord struct {
	Addr  net.IP
	InUse bool
}

func hnsStateFileIPs(result []byte) (map[string]string, error) {
	jsonType := bytes.TrimLeft(result, " \t\r\n")
	isObject := jsonType[0] == '{'
	isArray := jsonType[0] == '['
	hnsPodIps := make(map[string]string)

	switch {
	case isObject:
		var hnsResult HNSEndpoint
		err := json.Unmarshal(result, &hnsResult)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal hns endpoint list")
		}
		if !hnsResult.IsRemoteEndpoint {
			hnsPodIps[hnsResult.IPAddress.String()] = hnsResult.MacAddress
			if hnsResult.IPv6Address.String() != "<nil>" {
				hnsPodIps[hnsResult.IPv6Address.String()] = hnsResult.MacAddress
			}
		}
	case isArray:
		var hnsResult []HNSEndpoint
		err := json.Unmarshal(result, &hnsResult)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal hns endpoint list")
		}

		for _, v := range hnsResult {
			if !v.IsRemoteEndpoint {
				hnsPodIps[v.IPAddress.String()] = v.MacAddress
				if v.IPv6Address.String() != "<nil>" {
					hnsPodIps[v.IPv6Address.String()] = v.MacAddress
				}

			}
		}
	default:
		log.Printf("Leading character is - %v", jsonType[0])
		return nil, errors.New("json is malformed and does not have correct leading character")
	}

	return hnsPodIps, nil
}

// return windows HNS network state
func hnsNetworkState(result []byte) ([]HNSNetwork, error) {
	var hnsNetworkResult []HNSNetwork
	err := json.Unmarshal(result, &hnsNetworkResult)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal HNS network state file")
	}

	return hnsNetworkResult, nil
}

func azureVnetIps(result []byte) (map[string]string, error) {
	var azureVnetResult AzureVnet
	err := json.Unmarshal(result, &azureVnetResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal azure vnet")
	}

	azureVnetPodIps := make(map[string]string)
	for _, v := range azureVnetResult.NetworkInfo.ExternalInterfaces {
		for _, v := range v.Networks {
			for _, e := range v.Endpoints {
				for _, v := range e.IPAddresses {
					// collect both ipv4 and ipv6 addresses
					azureVnetPodIps[v.IP.String()] = e.IfName
				}
			}
		}
	}
	return azureVnetPodIps, nil
}

func azureVnetIpamIps(result []byte) (map[string]string, error) {
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

func validateHNSNetworkState(ctx context.Context, nodes *corev1.NodeList, clientset *kubernetes.Clientset, restConfig *rest.Config) error {
	// check windows HNS network state
	for index := range nodes.Items {
		pod, err := acnk8s.GetPodsByNode(ctx, clientset, privilegedNamespace, privilegedLabelSelector, nodes.Items[index].Name)
		if err != nil {
			return errors.Wrap(err, "failed to get privileged pod")
		}
		if len(pod.Items) == 0 {
			return errors.Errorf("there are no privileged pods on node - %v", nodes.Items[index].Name)
		}
		podName := pod.Items[0].Name
		// exec into the pod to get the state file
		result, err := acnk8s.ExecCmdOnPod(ctx, clientset, privilegedNamespace, podName, "", hnsNetworkCmd, restConfig)
		if err != nil {
			return errors.Wrap(err, "failed to exec into privileged pod")
		}

		hnsNetwork, err := hnsNetworkState(result)
		log.Printf("hnsNetwork: %+v", hnsNetwork)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal HNS state file")
		}

		// check hns properties
		if len(hnsNetwork) == 1 {
			return errors.New("HNS default ext network or azure network does not exist")
		}

		for _, network := range hnsNetwork {
			if network.State != 1 {
				return errors.New("windows HNS network state is not correct")
			}
			if network.ManagementIP == "" {
				return errors.New("windows HNS network is missing ipv4 management IP")
			}

			// dualstack scenario
			if network.IPv6 {
				if network.ManagementIPv6 == "" {
					return errors.New("windows HNS network is missing ipv6 management IP")
				}
			}
		}
	}
	return nil
}
