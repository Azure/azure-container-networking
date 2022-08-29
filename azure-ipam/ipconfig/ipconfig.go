package ipconfig

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

// CreateIPConfigReq creates an IPConfigRequest from the given CNI args.
func CreateIPConfigReq(args *cniSkel.CmdArgs) (cns.IPConfigRequest, error) {
	podConf, err := parsePodConf(args.Args)
	if err != nil {
		return cns.IPConfigRequest{}, errors.Wrapf(err, "failed to parse pod config from CNI args")
	}

	podInfo := cns.KubernetesPodInfo{
		PodName:      string(podConf.K8S_POD_NAME),
		PodNamespace: string(podConf.K8S_POD_NAMESPACE),
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return cns.IPConfigRequest{}, errors.Wrapf(err, "failed to marshal podInfo to JSON")
	}

	req := cns.IPConfigRequest{
		PodInterfaceID:      args.ContainerID,
		InfraContainerID:    args.ContainerID,
		OrchestratorContext: orchestratorContext,
		Ifname:              args.IfName,
	}

	return req, nil
}

// ProcessIPConfigResp processes the IPConfigResponse from the CNS.
func ProcessIPConfigResp(resp *cns.IPConfigResponse) (*netip.Prefix, *netip.Addr, error) {
	podCIDR := fmt.Sprintf(
		"%s/%d",
		resp.PodIpInfo.PodIPConfig.IPAddress,
		resp.PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
	)
	podIPNet, err := netip.ParsePrefix(podCIDR)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cns returned invalid pod CIDR %q", podCIDR)
	}

	ncGatewayIPAddress := resp.PodIpInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress
	gwIP, err := netip.ParseAddr(ncGatewayIPAddress)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cns returned an invalid gateway address %q", ncGatewayIPAddress)
	}

	return &podIPNet, &gwIP, nil
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
