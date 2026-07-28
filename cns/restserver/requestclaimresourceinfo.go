package restserver

import (
	"context"
	"net"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// swiftV2NICMiddleware is the subset of the SWIFT v2 middleware that the
// RequestClaimResourceInfo handler needs. *middlewares.K8sSWIFTv2Middleware
// satisfies it; the handler is a no-op for other middleware modes.
type swiftV2NICMiddleware interface {
	GetPodInfoByClaimUID(ctx context.Context, claimUID k8stypes.UID) (cns.PodInfo, types.ResponseCode, string)
	GetSwiftV2IPConfigs(ctx context.Context, podInfo cns.PodInfo) ([]cns.PodIpInfo, error)
	GetPodNICMACs(ctx context.Context, podInfo cns.PodInfo) ([]string, error)
}

// requestClaimResourceInfo is the handler for the RequestClaimResourceInfo API. It
// returns, in a single response, the pod's IP configs (the same set RequestIPConfigs
// produces, but WITHOUT the IsScheduledWithDRA filtering, so delegated NIC configs
// are always included) plus the resource-slice properties of every NIC allocated to
// the pod.
func (service *HTTPRestService) requestClaimResourceInfo(w http.ResponseWriter, r *http.Request) {
	l := service.Logger
	if l == nil {
		l = zap.NewNop()
	}
	ctx := r.Context()

	if !service.swiftV2PrefixAllocationEnabled() {
		respondJSON(w, http.StatusNotFound, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: types.UnsupportedAPI, Message: "requestClaimResourceInfo: SwiftV2 prefix allocation is not enabled"},
		})
		return
	}

	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusBadRequest, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: types.UnsupportedVerb, Message: "requestClaimResourceInfo only supports POST"},
		})
		return
	}

	var req cns.ClaimResourceInfoRequest
	if err := common.Decode(w, r, &req); err != nil {
		l.Error("failed to decode request", zap.Error(err))
		return
	}

	mw, ok := service.IPConfigsHandlerMiddleware.(swiftV2NICMiddleware)
	if !ok {
		respondJSON(w, http.StatusServiceUnavailable, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: types.UnexpectedError, Message: "requestClaimResourceInfo: SWIFT v2 middleware is not configured"},
		})
		return
	}

	// Resolve the DRA ResourceClaim to the pod that owns it via the pod's MTPNC.
	podInfo, respCode, message := mw.GetPodInfoByClaimUID(ctx, req.ClaimUID)
	if respCode != types.Success {
		respondJSON(w, http.StatusBadRequest, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: respCode, Message: message},
		})
		return
	}

	// 1) IP configs: the pod's SWIFT v2 delegated configs, including DRA allocations.
	podIPInfo, err := mw.GetSwiftV2IPConfigs(ctx, podInfo)
	if err != nil {
		l.Error("failed to get SWIFT v2 IP configs", zap.String("pod", podInfo.Name()), zap.String("namespace", podInfo.Namespace()), zap.Error(err))
		respondJSON(w, http.StatusInternalServerError, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: types.FailedToAllocateIPConfig, Message: errors.Wrap(err, "failed to get SWIFT v2 IP configs").Error()},
		})
		return
	}

	// 2) NIC resources: resource-slice properties for every NIC allocated to the pod.
	nicResources, err := service.podNICResources(ctx, l, mw, podInfo)
	if err != nil {
		l.Error("failed to get pod nic resources", zap.String("pod", podInfo.Name()), zap.String("namespace", podInfo.Namespace()), zap.Error(err))
		respondJSON(w, http.StatusInternalServerError, cns.ClaimResourceInfoResponse{
			Response: cns.Response{ReturnCode: types.UnexpectedError, Message: errors.Wrap(err, "failed to get pod nic resources").Error()},
		})
		return
	}

	respondJSON(w, http.StatusOK, cns.ClaimResourceInfoResponse{
		Response:     cns.Response{ReturnCode: types.Success},
		PodIPInfo:    podIPInfo,
		NICResources: nicResources,
	})
}

// podNICResources builds the resource-slice properties (networkID/subnetGUID/subnetName/
// capacity) for every NIC allocated to the pod, reusing the same enrichment as
// GetNICResources. The pod's NIC MACs come from its MTPNC; the per-NIC network/subnet/
// capacity comes from NICNetworkConfig, falling back to MTPNC for dedicated NICs.
func (service *HTTPRestService) podNICResources(ctx context.Context, l *zap.Logger, mw swiftV2NICMiddleware, podInfo cns.PodInfo) ([]cns.NICResource, error) {
	macs, err := mw.GetPodNICMACs(ctx, podInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pod NIC MACs")
	}

	var nicResourceNetworkInfoByMAC, mtpncResourceNetworkInfoByMAC map[string]*cns.NICResourceInfo
	if service.nicncClient != nil {
		if nicResourceNetworkInfoByMAC, err = service.nicncClient.GetNICResourceInfoFromNICNC(ctx); err != nil {
			l.Warn("failed to fetch NICNetworkConfig data", zap.Error(err))
		}
	}
	if service.mtpncClient != nil {
		if mtpncResourceNetworkInfoByMAC, err = service.mtpncClient.GetNICResourceInfoFromMTPNC(ctx); err != nil {
			l.Warn("failed to fetch MTPNC data", zap.Error(err))
		}
	}

	nicResources := make([]cns.NICResource, 0, len(macs))
	for _, mac := range macs {
		res := cns.NICResource{MacAddress: mac, Capacity: "1"} // default capacity is 1 if no NICNC or MTPNC is found
		key := mac
		if hw, parseErr := net.ParseMAC(mac); parseErr == nil {
			key = hw.String()
		}
		updateNICResource(&res, key, nicResourceNetworkInfoByMAC, mtpncResourceNetworkInfoByMAC)
		nicResources = append(nicResources, res)
	}
	return nicResources, nil
}
