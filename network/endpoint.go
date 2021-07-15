// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
)

const (
	InfraVnet = 0
)

// Endpoint represents a container network interface.
type endpoint struct {
	Id                       string
	HnsId                    string `json:",omitempty"`
	SandboxKey               string
	IfName                   string
	HostIfName               string
	MacAddress               net.HardwareAddr
	InfraVnetIP              net.IPNet
	LocalIP                  string
	IPAddresses              []net.IPNet
	Gateways                 []net.IP
	DNS                      DNSInfo
	Routes                   []RouteInfo
	VlanID                   int
	EnableSnatOnHost         bool
	EnableInfraVnet          bool
	EnableMultitenancy       bool
	AllowInboundFromHostToNC bool
	AllowInboundFromNCToHost bool
	NetworkContainerID       string
	NetworkNameSpace         string `json:",omitempty"`
	ContainerID              string
	PODName                  string `json:",omitempty"`
	PODNameSpace             string `json:",omitempty"`
	InfraVnetAddressSpace    string `json:",omitempty"`
	NetNs                    string `json:",omitempty"`
}

// EndpointInfo contains read-only information about an endpoint.
type EndpointInfo struct {
	Id                       string
	ContainerID              string
	NetNsPath                string
	IfName                   string
	SandboxKey               string
	IfIndex                  int
	MacAddress               net.HardwareAddr
	DNS                      DNSInfo
	IPAddresses              []net.IPNet
	IPsToRouteViaHost        []string
	InfraVnetIP              net.IPNet
	Routes                   []RouteInfo
	Policies                 []policy.Policy
	Gateways                 []net.IP
	EnableSnatOnHost         bool
	EnableInfraVnet          bool
	EnableMultiTenancy       bool
	EnableSnatForDns         bool
	AllowInboundFromHostToNC bool
	AllowInboundFromNCToHost bool
	NetworkContainerID       string
	PODName                  string
	PODNameSpace             string
	Data                     map[string]interface{}
	InfraVnetAddressSpace    string
	SkipHotAttachEp          bool
	IPV6Mode                 string
	VnetCidrs                string
	ServiceCidrs             string
}

// RouteInfo contains information about an IP route.
type RouteInfo struct {
	Dst      net.IPNet
	Src      net.IP
	Gw       net.IP
	Protocol int
	DevName  string
	Scope    int
	Priority int
}

// NewEndpoint creates a new endpoint in the network.
func (nw *network) newEndpoint(epInfo *EndpointInfo) (*endpoint, error) {
	var ep *endpoint
	var err error

	log.Printf("[net] Creating endpoint %+v in network %v.", epInfo, nw.Id)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to create endpoint %v, err:%v.", epInfo.Id, err)
		}
	}()

	// Call the platform implementation.
	ep, err = nw.newEndpointImpl(epInfo)
	if err != nil {
		return nil, err
	}

	nw.Endpoints[epInfo.Id] = ep
	log.Printf("[net] Created endpoint %+v.", ep)

	return ep, nil
}

// DeleteEndpoint deletes an existing endpoint from the network.
func (nw *network) deleteEndpoint(endpointId string) error {
	var err error

	log.Printf("[net] Deleting endpoint %v from network %v.", endpointId, nw.Id)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to delete endpoint %v, err:%v.", endpointId, err)
		}
	}()

	// Look up the endpoint.
	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		log.Printf("[net] Endpoint %v not found. Not Returning error", endpointId)
		return nil
	}

	// Call the platform implementation.
	err = nw.deleteEndpointImpl(ep)
	if err != nil {
		return err
	}

	// Remove the endpoint object.
	delete(nw.Endpoints, endpointId)

	log.Printf("[net] Deleted endpoint %+v.", ep)

	return nil
}

