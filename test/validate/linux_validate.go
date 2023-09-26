package validate

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/cns"
	restserver "github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/pkg/errors"
)

const (
	cnsLabelSelector        = "k8s-app=azure-cns"
	ciliumLabelSelector     = "k8s-app=cilium"
	overlayClusterLabelName = "overlay"
)

var (
	restartNetworkCmd      = []string{"bash", "-c", "chroot /host /bin/bash -c systemctl restart systemd-networkd"}
	cnsManagedStateFileCmd = []string{"bash", "-c", "cat /var/run/azure-cns/azure-endpoints.json"}
	azureVnetStateFileCmd  = []string{"bash", "-c", "cat /var/run/azure-vnet.json"}
	azureVnetIpamStateCmd  = []string{"bash", "-c", "cat /var/run/azure-vnet-ipam.json"}
	ciliumStateFileCmd     = []string{"bash", "-c", "cilium endpoint list -o json"}
	cnsLocalCacheCmd       = []string{"curl", "localhost:10090/debug/ipaddresses", "-d", "{\"IPConfigStateFilter\":[\"Assigned\"]}"}
)

type stateFileIpsFunc func([]byte) (map[string]string, error)

var linuxChecksMap = map[string][]check{
	"cilium": {
		{"cns", cnsManagedStateFileIps, cnsLabelSelector, privilegedNamespace, cnsManagedStateFileCmd}, // cns configmap "ManageEndpointState": true, | Endpoints managed in CNS State File
		{"cilium", ciliumStateFileIps, ciliumLabelSelector, privilegedNamespace, ciliumStateFileCmd},
		{"cns cache", cnsCacheStateFileIps, cnsLabelSelector, privilegedNamespace, cnsLocalCacheCmd},
	},
	"cniv1": {
		{"azure-vnet", azureVnetStateIps, privilegedLabelSelector, privilegedNamespace, azureVnetStateFileCmd},
		{"azure-vnet-ipam", azureVnetIpamStateIps, privilegedLabelSelector, privilegedNamespace, azureVnetIpamStateCmd},
	},
	"cniv2": {
		{"cns cache", cnsCacheStateFileIps, cnsLabelSelector, privilegedNamespace, cnsLocalCacheCmd},
		{"azure-vnet", azureVnetStateIps, privilegedLabelSelector, privilegedNamespace, azureVnetStateFileCmd}, // cns configmap "ManageEndpointState": false, | Endpoints managed in CNI State File
	},
	"dualstack": {
		{"cns cache", cnsCacheStateFileIps, cnsLabelSelector, privilegedNamespace, cnsLocalCacheCmd},
		{"azure dualstackoverlay", azureVnetStateIps, privilegedLabelSelector, privilegedNamespace, azureVnetStateFileCmd},
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
	Addr string `json:"ipv4"`
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

func cnsManagedStateFileIps(result []byte) (map[string]string, error) {
	var cnsResult CnsManagedState
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
