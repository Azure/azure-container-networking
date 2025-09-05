package restserver

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
)

// assignIBDevicesToPod handles POST requests to assign IB devices to a pod
func (service *HTTPRestService) assignIBDevicesToPod(w http.ResponseWriter, r *http.Request) {
	opName := "assignIBDevicesToPod"
	var req cns.AssignIBDevicesToPodRequest
	var response cns.AssignIBDevicesToPodResponse

	// Decode the request
	err := common.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	// Validate the request
	if err := validateAssignIBDevicesRequest(req); err != nil {
		response.Message = fmt.Sprintf("Invalid request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		err = common.Encode(w, &response)
		logger.Response(opName, response, types.InvalidRequest, err)
		return
	}

	// TODO: Check that the pod exists
	// TODO: Check that the IB devices are "unprogrammed" (i.e. available)
	// TODO: Create MTPNC with IB devices in spec (and update cache)

	// Report back a successful assignment
	response.Message = fmt.Sprintf("Successfully assigned %d IB devices to pod %s/%s",
		len(req.IBMACAddresses), req.PodNamespace, req.PodName)

	w.WriteHeader(http.StatusOK)
	err = common.Encode(w, &response)
	logger.Response(opName, response, types.Success, err)
}

func validateAssignIBDevicesRequest(req cns.AssignIBDevicesToPodRequest) error {
	if req.PodName == "" || req.PodNamespace == "" {
		return fmt.Errorf("pod name and namespace are required")
	}
	if len(req.IBMACAddresses) == 0 {
		return fmt.Errorf("at least one IB MAC address is required")
	}
	// TODO Make sure that the given MAC is valid too
	return nil
}
