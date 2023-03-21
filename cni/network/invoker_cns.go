package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	cnscli "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/networkutils"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
)

var (
	errEmptyCNIArgs    = errors.New("empty CNI cmd args not allowed")
	errInvalidArgs     = errors.New("invalid arg(s)")
	overlayGatewayv4IP = "169.254.1.1"
	overlayGatewayv6IP = "fe80::1234:5678:9abc"
)

type CNSIPAMInvoker struct {
	podName       string
	podNamespace  string
	cnsClient     cnsclient
	executionMode util.ExecutionMode
	ipamMode      util.IpamMode
}

type IPResultInfo struct {
	podIPAddress       string
	ncSubnetPrefix     uint8
	ncPrimaryIP        string
	ncGatewayIPAddress string
	hostSubnet         string
	hostPrimaryIP      string
	hostGateway        string
}

func NewCNSInvoker(podName, namespace string, cnsClient cnsclient, executionMode util.ExecutionMode, ipamMode util.IpamMode) *CNSIPAMInvoker {
	return &CNSIPAMInvoker{
		podName:       podName,
		podNamespace:  namespace,
		cnsClient:     cnsClient,
		executionMode: executionMode,
		ipamMode:      ipamMode,
	}
}

// Add uses the requestipconfig API in cns, and returns ipv4 and a nil ipv6 as CNS doesn't support IPv6 yet
func (invoker *CNSIPAMInvoker) Add(addConfig IPAMAddConfig) (IPAMAddResult, error) {
	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{
		PodName:      invoker.podName,
		PodNamespace: invoker.podNamespace,
	}

	log.Printf(podInfo.PodName)
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return IPAMAddResult{}, errors.Wrap(err, "Failed to unmarshal orchestrator context during add: %w")
	}

	if addConfig.args == nil {
		return IPAMAddResult{}, errEmptyCNIArgs
	}

	ipconfig := cns.IPConfigsRequest{
		OrchestratorContext: orchestratorContext,
		PodInterfaceID:      GetEndpointID(addConfig.args),
		InfraContainerID:    addConfig.args.ContainerID,
	}

	log.Printf("Requesting IP for pod %+v using ipconfig %+v", podInfo, ipconfig)
	response, err := invoker.cnsClient.RequestIPs(context.TODO(), ipconfig)
	if err != nil {
		log.Printf("Failed to get IP address from CNS with error %v, response: %v", err, response)
		return IPAMAddResult{}, errors.Wrap(err, "Failed to get IP address from CNS with error: %w")
	}

	addResult := IPAMAddResult{}

	for i := 0; i < len(response.PodIPInfo); i++ {
		info := IPResultInfo{
			podIPAddress:       response.PodIPInfo[i].PodIPConfig.IPAddress,
			ncSubnetPrefix:     response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
			ncPrimaryIP:        response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress,
			ncGatewayIPAddress: response.PodIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPAddress,
			hostSubnet:         response.PodIPInfo[i].HostPrimaryIPInfo.Subnet,
			hostPrimaryIP:      response.PodIPInfo[i].HostPrimaryIPInfo.PrimaryIP,
			hostGateway:        response.PodIPInfo[i].HostPrimaryIPInfo.Gateway,
		}

		// set the NC Primary IP in options
		addConfig.options[network.SNATIPKey] = info.ncPrimaryIP

		log.Printf("[cni-invoker-cns] Received info %+v for pod %v", info, podInfo)

		ncgw := net.ParseIP(info.ncGatewayIPAddress)
		if ncgw == nil {
			if (invoker.ipamMode != util.V4Overlay) && (invoker.ipamMode != util.DualStackOverlay) {
				return IPAMAddResult{}, errors.Wrap(errInvalidArgs, "%w: Gateway address "+info.ncGatewayIPAddress+" from response is invalid")
			}

			if net.ParseIP(info.podIPAddress).To4() != nil {
				ncgw = net.ParseIP(overlayGatewayv4IP)
			} else {
				ncgw = net.ParseIP(overlayGatewayv6IP)
			}
		}

		// set result ipconfigArgument from CNS Response Body
		ip, ncIPNet, err := net.ParseCIDR(info.podIPAddress + "/" + fmt.Sprint(info.ncSubnetPrefix))
		if ip == nil {
			return IPAMAddResult{}, errors.Wrap(err, "Unable to parse IP from response: "+info.podIPAddress+" with err %w")
		}

		// construct ipnet for result
		resultIPnet := net.IPNet{
			IP:   ip,
			Mask: ncIPNet.Mask,
		}

		if net.ParseIP(info.podIPAddress).To4() != nil {
			addResult.ipv4Result = &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: resultIPnet,
						Gateway: ncgw,
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  ncgw,
					},
				},
			}
		} else {
			addResult.ipv6Result = &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: resultIPnet,
						Gateway: ncgw,
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv6DefaultRouteDstPrefix,
						GW:  ncgw,
					},
				},
			}
		}

		// get the name of the primary IP address
		_, hostIPNet, err := net.ParseCIDR(info.hostSubnet)
		if err != nil {
			return IPAMAddResult{}, fmt.Errorf("unable to parse hostSubnet: %w", err)
		}

		addResult.hostSubnetPrefix = *hostIPNet

		// set subnet prefix for host vm
		// setHostOptions will execute if IPAM mode is not v4 overlay
		if (invoker.ipamMode != util.V4Overlay) && (invoker.ipamMode != util.DualStackOverlay) {
			if err := setHostOptions(ncIPNet, addConfig.options, &info); err != nil {
				return IPAMAddResult{}, err
			}
		}
	}

	return addResult, nil
}

