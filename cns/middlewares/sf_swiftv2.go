package middlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"net/http"
	"net/http/httptest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	mux *http.ServeMux
)

type SFSWIFTv2Middleware struct {
	Cli client.Client
	st  string
}

// ValidateIPConfigsRequest validates if pod is multitenant by checking the pod labels, used in SWIFT V2 scenario.
// nolint
func (m *SFSWIFTv2Middleware) ValidateIPConfigsRequests(ctx context.Context, req *cns.IPConfigsRequest) (respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := errors.Wrapf(err, "failed to unmarshalling pod info from ipconfigs request %+v", req)
		return types.UnexpectedError, errBuf.Error()
	}
	logger.Printf("[SWIFTv2Middleware] validate ipconfigs request for pod %s", podInfo.Name())
	podNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod := v1.Pod{}
	if err := m.Cli.Get(ctx, podNamespacedName, &pod); err != nil {
		errBuf := errors.Wrapf(err, "failed to get pod %+v", podNamespacedName)
		return types.UnexpectedError, errBuf.Error()
	}
	req.SecondaryInterfacesExist = true
	logger.Printf("[SWIFTv2Middleware] pod %s has secondary interface : %v", podInfo.Name(), req.SecondaryInterfacesExist)
	return types.Success, ""
}

// GetIPConfig returns the pod's SWIFT V2 IP configuration.
func (m *SFSWIFTv2Middleware) GetIPConfigs(podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	var body bytes.Buffer
	var resp cns.GetNetworkContainerResponse

	podInfoBytes, err := json.Marshal(podInfo)
	getReq := &cns.GetNetworkContainerRequest{OrchestratorContext: podInfoBytes}

	json.NewEncoder(&body).Encode(getReq)
	req, err := http.NewRequest(http.MethodPost, cns.GetNetworkContainerByOrchestratorContext, &body)
	if err != nil {
		return cns.PodIpInfo{}, fmt.Errorf("sending post request to GetNetworkContainerByOrchestratorContext: %w", err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	err = json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		return cns.PodIpInfo{}, fmt.Errorf("decoding response: %w", err)
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if resp.IPConfiguration.IPSubnet.IPAddress == "" || resp.NetworkInterfaceInfo.MACAddress == "" || resp.NetworkContainerID == "" || resp.IPConfiguration.GatewayIPAddress == "" {
		return cns.PodIpInfo{}, errMTPNCNotReady
	}
	logger.Printf("[SWIFTv2Middleware] networkcontainerrequest for pod %s is : %+v", podInfo.Name(), resp)

	podIPInfo := cns.PodIpInfo{
		PodIPConfig:       resp.IPConfiguration.IPSubnet,
		MacAddress:        resp.NetworkInterfaceInfo.MACAddress,
		NICType:           resp.NetworkInterfaceInfo.NICType,
		SkipDefaultRoutes: false,
		// InterfaceName is empty for DelegatedVMNIC
	}

	return podIPInfo, nil
}
