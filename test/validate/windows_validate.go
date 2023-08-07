package validate

import (
	"encoding/json"
	"net"

	"github.com/pkg/errors"
)

var (
	hnsEndPointCmd   = []string{"powershell", "-c", "Get-HnsEndpoint | ConvertTo-Json"}
	azureVnetCmd     = []string{"powershell", "-c", "cat ../../k/azure-vnet.json"}
	azureVnetIpamCmd = []string{"powershell", "-c", "cat ../../k/azure-vnet-ipam.json"}
)

var windowsChecksMap = map[string][]check{
	"cniv1": {
		{"hns", hnsStateFileIps, privilegedLabelSelector, privilegedNamespace, hnsEndPointCmd},
		{"azure-vnet", azureVnetIps, privilegedLabelSelector, privilegedNamespace, azureVnetCmd},
		{"azure-vnet-ipam", azureVnetIpamIps, privilegedLabelSelector, privilegedNamespace, azureVnetIpamCmd},
	},
	"cniv2": {
		{"azure-vnet", azureVnetIps, privilegedLabelSelector, privilegedNamespace, azureVnetCmd},
	},
}

type HNSEndpoint struct {
	MacAddress       string `json:"MacAddress"`
	IPAddress        net.IP `json:"IPAddress"`
	IPv6Address      net.IP `json:",omitempty"`
	IsRemoteEndpoint bool   `json:",omitempty"`
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

func hnsStateFileIps(result []byte) (map[string]string, error) {
	var hnsResult []HNSEndpoint
	err := json.Unmarshal(result, &hnsResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal hns endpoint list")
	}

	hnsPodIps := make(map[string]string)
	for _, v := range hnsResult {
		if !v.IsRemoteEndpoint {
			hnsPodIps[v.IPAddress.String()] = v.MacAddress
		}
	}
	return hnsPodIps, nil
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