func setHostOptions(ncSubnetPrefix *net.IPNet, options map[string]interface{}, info *IPResultInfo) error {
	// get the host ip
	hostIP := net.ParseIP(info.hostPrimaryIP)
	if hostIP == nil {
		return fmt.Errorf("Host IP address %v from response is invalid", info.hostPrimaryIP)
	}

	// get host gateway
	hostGateway := net.ParseIP(info.hostGateway)
	if hostGateway == nil {
		return fmt.Errorf("Host Gateway %v from response is invalid", info.hostGateway)
	}

	// this route is needed when the vm on subnet A needs to send traffic to a pod in subnet B on a different vm
	options[network.RoutesKey] = []network.RouteInfo{
		{
			Dst: *ncSubnetPrefix,
			Gw:  hostGateway,
		},
	}

	azureDNSUDPMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureDNS, iptables.UDP, iptables.DNSPort)
	azureDNSTCPMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureDNS, iptables.TCP, iptables.DNSPort)
	azureIMDSMatch := fmt.Sprintf(" -m addrtype ! --dst-type local -s %s -d %s -p %s --dport %d", ncSubnetPrefix.String(), networkutils.AzureIMDS, iptables.TCP, iptables.HTTPPort)

	snatPrimaryIPJump := fmt.Sprintf("%s --to %s", iptables.Snat, info.ncPrimaryIP)
	// we need to snat IMDS traffic to node IP, this sets up snat '--to'
	snatHostIPJump := fmt.Sprintf("%s --to %s", iptables.Snat, info.hostPrimaryIP)

	var iptableCmds []iptables.IPTableEntry
	if !iptables.ChainExists(iptables.V4, iptables.Nat, iptables.Swift) {
		iptableCmds = append(iptableCmds, iptables.GetCreateChainCmd(iptables.V4, iptables.Nat, iptables.Swift))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Postrouting, "", iptables.Swift) {
		iptableCmds = append(iptableCmds, iptables.GetAppendIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Postrouting, "", iptables.Swift))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureDNSUDPMatch, snatPrimaryIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureDNSUDPMatch, snatPrimaryIPJump))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureDNSTCPMatch, snatPrimaryIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureDNSTCPMatch, snatPrimaryIPJump))
	}

	if !iptables.RuleExists(iptables.V4, iptables.Nat, iptables.Swift, azureIMDSMatch, snatHostIPJump) {
		iptableCmds = append(iptableCmds, iptables.GetInsertIptableRuleCmd(iptables.V4, iptables.Nat, iptables.Swift, azureIMDSMatch, snatHostIPJump))
	}

	options[network.IPTablesKey] = iptableCmds

	return nil
}

// Delete calls into the releaseipconfiguration API in CNS
func (invoker *CNSIPAMInvoker) Delete(addresses []*net.IPNet, nwCfg *cni.NetworkConfig, args *cniSkel.CmdArgs, _ map[string]interface{}) error {
	// Parse Pod arguments.
	podInfo := cns.KubernetesPodInfo{
		PodName:      invoker.podName,
		PodNamespace: invoker.podNamespace,
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return err
	}

	if args == nil {
		return errEmptyCNIArgs
	}

	req := cns.IPConfigsRequest{
		OrchestratorContext: orchestratorContext,
		PodInterfaceID:      GetEndpointID(args),
		InfraContainerID:    args.ContainerID,
	}

	if len(addresses) > 0 {
		req.DesiredIPAddresses = make([]string, len(addresses))
		for i, ipAddress := range addresses {
			req.DesiredIPAddresses[i] = ipAddress.IP.String()
		}
	} else {
		log.Printf("CNS invoker called with empty IP address")
	}

	if err := invoker.cnsClient.ReleaseIPs(context.TODO(), req); err != nil {
		// if we fail a release with a 404 error try using the old API
		if errors.Is(err, cnscli.ErrAPINotFound) {
			ipconfigRequest := cns.IPConfigRequest{
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      GetEndpointID(args),
				InfraContainerID:    args.ContainerID,
				DesiredIPAddress:    req.DesiredIPAddresses[0],
			}
			log.Errorf("Failed to release IPs using ReleaseIPs from CNS, going to try ReleaseIPAddress. error: %v request: %v", err, req)

			if err := invoker.cnsClient.ReleaseIPAddress(context.TODO(), ipconfigRequest); err != nil {
				// if the old API fails as well then we just return the error
				log.Errorf("Failed to release IP address from CNS using ReleaseIPAddress. error: %v request: %v", err, req)
				return errors.Wrap(err, fmt.Sprintf("failed to release IP %v using ReleaseIPAddress with err ", addresses)+"%w")
			}
		} else {
			log.Errorf("Failed to release IP address from CNS error: %v request: %v", err, req)
			return errors.Wrap(err, fmt.Sprintf("failed to release IP %v using ReleaseIPs with err ", addresses)+"%w")
		}

	}

	return nil
}
