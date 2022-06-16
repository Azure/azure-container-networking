package main

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
)

func createCNSRequest(args *cniSkel.CmdArgs) (cns.IPConfigRequest, error) {
	podConf, err := parsePodConf(args.Args)
	if err != nil {
		return cns.IPConfigRequest{}, errors.Wrapf(err, "Could not parse CNI args")
	}

	podInfo := cns.KubernetesPodInfo{
		PodName:      string(podConf.K8S_POD_NAME),
		PodNamespace: string(podConf.K8S_POD_NAMESPACE),
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		return cns.IPConfigRequest{}, errors.Wrapf(err, "Could not marshal podInfo to JSON")
	}

	req := cns.IPConfigRequest{
		PodInterfaceID:      getEndpointID(args.ContainerID, args.IfName),
		InfraContainerID:    args.ContainerID,
		OrchestratorContext: orchestratorContext,
	}

	return req, nil
}

func processCNSResponse(resp *cns.IPConfigResponse) (*net.IPNet, net.IP, error) {
	podCIDR := fmt.Sprintf(
		"%s/%d",
		resp.PodIpInfo.PodIPConfig.IPAddress,
		resp.PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength,
	)
	podIP, podIPNet, err := net.ParseCIDR(podCIDR)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "CNS returned invalid pod CIDR %q", podCIDR)
	}

	resultIPNet := &net.IPNet{
		IP:   podIP,
		Mask: podIPNet.Mask,
	}

	ncGatewayIPAddress := resp.PodIpInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress
	gwIP := net.ParseIP(ncGatewayIPAddress)
	if gwIP == nil {
		return nil, nil, fmt.Errorf("CNS returned an invalid gateway address: %s", ncGatewayIPAddress)
	}

	return resultIPNet, gwIP, nil
}

type K8SPodEnvArgs struct {
	cniTypes.CommonArgs
	K8S_POD_NAMESPACE          cniTypes.UnmarshallableString `json:"K8S_POD_NAMESPACE,omitempty"`
	K8S_POD_NAME               cniTypes.UnmarshallableString `json:"K8S_POD_NAME,omitempty"`
	K8S_POD_INFRA_CONTAINER_ID cniTypes.UnmarshallableString `json:"K8S_POD_INFRA_CONTAINER_ID,omitempty"`
}

func parsePodConf(args string) (*K8SPodEnvArgs, error) {
	podCfg := K8SPodEnvArgs{}
	err := cniTypes.LoadArgs(args, &podCfg)
	if err != nil {
		return nil, err
	}
	return &podCfg, nil
}

func getEndpointID(containerID string, ifName string) string {
	if len(containerID) > 8 {
		containerID = containerID[:8]
	} else {
		log.Printf("Container ID length is not greater than 8: %v", containerID)
		return ""
	}

	infraEpName := containerID + "-" + ifName

	return infraEpName
}
