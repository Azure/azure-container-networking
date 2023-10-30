// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

const (
	InfraVnet = 0
)

var logger = log.CNILogger.With(zap.String("component", "net"))

type AzureHNSEndpoint struct{}

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
	NATInfo                  []policy.NATInfo
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
	Table    int
}

type apipaClient interface {
	DeleteHostNCApipaEndpoint(ctx context.Context, networkContainerID string) error
	CreateHostNCApipaEndpoint(ctx context.Context, networkContainerID string) (string, error)
}

func (epInfo *EndpointInfo) PrettyString() string {
	return fmt.Sprintf("Id:%s ContainerID:%s NetNsPath:%s IfName:%s IfIndex:%d MacAddr:%s IPAddrs:%v Gateways:%v Data:%+v",
		epInfo.Id, epInfo.ContainerID, epInfo.NetNsPath, epInfo.IfName, epInfo.IfIndex, epInfo.MacAddress.String(), epInfo.IPAddresses,
		epInfo.Gateways, epInfo.Data)
}

// NewEndpoint creates a new endpoint in the network.
func (nw *network) newEndpoint(
	apipaCli apipaClient,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
	netioCli netio.NetIOInterface,
	epInfo *EndpointInfo,
) (*endpoint, error) {
	var ep *endpoint
	var err error

	defer func() {
		if err != nil {
			logger.Error("Failed to create endpoint with err", zap.String("id", epInfo.Id), zap.Error(err))
		}
	}()

	// Call the platform implementation.
	// Pass nil for epClient and will be initialized in newendpointImpl
	ep, err = nw.newEndpointImpl(apipaCli, nl, plc, netioCli, nil, epInfo)
	if err != nil {
		return nil, err
	}

	nw.Endpoints[epInfo.Id] = ep
	logger.Info("Created endpoint. Num of endpoints", zap.Any("ep", ep), zap.Int("numEndpoints", len(nw.Endpoints)))
	return ep, nil
}

// DeleteEndpoint deletes an existing endpoint from the network.
func (nw *network) deleteEndpoint(nl netlink.NetlinkInterface, plc platform.ExecClient, endpointID string) error {
	var err error

	logger.Info("Deleting endpoint from network", zap.String("endpointID", endpointID), zap.String("id", nw.Id))
	defer func() {
		if err != nil {
			logger.Error("Failed to delete endpoint with", zap.String("endpointID", endpointID), zap.Error(err))
		}
	}()

	// Look up the endpoint.
	ep, err := nw.getEndpoint(endpointID)
	if err != nil {
		logger.Error("Endpoint not found. Not Returning error", zap.String("endpointID", endpointID), zap.Error(err))
		return nil
	}

	// Call the platform implementation.
	// Pass nil for epClient and will be initialized in deleteEndpointImpl
	err = nw.deleteEndpointImpl(nl, plc, nil, ep)
	if err != nil {
		return err
	}

	// Remove the endpoint object.
	delete(nw.Endpoints, endpointID)
	logger.Info("Deleted endpoint. Num of endpoints", zap.Any("ep", ep), zap.Int("numEndpoints", len(nw.Endpoints)))
	return nil
}

// GetEndpoint returns the endpoint with the given ID.
func (nw *network) getEndpoint(endpointId string) (*endpoint, error) {
	ep := nw.Endpoints[endpointId]

	if ep == nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

// GetEndpointByPOD returns the endpoint with the given ID.
func (nw *network) getEndpointByPOD(podName string, podNameSpace string, doExactMatchForPodName bool) (*endpoint, error) {
	logger.Info("Trying to retrieve endpoint for pod name in namespace", zap.String("podName", podName), zap.String("podNameSpace", podNameSpace))

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
		return source == actualValue
	} else {
		// If exact match flag is disabled we just check if the existing podname field for an endpoint
		// starts with passed podname string.
		return actualValue == GetPodNameWithoutSuffix(source)
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

	logger.Info("Attached endpoint to sandbox", zap.String("id", ep.Id), zap.String("sandboxKey", sandboxKey))

	return nil
}

// Detach detaches an endpoint from its sandbox.
func (ep *endpoint) detach() error {
	if ep.SandboxKey == "" {
		return errEndpointNotInUse
	}

	logger.Info("Detached endpoint from sandbox", zap.String("id", ep.Id), zap.String("sandboxKey", ep.SandboxKey))

	ep.SandboxKey = ""

	return nil
}

// updateEndpoint updates an existing endpoint in the network.
func (nm *networkManager) updateEndpoint(nw *network, exsitingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) error {
	var err error

	logger.Info("Updating existing endpoint in network to target", zap.Any("exsitingEpInfo", exsitingEpInfo),
		zap.String("id", nw.Id), zap.Any("targetEpInfo", targetEpInfo))
	defer func() {
		if err != nil {
			logger.Error("Failed to update endpoint with err", zap.String("id", exsitingEpInfo.Id), zap.Error(err))
		}
	}()

	logger.Info("Trying to retrieve endpoint id", zap.String("id", exsitingEpInfo.Id))

	ep := nw.Endpoints[exsitingEpInfo.Id]
	if ep == nil {
		return errEndpointNotFound
	}

	logger.Info("Retrieved endpoint to update", zap.Any("ep", ep))

	// Call the platform implementation.
	ep, err = nm.updateEndpointImpl(nw, exsitingEpInfo, targetEpInfo)
	if err != nil {
		return err
	}

	// Update routes for existing endpoint
	nw.Endpoints[exsitingEpInfo.Id].Routes = ep.Routes

	return nil
}

func GetPodNameWithoutSuffix(podName string) string {
	nameSplit := strings.Split(podName, "-")
	logger.Info("namesplit", zap.Any("nameSplit", nameSplit))
	if len(nameSplit) > 2 {
		nameSplit = nameSplit[:len(nameSplit)-2]
	} else {
		return podName
	}

	logger.Info("Pod name after splitting based on", zap.Any("nameSplit", nameSplit))
	return strings.Join(nameSplit, "-")
}