// GetEndpoint returns the endpoint with the given ID.
func (nw *network) getEndpoint(endpointId string) (*endpoint, error) {
	log.Printf("Trying to retrieve endpoint id %v", endpointId)

	ep := nw.Endpoints[endpointId]

	if ep == nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

// GetEndpointByPOD returns the endpoint with the given ID.
func (nw *network) getEndpointByPOD(podName string, podNameSpace string, doExactMatchForPodName bool) (*endpoint, error) {
	log.Printf("Trying to retrieve endpoint for pod name: %v in namespace: %v", podName, podNameSpace)

	var ep *endpoint

	for _, endpoint := range nw.Endpoints {
		if podNameMatches(endpoint.PODName, podName, doExactMatchForPodName) && endpoint.PODNameSpace == podNameSpace {
			if ep == nil {
				ep = endpoint
			} else {
				return nil, errMultipleEndpointsFound
			}
		}
	}

	if ep == nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

func podNameMatches(source string, actualValue string, doExactMatch bool) bool {
	if doExactMatch {
		return (source == actualValue)
	} else {
		// If exact match flag is disabled we just check if the existing podname field for an endpoint
		// starts with passed podname string.
		return (actualValue == GetPodNameWithoutSuffix(source))
	}
}

//
// Endpoint
//

// GetInfo returns information about the endpoint.
func (ep *endpoint) getInfo() *EndpointInfo {
	info := &EndpointInfo{
		Id:                       ep.Id,
		IPAddresses:              ep.IPAddresses,
		InfraVnetIP:              ep.InfraVnetIP,
		Data:                     make(map[string]interface{}),
		MacAddress:               ep.MacAddress,
		SandboxKey:               ep.SandboxKey,
		IfIndex:                  0, // Azure CNI supports only one interface
		DNS:                      ep.DNS,
		EnableSnatOnHost:         ep.EnableSnatOnHost,
		EnableInfraVnet:          ep.EnableInfraVnet,
		EnableMultiTenancy:       ep.EnableMultitenancy,
		AllowInboundFromHostToNC: ep.AllowInboundFromHostToNC,
		AllowInboundFromNCToHost: ep.AllowInboundFromNCToHost,
		IfName:                   ep.IfName,
		ContainerID:              ep.ContainerID,
		NetNsPath:                ep.NetworkNameSpace,
		PODName:                  ep.PODName,
		PODNameSpace:             ep.PODNameSpace,
		NetworkContainerID:       ep.NetworkContainerID,
	}

	info.Routes = append(info.Routes, ep.Routes...)

	info.Gateways = append(info.Gateways, ep.Gateways...)

	// Call the platform implementation.
	ep.getInfoImpl(info)

	return info
}

// Attach attaches an endpoint to a sandbox.
func (ep *endpoint) attach(sandboxKey string) error {
	if ep.SandboxKey != "" {
		return errEndpointInUse
	}

	ep.SandboxKey = sandboxKey

	log.Printf("[net] Attached endpoint %v to sandbox %v.", ep.Id, sandboxKey)

	return nil
}

// Detach detaches an endpoint from its sandbox.
func (ep *endpoint) detach() error {
	if ep.SandboxKey == "" {
		return errEndpointNotInUse
	}

	log.Printf("[net] Detached endpoint %v from sandbox %v.", ep.Id, ep.SandboxKey)

	ep.SandboxKey = ""

	return nil
}

// updateEndpoint updates an existing endpoint in the network.
func (nw *network) updateEndpoint(exsitingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) (*endpoint, error) {
	var err error

	log.Printf("[net] Updating existing endpoint [%+v] in network %v to target [%+v].", exsitingEpInfo, nw.Id, targetEpInfo)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to update endpoint %v, err:%v.", exsitingEpInfo.Id, err)
		}
	}()

	log.Printf("Trying to retrieve endpoint id %v", exsitingEpInfo.Id)

	ep := nw.Endpoints[exsitingEpInfo.Id]
	if ep == nil {
		return nil, errEndpointNotFound
	}

	log.Printf("[net] Retrieved endpoint to update %+v.", ep)

	// Call the platform implementation.
	ep, err = nw.updateEndpointImpl(exsitingEpInfo, targetEpInfo)
	if err != nil {
		return nil, err
	}

	// Update routes for existing endpoint
	nw.Endpoints[exsitingEpInfo.Id].Routes = ep.Routes

	return ep, nil
}

func GetPodNameWithoutSuffix(podName string) string {
	nameSplit := strings.Split(podName, "-")
	log.Printf("namesplit %v", nameSplit)
	if len(nameSplit) > 2 {
		nameSplit = nameSplit[:len(nameSplit)-2]
	} else {
		return podName
	}

	log.Printf("Pod name after splitting based on - : %v", nameSplit)
	return strings.Join(nameSplit, "-")
}
