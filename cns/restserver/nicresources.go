// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (service *HTTPRestService) getNICResources(w http.ResponseWriter, r *http.Request) {
	l := service.Logger
	if l == nil {
		l = zap.NewNop()
	}
	ctx := r.Context()

	if r.Method != http.MethodGet {
		returnMessage := fmt.Sprintf("[Azure CNS] Error. getNICResources did not receive a GET."+
			" Received: %s", r.Method)
		returnCode := types.UnsupportedVerb
		service.setResponse(w, returnCode, cns.GetNICResourcesResponse{
			Response: cns.Response{ReturnCode: returnCode, Message: returnMessage},
		})
		return
	}

	// The NodeInfo client and node name are required to serve this endpoint.
	// They may be unset if AttachNodeInfoClient was never called (e.g. the
	// feature is not configured), so guard against a nil client / empty name
	// instead of panicking.
	if service.nodeinfoClient == nil || service.nodeName == "" {
		l.Error("getNICResources not configured", zap.Bool("nodeinfoClientSet", service.nodeinfoClient != nil), zap.Bool("nodeNameSet", service.nodeName != ""))
		resp := cns.GetNICResourcesResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    "getNICResources: NodeInfo client or node name is not configured",
			},
		}
		respondJSON(w, http.StatusInternalServerError, resp)
		return
	}

	// Step 1: Get list of NICs (MACs) from NodeInfo CRD.
	nodeInfo, err := service.nodeinfoClient.Get(ctx, service.nodeName)
	if err != nil {
		l.Error("failed to fetch NodeInfo CRD", zap.String("node", service.nodeName), zap.Error(err))
		resp := cns.GetNICResourcesResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    errors.Wrap(err, "failed to get NodeInfo CRD").Error(),
			},
		}
		respondJSON(w, http.StatusInternalServerError, resp)
		return
	}
	vmUniqueID := nodeInfo.Spec.VMUniqueID

	// Initialize map keyed by normalized MAC for every device in NodeInfo.
	nicByMAC := make(map[string]*cns.NICResource, len(nodeInfo.Status.DeviceInfos))
	for _, device := range nodeInfo.Status.DeviceInfos {
		// DRA NIC sharing is only supported for Vnet NICs, so skip any other device
		// type (e.g. InfiniBand) — those NICs are not advertised as DRA resources.
		if device.DeviceType != v1alpha1.DeviceTypeVnetNIC {
			continue
		}
		normalizedMAC, parseErr := net.ParseMAC(device.MacAddress)
		if parseErr != nil {
			// Skip this NIC, but surface the failure as an alertable metric in addition
			// to the log so a bad MAC in NodeInfo isn't silently ignored. The MAC value
			// stays in the log — it's too high-cardinality to be a metric label.
			nicResourceMACParseErrors.Inc()
			l.Warn("failed to parse MAC from NodeInfo", zap.String("mac", device.MacAddress), zap.Error(parseErr))
			continue
		}
		key := normalizedMAC.String()
		nicByMAC[key] = &cns.NICResource{
			MacAddress: device.MacAddress,
			VMUniqueID: vmUniqueID,
			Capacity:   "1", // default capacity is 1
		}
	}

	// Step 2: Populate InterfaceName from host network interfaces.
	ifaces, ifErr := net.Interfaces()
	if ifErr != nil {
		l.Warn("failed to list host network interfaces", zap.Error(ifErr))
	} else {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) == 0 {
				continue
			}
			key := iface.HardwareAddr.String() // already normalized
			if res, ok := nicByMAC[key]; ok {
				// On Azure VMs with Accelerated Networking a NIC surfaces as both a
				// synthetic netvsc interface (e.g. eth1) and an SR-IOV VF interface
				// (e.g. enP56082s2). Only "eth"-prefixed synthetic names are stable
				// across VF hot-swap, so consider only those; if the MAC has no such
				// interface, InterfaceName is left unset.
				if strings.HasPrefix(iface.Name, "eth") {
					res.InterfaceName = iface.Name
				}
			}
		}
	}

	// Step 3: Fetch per-NIC network/subnet/capacity from NICNetworkConfig (shared
	// prefix-on-NIC NICs).
	var nicResourceNetworkInfoByMAC map[string]*cns.NICResourceNetworkInfo
	if service.nicncClient != nil {
		nicResourceNetworkInfoByMAC, err = service.nicncClient.GetNICResourceNetworkInfoFromNICNC(ctx)
		if err != nil {
			l.Warn("failed to fetch NICNetworkConfig data", zap.Error(err))
		}
	}

	// Step 4: Fetch per-NIC data from MTPNC. Dedicated NICs do not have
	// NICNetworkConfig and are served from MTPNC.
	var mtpncResourceNetworkInfoByMAC map[string]*cns.NICResourceNetworkInfo
	if service.mtpncClient != nil {
		mtpncResourceNetworkInfoByMAC, err = service.mtpncClient.GetNICResourceNetworkInfoFromMTPNC(ctx)
		if err != nil {
			l.Warn("failed to fetch MTPNC data", zap.Error(err))
		}
	}

	// Step 5: Enrich each NIC discovered from NodeInfo, preferring NICNetworkConfig
	// and falling back to MTPNC for MACs it doesn't cover. The map key is the
	// canonical MAC used for lookups.
	for mac, res := range nicByMAC {
		enrichNICResource(res, mac, nicResourceNetworkInfoByMAC, mtpncResourceNetworkInfoByMAC)
	}

	// Convert map to slice for the response.
	nicResources := make([]cns.NICResource, 0, len(nicByMAC))
	for _, res := range nicByMAC {
		nicResources = append(nicResources, *res)
	}

	resp := cns.GetNICResourcesResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
		},
		NICResources: nicResources,
	}
	respondJSON(w, http.StatusOK, resp)
}

// enrichNICResource populates res.NetworkID/SubnetGUID/SubnetName/Capacity for a NIC, looked up by canonical MAC.
// NICNetworkConfig is preferred (shared prefix-on-NIC NICs); MTPNC is the fallback for dedicated NICs, which has no NICNetworkConfig.
// MTPNC never overrides NICNetworkConfig, and a NIC found in neither keeps capacity as 1 since it is available for scheduling for either type of pods.
func enrichNICResource(res *cns.NICResource, mac string, nicResourceNetworkInfoByMAC, mtpncResourceNetworkInfoByMAC map[string]*cns.NICResourceNetworkInfo) {
	info := nicResourceNetworkInfoByMAC[mac]
	if info == nil {
		info = mtpncResourceNetworkInfoByMAC[mac]
	}
	if info == nil {
		return
	}
	res.NetworkID = info.NetworkID
	res.SubnetGUID = info.SubnetGUID
	res.SubnetName = info.SubnetName
	res.Capacity = strconv.Itoa(info.Capacity)
}
