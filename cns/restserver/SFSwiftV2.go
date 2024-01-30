package restserver

import (
	"context"
	"fmt"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
)

type SFSWIFTv2Middleware struct {
	CnsClient *client.Client
}

// IPConfigsRequestHandlerWrapper is the middleware function for handling SWIFT v2 IP config requests for SF standalone scenario. This function wraps the default SWIFT request
// and release IP configs handlers.
func (m *SFSWIFTv2Middleware) IPConfigsRequestHandlerWrapper(defaultHandler, failureHandler cns.IPConfigsHandlerFunc) cns.IPConfigsHandlerFunc {
	return func(ctx context.Context, req *cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		podInfo, respCode, message := m.validateIPConfigsRequest(ctx, req)

		if respCode != types.Success {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: respCode,
					Message:    message,
				},
			}, errors.New("failed to validate ip configs request")
		}
		podInfo, err := cns.NewPodInfoFromIPConfigsRequest(*req)
		ipConfigsResp := &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.Success,
			},
			PodIPInfo: []cns.PodIpInfo{},
		}

		// If the pod is v2, get the infra IP configs from the handler first and then add the SWIFTv2 IP config
		defer func() {
			// Release the default IP config if there is an error
			if err != nil {
				_, err = failureHandler(ctx, req)
				if err != nil {
					logger.Errorf("failed to release default IP config : %v", err)
				}
			}
		}()
		if err != nil {
			return ipConfigsResp, err
		}
		SWIFTv2PodIPInfo, err := m.getIPConfig(ctx, podInfo)
		if err != nil {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.FailedToAllocateIPConfig,
					Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %v", err, req),
				},
				PodIPInfo: []cns.PodIpInfo{},
			}, errors.Wrapf(err, "failed to get SWIFTv2 IP config : %v", req)
		}
		ipConfigsResp.PodIPInfo = append(ipConfigsResp.PodIPInfo, SWIFTv2PodIPInfo)
		return ipConfigsResp, nil
	}
}

// validateIPConfigsRequest validates if pod orchestrator context is unmarshalled & sets secondary interfaces true
// nolint
func (m *SFSWIFTv2Middleware) validateIPConfigsRequest(ctx context.Context, req *cns.IPConfigsRequest) (podInfo cns.PodInfo, respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := errors.Wrapf(err, "failed to unmarshalling pod info from ipconfigs request %+v", req)
		return nil, types.UnexpectedError, errBuf.Error()
	}
	logger.Printf("[SWIFTv2Middleware] validate ipconfigs request for pod %s", podInfo.Name())

	req.SecondaryInterfacesExist = true
	// swiftv2 SF scenario for windows requires host interface info to be populated
	// which can be done only by restserver service function, hence setting this flag to do so in ipam.go
	req.AddInterfacesDataToResponse = true

	logger.Printf("[SWIFTv2Middleware] pod %s has secondary interface : %v", podInfo.Name(), req.SecondaryInterfacesExist)
	// retrieve podinfo from orchestrator context
	return podInfo, types.Success, ""
}

// getIPConfig returns the pod's SWIFT V2 IP configuration.
func (m *SFSWIFTv2Middleware) getIPConfig(ctx context.Context, podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	orchestratorContext, err := podInfo.OrchestratorContext()
	if err != nil {
		return cns.PodIpInfo{}, fmt.Errorf("error getting orchestrator context from PodInfo %w", err)
	}
	// call getNC via CNSClient
	resp, err := m.CnsClient.GetNetworkContainer(ctx, orchestratorContext)
	if err != nil {
		return cns.PodIpInfo{}, fmt.Errorf("error getNetworkContainerByOrchestrator Context %w", err)
	}

	// Check if the ncstate/ipconfig ready. If one of the fields is empty, return error
	if resp.IPConfiguration.IPSubnet.IPAddress == "" || resp.NetworkInterfaceInfo.MACAddress == "" || resp.NetworkContainerID == "" || resp.IPConfiguration.GatewayIPAddress == "" {
		return cns.PodIpInfo{}, fmt.Errorf("one of the fields for GetNCResponse is empty for given NC: %+v", resp) //nolint:goerr113 // return error
	}
	logger.Debugf("[SWIFTv2-SF] NetworkContainerResponse for pod %s is : %+v", podInfo.Name(), resp)

	podIPInfo := cns.PodIpInfo{
		PodIPConfig:                     resp.IPConfiguration.IPSubnet,
		MacAddress:                      resp.NetworkInterfaceInfo.MACAddress,
		NICType:                         resp.NetworkInterfaceInfo.NICType,
		SkipDefaultRoutes:               false,
		NetworkContainerPrimaryIPConfig: resp.IPConfiguration,
	}
	return podIPInfo, nil
}
