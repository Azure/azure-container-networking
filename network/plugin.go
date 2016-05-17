// Copyright Microsoft Corp.
// All rights reserved.

package network

import (
	"net/http"
	"sync"

	"github.com/Azure/Aqua/common"
	"github.com/Azure/Aqua/log"
)

// Plugin capabilities.
const (
	scope = "local"
)

// NetPlugin object and its interface
type netPlugin struct {
	*common.Plugin
	scope    string
	listener *common.Listener
	nm       *networkManager
	sync.Mutex
}

type NetPlugin interface {
	Start(chan error) error
	Stop()
}

// Creates a new NetPlugin object.
func NewPlugin(name string, version string) (NetPlugin, error) {
	// Setup base plugin.
	plugin, err := common.NewPlugin(name, version, endpointType)
	if err != nil {
		return nil, err
	}

	// Setup network manager.
	nm, err := newNetworkManager()
	if err != nil {
		return nil, err
	}

	return &netPlugin{
		Plugin: plugin,
		scope:  scope,
		nm:     nm,
	}, nil
}

// Starts the plugin.
func (plugin *netPlugin) Start(errChan chan error) error {
	err := plugin.Initialize(errChan)
	if err != nil {
		log.Printf("%s: Failed to start: %v", plugin.Name, err)
		return err
	}

	// Add protocol handlers.
	listener := plugin.Listener
	listener.AddHandler(getCapabilitiesPath, plugin.getCapabilities)
	listener.AddHandler(createNetworkPath, plugin.createNetwork)
	listener.AddHandler(deleteNetworkPath, plugin.deleteNetwork)
	listener.AddHandler(createEndpointPath, plugin.createEndpoint)
	listener.AddHandler(deleteEndpointPath, plugin.deleteEndpoint)
	listener.AddHandler(joinPath, plugin.join)
	listener.AddHandler(leavePath, plugin.leave)
	listener.AddHandler(endpointOperInfoPath, plugin.endpointOperInfo)

	log.Printf("%s: Plugin started.", plugin.Name)

	return nil
}

// Stops the plugin.
func (plugin *netPlugin) Stop() {
	plugin.Uninitialize()
	log.Printf("%s: Plugin stopped.\n", plugin.Name)
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
	err := plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles CreateNetwork requests.
func (plugin *netPlugin) createNetwork(w http.ResponseWriter, r *http.Request) {
	var req createNetworkRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	_, err = plugin.nm.newNetwork(req.NetworkID, req.Options, req.IPv4Data, req.IPv6Data)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := createNetworkResponse{}
	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles DeleteNetwork requests.
func (plugin *netPlugin) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	var req deleteNetworkRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	err = plugin.nm.deleteNetwork(req.NetworkID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := deleteNetworkResponse{}
	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles CreateEndpoint requests.
func (plugin *netPlugin) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var req createEndpointRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	var ipaddressToAttach string

	for key, value := range req.Options {
		if key == "com.docker.network.endpoint.ipaddresstoattach" {
			ipaddressToAttach = value.(string)
			log.Printf("Received request to attach following ipaddress: %s", value)
		}
	}

	if req.Interface != nil {
		ipaddressToAttach = req.Interface.Address
	}

	plugin.Lock()

	nw, err := plugin.getNetwork(req.NetworkID)
	if err != nil {
		plugin.Unlock()
		plugin.SendErrorResponse(w, err)
		return
	}

	_, err = nw.newEndpoint(req.EndpointID, ipaddressToAttach)
	if err != nil {
		plugin.Unlock()
		plugin.SendErrorResponse(w, err)
		return
	}

	plugin.Unlock()

	// Encode response.
	resp := createEndpointResponse{
		Interface: nil,
	}

	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles Join requests.
func (plugin *netPlugin) join(w http.ResponseWriter, r *http.Request) {
	var req joinRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	plugin.Lock()
	defer plugin.Unlock()

	nw, err := plugin.getNetwork(req.NetworkID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	ep, err := nw.getEndpoint(req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	err = ep.join(req.SandboxKey)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	ifname := interfaceName{
		SrcName:   ep.coreep.SrcName,
		DstPrefix: ep.coreep.DstPrefix,
	}

	resp := joinResponse{
		InterfaceName: ifname,
		Gateway:       ep.coreep.GatewayIPv4.String(),
	}

	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles DeleteEndpoint requests.
func (plugin *netPlugin) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	var req deleteEndpointRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	plugin.Lock()
	defer plugin.Unlock()

	nw, err := plugin.getNetwork(req.NetworkID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	err = nw.deleteEndpoint(req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := deleteEndpointResponse{}
	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles Leave requests.
func (plugin *netPlugin) leave(w http.ResponseWriter, r *http.Request) {
	var req leaveRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	// Process request.
	plugin.Lock()
	defer plugin.Unlock()

	nw, err := plugin.getNetwork(req.NetworkID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	ep, err := nw.getEndpoint(req.EndpointID)
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	err = ep.leave()
	if err != nil {
		plugin.SendErrorResponse(w, err)
		return
	}

	// Encode response.
	resp := leaveResponse{}
	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}

// Handles EndpointOperInfo requests.
func (plugin *netPlugin) endpointOperInfo(w http.ResponseWriter, r *http.Request) {
	var req endpointOperInfoRequest

	// Decode request.
	err := plugin.listener.Decode(w, r, &req)
	log.Request(plugin.Name, &req, err)
	if err != nil {
		return
	}

	value := make(map[string]interface{})
	//value["com.docker.network.endpoint.macaddress"] = macAddress
	//value["MacAddress"] = macAddress

	// Encode response.
	resp := endpointOperInfoResponse{Value: value}
	err = plugin.listener.Encode(w, &resp)

	log.Response(plugin.Name, &resp, err)
}
