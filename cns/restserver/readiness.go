package restserver

import (
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
)

func (service *HTTPRestService) configureNetworkReadiness(
	requireNNC bool,
	requireNCProgrammed bool,
	requireIPAMReady bool,
) {
	service.Lock()
	defer service.Unlock()

	service.networkReadiness.requiresNNC = requireNNC
	service.networkReadiness.requiresNCProgrammed = requireNCProgrammed
	service.networkReadiness.requiresIPAMReady = requireIPAMReady
}

func (service *HTTPRestService) setIPAMReadyLocked(ncProgrammed bool) {
	service.networkReadiness.ipamReady = true
	service.networkReadiness.ncProgrammed = service.networkReadiness.ncProgrammed || ncProgrammed
}

func (service *HTTPRestService) setIPAMReady(ncProgrammed bool) {
	service.Lock()
	defer service.Unlock()

	service.setIPAMReadyLocked(ncProgrammed)
}

func (service *HTTPRestService) setCNIConflistWritten() {
	service.Lock()
	defer service.Unlock()

	service.networkReadiness.cniConflistWritten = true
}

func (service *HTTPRestService) networkReadinessResponse() cns.NetworkReadinessResponse {
	service.RLock()
	defer service.RUnlock()

	nncReceived := false
	if service.state != nil {
		nncReceived = len(service.state.ContainerStatus) > 0
	}

	details := cns.NetworkReadinessDetails{
		NNCReceived:          nncReceived,
		NCProgrammed:         service.networkReadiness.ncProgrammed,
		IPAMReady:            service.networkReadiness.ipamReady,
		CNIConflistWritten:   service.networkReadiness.cniConflistWritten,
		RequiresNNC:          service.networkReadiness.requiresNNC,
		RequiresNCProgrammed: service.networkReadiness.requiresNCProgrammed,
		RequiresIPAMReady:    service.networkReadiness.requiresIPAMReady,
		RequiresCNIConflist:  service.networkReadiness.requiresCNIConflist,
	}

	switch {
	case details.RequiresNNC && !details.NNCReceived:
		return notReadyResponse(cns.NetworkReadinessReasonNNCNotReceived, "nnc not received", details)
	case details.RequiresNCProgrammed && !details.NCProgrammed:
		return notReadyResponse(cns.NetworkReadinessReasonNCNotProgrammed, "nc not programmed", details)
	case details.RequiresIPAMReady && !details.IPAMReady:
		return notReadyResponse(cns.NetworkReadinessReasonIPAMNotReady, "ipam not ready", details)
	case details.RequiresCNIConflist && !details.CNIConflistWritten:
		return notReadyResponse(cns.NetworkReadinessReasonConflistNotWritten, "cni conflist not written", details)
	default:
		return cns.NetworkReadinessResponse{
			Response: cns.Response{
				ReturnCode: types.Success,
				Message:    string(cns.NetworkReadinessReasonReady),
			},
			State:   cns.NetworkReadinessStateReady,
			Reason:  cns.NetworkReadinessReasonReady,
			Details: details,
		}
	}
}

func notReadyResponse(
	reason cns.NetworkReadinessReason,
	message string,
	details cns.NetworkReadinessDetails,
) cns.NetworkReadinessResponse {
	return cns.NetworkReadinessResponse{
		Response: cns.Response{
			ReturnCode: types.NetworkNotReady,
			Message:    message,
		},
		State:   cns.NetworkReadinessStateNotReady,
		Reason:  reason,
		Message: message,
		Details: details,
	}
}

func (service *HTTPRestService) getNetworkReadiness(w http.ResponseWriter, _ *http.Request) {
	resp := service.networkReadinessResponse()
	if resp.State == cns.NetworkReadinessStateNotReady {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := common.Encode(w, &resp); err != nil {
		return
	}
}
