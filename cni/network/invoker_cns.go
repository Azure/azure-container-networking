package network

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	cnsPort = 10090
)

type CNSIPAMInvoker struct {
	podName              string
	podNamespace         string
	primaryInterfaceName string
	cnsClient            *cnsclient.CNSClient
}

type IPv4ResultInfo struct {
	podIPAddress       string
	ncSubnetPrefix     uint8
	ncPrimaryIP        string
	ncGatewayIPAddress string
	hostSubnet         string
	hostPrimaryIP      string
	hostGateway        string
}

func NewCNSInvoker(podName, namespace string) (*CNSIPAMInvoker, error) {
	cnsURL := "http://localhost:" + strconv.Itoa(cnsPort)
	cnsClient, err := cnsclient.InitCnsClient(cnsURL)

	return &CNSIPAMInvoker{
		podName:      podName,
		podNamespace: namespace,
		cnsClient:    cnsClient,
	}, err
}

//Add uses the requestipconfig API in cns, and returns ipv4 and a nil ipv6 as CNS doesn't support IPv6 yet
func (invoker *CNSIPAMInvoker) Add(nwCfg *cni.NetworkConfig, subnetPrefix *net.IPNet, options map[string]interface{}) (*cniTypesCurr.Result, *cniTypesCurr.Result, error) {

	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{PodName: invoker.podName, PodNamespace: invoker.podNamespace}
	orchestratorContext, err := json.Marshal(podInfo)

	log.Printf("Requesting IP for pod %v", podInfo)
	response, err := invoker.cnsClient.RequestIPAddress(orchestratorContext)
	if err != nil {
		log.Printf("Failed to get IP address from CNS with error %v, response: %v", err, response)
		return nil, nil, err
	}

	resultIPv4 := IPv4ResultInfo{
		podIPAddress:       response.PodIpInfo.PodIPConfig.IPAddress,
		ncSubnetPrefix:     response.PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
		ncPrimaryIP:        response.PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress,
		ncGatewayIPAddress: response.PodIpInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress,
		hostSubnet:         response.PodIpInfo.HostPrimaryIPInfo.Subnet,
		hostPrimaryIP:      response.PodIpInfo.HostPrimaryIPInfo.PrimaryIP,
		hostGateway:        response.PodIpInfo.HostPrimaryIPInfo.Gateway,
	}

	ncgw := net.ParseIP(resultIPv4.ncGatewayIPAddress)
	if ncgw == nil {
		return nil, nil, fmt.Errorf("Gateway address %v from response is invalid", resultIPv4.ncGatewayIPAddress)
	}

	// set the NC Primary IP in options
	options[network.SNATIPKey] = resultIPv4.ncPrimaryIP

	// set host gateway in options
	options[network.HostGWKey] = resultIPv4.hostGateway

	log.Printf("Received result %+v for pod %v", resultIPv4, podInfo)

	result, err := getCNIIPv4Result(resultIPv4, subnetPrefix)
	if err != nil {
		return nil, nil, err
	}

	// first result is ipv4, second is ipv6, SWIFT doesn't currently support IPv6
	return result, nil, nil
}

func getCNIIPv4Result(info IPv4ResultInfo, subnetPrefix *net.IPNet) (*cniTypesCurr.Result, error) {

	gw := net.ParseIP(info.ncGatewayIPAddress)
	if gw == nil {
		return nil, fmt.Errorf("Gateway address %v from response is invalid", gw)
	}

	hostIP := net.ParseIP(info.hostPrimaryIP)
	if hostIP == nil {
		return nil, fmt.Errorf("Host IP address %v from response is invalid", hostIP)
	}

	// set result ipconfig from CNS Response Body
	ip, ipnet, err := net.ParseCIDR(info.podIPAddress + "/" + fmt.Sprint(info.ncSubnetPrefix))
	if ip == nil {
		return nil, fmt.Errorf("Unable to parse IP from response: %v", info.podIPAddress)
	}

	// get the name of the primary IP address
	_, hostIPNet, err := net.ParseCIDR(info.hostSubnet)
	if err != nil {
		return nil, err
	}

	// set subnet prefix for host vm
	*subnetPrefix = *hostIPNet

	// construct ipnet for result
	resultIPnet := net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}

	return &cniTypesCurr.Result{
		IPs: []*cniTypesCurr.IPConfig{
			{
				Version: "4",
				Address: resultIPnet,
				Gateway: gw,
			},
		},
		Routes: []*cniTypes.Route{
			{
				Dst: network.Ipv4DefaultRouteDstPrefix,
				GW:  gw,
			},
		},
	}, nil
}

// Delete calls into the releaseipconfiguration API in CNS
func (invoker *CNSIPAMInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, options map[string]interface{}) error {

	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{PodName: invoker.podName, PodNamespace: invoker.podNamespace}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return err
	}

	return invoker.cnsClient.ReleaseIPAddress(orchestratorContext)
}
