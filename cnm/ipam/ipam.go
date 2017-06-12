// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"net/http"

	"github.com/Azure/azure-container-networking/cnm"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/log"
)

const (
	// Plugin name.
	name = "azure-vnet-ipam"

	// Plugin capabilities reported to libnetwork.
	requiresMACAddress    = false
	requiresRequestReplay = false
)

// IpamPlugin represents a CNM (libnetwork) IPAM plugin.
type ipamPlugin struct {
	*cnm.Plugin
	am ipam.AddressManager
}

type IpamPlugin interface {
	common.PluginApi
}

// NewPlugin creates a new IpamPlugin object.
func NewPlugin(config *common.PluginConfig) (IpamPlugin, error) {
	// Setup base plugin.
	plugin, err := cnm.NewPlugin(name, config.Version, EndpointType)
	if err != nil {
		return nil, err
	}

	// Setup address manager.
	am, err := ipam.NewAddressManager()
	if err != nil {
		return nil, err
	}

	config.IpamApi = am

	return &ipamPlugin{
		Plugin: plugin,
		am:     am,
	}, nil
}

// Start starts the plugin.
func (plugin *ipamPlugin) Start(config *common.PluginConfig) error {
	// Initialize base plugin.
	err := plugin.Initialize(config)
	if err != nil {
		log.Printf("[ipam] Failed to initialize base plugin, err:%v.", err)
		return err
	}

	// Initialize address manager.
	err = plugin.am.Initialize(config, plugin.Options)
	if err != nil {
		log.Printf("[ipam] Failed to initialize address manager, err:%v.", err)
		return err
	}

	// Add protocol handlers.
	listener := plugin.Listener
	listener.AddEndpoint(plugin.EndpointType)
	listener.AddHandler(getCapabilitiesPath, plugin.getCapabilities)
	listener.AddHandler(getAddressSpacesPath, plugin.getDefaultAddressSpaces)
	listener.AddHandler(requestPoolPath, plugin.requestPool)
	listener.AddHandler(releasePoolPath, plugin.releasePool)
	listener.AddHandler(getPoolInfoPath, plugin.getPoolInfo)
	listener.AddHandler(requestAddressPath, plugin.requestAddress)
	listener.AddHandler(releaseAddressPath, plugin.releaseAddress)

	// Plugin is ready to be discovered.
	err = plugin.EnableDiscovery()
	if err != nil {
		log.Printf("[ipam] Failed to enable discovery: %v.", err)
		return err
	}

	log.Printf("[ipam] Plugin started.")

	return nil
}

// Stop stops the plugin.
func (plugin *ipamPlugin) Stop() {
	plugin.DisableDiscovery()
	plugin.am.Uninitialize()
	plugin.Uninitialize()
	log.Printf("[ipam] Plugin stopped.")
}

//
// Libnetwork remote IPAM API implementation
// https://github.com/docker/libnetwork/blob/master/docs/ipam.md
//

// Handles GetCapabilities requests.
func (plugin *ipamPlugin) getCapabilities(w http.ResponseWriter, r *http.Request) {
	var req getCapabilitiesRequest

	log.Request(plugin.Name, &req, nil)

	resp := getCapabilitiesResponse{
		RequiresMACAddress:    requiresMACAddress,
		RequiresRequestReplay: requiresRequestReplay,
	}

	err := plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles GetDefaultAddressSpaces requests.
func (plugin *ipamPlugin) getDefaultAddressSpaces(w http.ResponseWriter, r *http.Request) {
	var req getDefaultAddressSpacesRequest
	var resp getDefaultAddressSpacesResponse

	log.Request(plugin.Name, &req, nil)

	localId, globalId := plugin.am.GetDefaultAddressSpaces()

	resp.LocalDefaultAddressSpace = localId
	resp.GlobalDefaultAddressSpace = globalId

	err := plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles RequestPool requests.
func (plugin *ipamPlugin) requestPool(w http.ResponseWriter, r *http.Request) {
	var req requestPoolRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	poolId, subnet, err := plugin.am.RequestPool(req.AddressSpace, req.Pool, req.SubPool, req.Options, req.V6)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	data := make(map[string]string)
	poolId = ipam.NewAddressPoolId(req.AddressSpace, poolId, "").String()
	resp := requestPoolResponse{PoolID: poolId, Pool: subnet, Data: data}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles ReleasePool requests.
func (plugin *ipamPlugin) releasePool(w http.ResponseWriter, r *http.Request) {
	var req releasePoolRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	poolId, err := ipam.NewAddressPoolIdFromString(req.PoolID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	err = plugin.am.ReleasePool(poolId.AsId, poolId.Subnet)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := releasePoolResponse{}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles GetPoolInfo requests.
func (plugin *ipamPlugin) getPoolInfo(w http.ResponseWriter, r *http.Request) {
	var req getPoolInfoRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	poolId, err := ipam.NewAddressPoolIdFromString(req.PoolID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	apInfo, err := plugin.am.GetPoolInfo(poolId.AsId, poolId.Subnet)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := getPoolInfoResponse{
		Capacity:  apInfo.Capacity,
		Available: apInfo.Available,
	}

	for _, addr := range apInfo.UnhealthyAddrs {
		resp.UnhealthyAddresses = append(resp.UnhealthyAddresses, addr.String())
	}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles RequestAddress requests.
func (plugin *ipamPlugin) requestAddress(w http.ResponseWriter, r *http.Request) {
	var req requestAddressRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	poolId, err := ipam.NewAddressPoolIdFromString(req.PoolID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Convert libnetwork IPAM options to core IPAM options.
	options := make(map[string]string)
	if req.Options[OptAddressType] == OptAddressTypeGateway {
		options[ipam.OptAddressType] = ipam.OptAddressTypeGateway
	}

	options[ipam.OptAddressID] = req.Options[ipam.OptAddressID]

	addr, err := plugin.am.RequestAddress(poolId.AsId, poolId.Subnet, req.Address, options)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	data := make(map[string]string)
	resp := requestAddressResponse{Address: addr, Data: data}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles ReleaseAddress requests.
func (plugin *ipamPlugin) releaseAddress(w http.ResponseWriter, r *http.Request) {
	var req releaseAddressRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	poolId, err := ipam.NewAddressPoolIdFromString(req.PoolID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	err = plugin.am.ReleaseAddress(poolId.AsId, poolId.Subnet, req.Address, req.Options)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := releaseAddressResponse{}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}
