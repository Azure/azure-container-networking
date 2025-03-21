package ipconfig

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

func CreateOrchestratorContext(args *cniSkel.CmdArgs) ([]byte, error) {
	podConf, err := parsePodConf(args.Args)
	if err != nil {
		return []byte{}, errors.Wrapf(err, "failed to parse pod config from CNI args")
	}

	podInfo := cns.KubernetesPodInfo{
		PodName:      string(podConf.K8S_POD_NAME),
		PodNamespace: string(podConf.K8S_POD_NAMESPACE),
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return []byte{}, errors.Wrapf(err, "failed to marshal podInfo to JSON")
	}
	return orchestratorContext, nil
}

// CreateIPConfigReq creates an IPConfigRequest from the given CNI args.
func CreateIPConfigReq(args *cniSkel.CmdArgs) (cns.IPConfigRequest, error) {
	orchestratorContext, err := CreateOrchestratorContext(args)
	if err != nil {
		return cns.IPConfigRequest{}, errors.Wrapf(err, "failed to create orchestrator context")
	}

	req := cns.IPConfigRequest{
		PodInterfaceID:      args.ContainerID,
		InfraContainerID:    args.ContainerID,
		OrchestratorContext: orchestratorContext,
		Ifname:              args.IfName,
	}

	return req, nil
}

// CreateIPConfigReq creates an IPConfigsRequest from the given CNI args.
func CreateIPConfigsReq(args *cniSkel.CmdArgs) (cns.IPConfigsRequest, error) {
	orchestratorContext, err := CreateOrchestratorContext(args)
	if err != nil {
		return cns.IPConfigsRequest{}, errors.Wrapf(err, "failed to create orchestrator context")
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:      args.ContainerID,
		InfraContainerID:    args.ContainerID,
		OrchestratorContext: orchestratorContext,
		Ifname:              args.IfName,
	}

	return req, nil
}

func ProcessIPConfigsResp(resp *cns.IPConfigsResponse) (*[]netip.Prefix, *[]net.IP, error) {
	podIPNets := make([]netip.Prefix, len(resp.PodIPInfo))
	gatewaysIPs := make([]net.IP, len(resp.PodIPInfo))

	for i := range resp.PodIPInfo {
		var gatewayIP net.IP

		podCIDR := fmt.Sprintf(
			"%s/%d",
			resp.PodIPInfo[i].PodIPConfig.IPAddress,
			resp.PodIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
		)
		podIPNet, err := netip.ParsePrefix(podCIDR)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cns returned invalid pod CIDR %q", podCIDR)
		}
		podIPNets[i] = podIPNet

		if podIPNet.Addr().Is4() {
			gatewayIP = net.ParseIP(resp.PodIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPAddress)
		} else if podIPNet.Addr().Is6() {
			gatewayIP = net.ParseIP(resp.PodIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPv6Address)
		}

		if gatewayIP != nil {
			gatewaysIPs[i] = gatewayIP
		}
	}

	return &podIPNets, &gatewaysIPs, nil
}

type k8sPodEnvArgs struct {
	cniTypes.CommonArgs
	K8S_POD_NAMESPACE          cniTypes.UnmarshallableString `json:"K8S_POD_NAMESPACE,omitempty"`          // nolint
	K8S_POD_NAME               cniTypes.UnmarshallableString `json:"K8S_POD_NAME,omitempty"`               // nolint
	K8S_POD_INFRA_CONTAINER_ID cniTypes.UnmarshallableString `json:"K8S_POD_INFRA_CONTAINER_ID,omitempty"` // nolint
}

func parsePodConf(args string) (*k8sPodEnvArgs, error) {
	podCfg := k8sPodEnvArgs{}
	podCfg.CommonArgs.IgnoreUnknown = true
	err := cniTypes.LoadArgs(args, &podCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse pod config from env args")
	}
	return &podCfg, nil
}
