// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"
	"net/http"

	"github.com/Azure/azure-container-networking/cnm"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Plugin name.
	name = "azure-vnet"

	// Plugin capabilities.
	scope = "local"

	// Prefix for container network interface names.
	containerInterfacePrefix = "eth"
	returnCode               = 0
	returnStr                = "Success"
)

// NetPlugin represents a CNM (libnetwork) network plugin.
type netPlugin struct {
	*cnm.Plugin
	scope string
	nm    network.NetworkManager
}

type NetPlugin interface {
	common.PluginApi
}

// NewPlugin creates a new NetPlugin object.
func NewPlugin(config *common.PluginConfig) (NetPlugin, error) {
	// Setup base plugin.
	plugin, err := cnm.NewPlugin(name, config.Version, endpointType)
	if err != nil {
		return nil, err
	}

	// Setup network manager.
	nm, err := network.NewNetworkManager()
	if err != nil {
		return nil, err
	}

	config.NetApi = nm

	return &netPlugin{
		Plugin: plugin,
		scope:  scope,
		nm:     nm,
	}, nil
}

// Start starts the plugin.
func (plugin *netPlugin) Start(config *common.PluginConfig) error {
	// Initialize base plugin.
	err := plugin.Initialize(config)
	if err != nil {
		log.Printf("[net] Failed to initialize base plugin, err:%v.", err)
		return err
	}

	// Initialize network manager. rehyrdration required on reboot for cnm plugin
	err = plugin.nm.Initialize(config, true)
	if err != nil {
		log.Printf("[net] Failed to initialize network manager, err:%v.", err)
		return err
	}

	// Add protocol handlers.
	listener := plugin.Listener
	listener.AddEndpoint(plugin.EndpointType)
	listener.AddHandler(getCapabilitiesPath, plugin.getCapabilities)
	listener.AddHandler(createNetworkPath, plugin.createNetwork)
	listener.AddHandler(deleteNetworkPath, plugin.deleteNetwork)
	listener.AddHandler(createEndpointPath, plugin.createEndpoint)
	listener.AddHandler(deleteEndpointPath, plugin.deleteEndpoint)
	listener.AddHandler(joinPath, plugin.join)
	listener.AddHandler(leavePath, plugin.leave)
	listener.AddHandler(endpointOperInfoPath, plugin.endpointOperInfo)

	// Plugin is ready to be discovered.
	err = plugin.EnableDiscovery()
	if err != nil {
		log.Printf("[net] Failed to enable discovery: %v.", err)
		return err
	}

	log.Printf("[net] Plugin started.")

	return nil
}

// Stop stops the plugin.
func (plugin *netPlugin) Stop() {
	plugin.DisableDiscovery()
	plugin.nm.Uninitialize()
	plugin.Uninitialize()
	log.Printf("[net] Plugin stopped.")
}

//
// Libnetwork remote network API implementation
// https://github.com/docker/libnetwork/blob/master/docs/remote.md
//

// Handles GetCapabilities requests.
func (plugin *netPlugin) getCapabilities(w http.ResponseWriter, r *http.Request) {
	var req getCapabilitiesRequest

	log.Request(plugin.Name, &req, nil)

	resp := getCapabilitiesResponse{Scope: plugin.scope}
	err := plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles CreateNetwork requests.
func (plugin *netPlugin) createNetwork(w http.ResponseWriter, r *http.Request) {
	var req createNetworkRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	nwInfo := network.NetworkInfo{
		Id:      req.NetworkID,
		Options: req.Options,
	}

	// Parse network options.
	options := plugin.ParseOptions(req.Options)
	if options != nil {
		nwInfo.Mode, _ = options[modeOption].(string)
	}

	// Populate subnets.
	for _, data := range [][]ipamData{req.IPv4Data, req.IPv6Data} {
		for _, ipamData := range data {
			_, prefix, err := net.ParseCIDR(ipamData.Pool)
			if err != nil {
				continue
			}

			subnet := network.SubnetInfo{
				Family:  platform.GetAddressFamily(&prefix.IP),
				Prefix:  *prefix,
				Gateway: platform.ConvertStringToIPAddress(ipamData.Gateway),
			}

			nwInfo.Subnets = append(nwInfo.Subnets, subnet)
		}
	}

	err = plugin.nm.CreateNetwork(&nwInfo)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := createNetworkResponse{}
	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles DeleteNetwork requests.
func (plugin *netPlugin) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	var req deleteNetworkRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	err = plugin.nm.DeleteNetwork(req.NetworkID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := deleteNetworkResponse{}
	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles CreateEndpoint requests.
func (plugin *netPlugin) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var req createEndpointRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	var ipv4Address *net.IPNet
	if req.Interface.Address != "" {
		var ip net.IP
		ip, ipv4Address, err = net.ParseCIDR(req.Interface.Address)
		if err != nil {
			plugin.SendErrorResponse(w, err)
			return
		}
		ipv4Address.IP = ip
	}

	epInfo := network.EndpointInfo{
		Id:              req.EndpointID,
		IPAddresses:     []net.IPNet{*ipv4Address},
		SkipHotAttachEp: true, // Skip hot attach endpoint as it's done in Join
	}

	epInfo.Data = make(map[string]interface{})

	err = plugin.nm.CreateEndpoint(req.NetworkID, &epInfo)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := createEndpointResponse{}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles DeleteEndpoint requests.
func (plugin *netPlugin) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	var req deleteEndpointRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	err = plugin.nm.DeleteEndpoint(req.NetworkID, req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := deleteEndpointResponse{}
	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles Join requests.
func (plugin *netPlugin) join(w http.ResponseWriter, r *http.Request) {
	var req joinRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	ep, err := plugin.nm.AttachEndpoint(req.NetworkID, req.EndpointID, req.SandboxKey)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	ifname := interfaceName{
		SrcName:   ep.IfName,
		DstPrefix: containerInterfacePrefix,
	}

	resp := joinResponse{
		InterfaceName: ifname,
		Gateway:       ep.Gateways[0].String(),
	}

	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles Leave requests.
func (plugin *netPlugin) leave(w http.ResponseWriter, r *http.Request) {
	var req leaveRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	err = plugin.nm.DetachEndpoint(req.NetworkID, req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := leaveResponse{}
	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}

// Handles EndpointOperInfo requests.
func (plugin *netPlugin) endpointOperInfo(w http.ResponseWriter, r *http.Request) {
	var req endpointOperInfoRequest

	// Decode request.
	err := plugin.Listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	epInfo, err := plugin.nm.GetEndpointInfo(req.NetworkID, req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := endpointOperInfoResponse{Value: epInfo.Data}
	err = plugin.Listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, returnCode, returnStr, err)
}
