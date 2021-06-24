// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/cns/common"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// Container Network Service remote API Contract
const (
	SetEnvironmentPath            = "/network/environment"
	CreateNetworkPath             = "/network/create"
	DeleteNetworkPath             = "/network/delete"
	CreateHnsNetworkPath          = "/network/hns/create"
	DeleteHnsNetworkPath          = "/network/hns/delete"
	ReserveIPAddressPath          = "/network/ip/reserve"
	ReleaseIPAddressPath          = "/network/ip/release"
	GetHostLocalIPPath            = "/network/ip/hostlocal"
	GetIPAddressUtilizationPath   = "/network/ip/utilization"
	GetUnhealthyIPAddressesPath   = "/network/ipaddresses/unhealthy"
	GetHealthReportPath           = "/network/health"
	NumberOfCPUCoresPath          = "/hostcpucores"
	CreateHostNCApipaEndpointPath = "/network/createhostncapipaendpoint"
	DeleteHostNCApipaEndpointPath = "/network/deletehostncapipaendpoint"
	NmAgentSupportedApisPath      = "/network/nmagentsupportedapis"
	V1Prefix                      = "/v0.1"
	V2Prefix                      = "/v0.2"
)

// HTTPService describes the min API interface that every service should have.
type HTTPService interface {
	common.ServiceAPI
	SendNCSnapShotPeriodically(context.Context, int)
	SetNodeOrchestrator(*SetOrchestratorTypeRequest)
	SyncNodeStatus(string, string, string, json.RawMessage) (int, string)
	GetPendingProgramIPConfigs() []IPConfigurationStatus
	GetAvailableIPConfigs() []IPConfigurationStatus
	GetAllocatedIPConfigs() []IPConfigurationStatus
	GetPendingReleaseIPConfigs() []IPConfigurationStatus
	GetPodIPConfigState() map[string]IPConfigurationStatus
	MarkIPAsPendingRelease(numberToMark int) (map[string]IPConfigurationStatus, error)
}

// This is used for KubernetesCRD orchestrator Type where NC has multiple ips.
// This struct captures the state for SecondaryIPs associated to a given NC
type IPConfigurationStatus struct {
	NCID      string
	ID        string //uuid
	IPAddress string
	State     string
	PodInfo   PodInfo
}

func (i IPConfigurationStatus) String() string {
	return fmt.Sprintf("IPConfigurationStatus: Id: [%s], NcId: [%s], IpAddress: [%s], State: [%s], PodInfo: [%v]",
		i.ID, i.NCID, i.IPAddress, i.State, i.PodInfo)
}

// SetEnvironmentRequest describes the Request to set the environment in CNS.
type SetEnvironmentRequest struct {
	Location    string
	NetworkType string
}

// OverlayConfiguration describes configuration for all the nodes that are part of overlay.
type OverlayConfiguration struct {
	NodeCount     int
	LocalNodeIP   string
	OverlaySubent Subnet
	NodeConfig    []NodeConfiguration
}

// CreateNetworkRequest describes request to create the network.
type CreateNetworkRequest struct {
	NetworkName          string
	OverlayConfiguration OverlayConfiguration
	Options              map[string]interface{}
}

// DeleteNetworkRequest describes request to delete the network.
type DeleteNetworkRequest struct {
	NetworkName string
}

// CreateHnsNetworkRequest describes request to create the HNS network.
type CreateHnsNetworkRequest struct {
	NetworkName          string
	NetworkType          string
	NetworkAdapterName   string            `json:",omitempty"`
	SourceMac            string            `json:",omitempty"`
	Policies             []json.RawMessage `json:",omitempty"`
	MacPools             []MacPool         `json:",omitempty"`
	Subnets              []SubnetInfo
	DNSSuffix            string `json:",omitempty"`
	DNSServerList        string `json:",omitempty"`
	DNSServerCompartment uint32 `json:",omitempty"`
	ManagementIP         string `json:",omitempty"`
	AutomaticDNS         bool   `json:",omitempty"`
}

// SubnetInfo is assoicated with HNS network and represents a list
// of subnets available to the network
type SubnetInfo struct {
	AddressPrefix  string
	GatewayAddress string
	Policies       []json.RawMessage `json:",omitempty"`
}

// MacPool is assoicated with HNS  network and represents a list
// of macaddresses available to the network
type MacPool struct {
	StartMacAddress string
	EndMacAddress   string
}

// DeleteHnsNetworkRequest describes request to delete the HNS network.
type DeleteHnsNetworkRequest struct {
	NetworkName string
}

// ReserveIPAddressRequest describes request to reserve an IP Address
type ReserveIPAddressRequest struct {
	ReservationID string
}

// ReserveIPAddressResponse describes response to reserve an IP address.
type ReserveIPAddressResponse struct {
	Response  Response
	IPAddress string
}

// ReleaseIPAddressRequest describes request to release an IP Address.
type ReleaseIPAddressRequest struct {
	ReservationID string
}

// IPAddressesUtilizationResponse describes response for ip address utilization.
type IPAddressesUtilizationResponse struct {
	Response  Response
	Available int
	Reserved  int
	Unhealthy int
}

// GetIPAddressesResponse describes response containing requested ip addresses.
type GetIPAddressesResponse struct {
	Response    Response
	IPAddresses []string
}

// HostLocalIPAddressResponse describes reponse that returns the host local IP Address.
type HostLocalIPAddressResponse struct {
	Response  Response
	IPAddress string
}

// Subnet contains the ip address and the number of bits in prefix.
type Subnet struct {
	IPAddress    string
	PrefixLength int
}

// NodeConfiguration describes confguration for a node in overlay network.
type NodeConfiguration struct {
	NodeIP     string
	NodeID     string
	NodeSubnet Subnet
}
type IPAMPoolMonitor interface {
	Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error
	Update(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error
	GetStateSnapshot() IpamPoolMonitorStateSnapshot
}

//struct to expose state values for IPAMPoolMonitor struct
type IpamPoolMonitorStateSnapshot struct {
	MinimumFreeIps           int64
	MaximumFreeIps           int64
	UpdatingIpsNotInUseCount int
	CachedNNC                nnc.NodeNetworkConfig
}

// Response describes generic response from CNS.
type Response struct {
	ReturnCode int
	Message    string
}

// NumOfCPUCoresResponse describes num of cpu cores present on host.
type NumOfCPUCoresResponse struct {
	Response      Response
	NumOfCPUCores int
}

// OptionMap describes generic options that can be passed to CNS.
type OptionMap map[string]interface{}

// Response to a failed request.
type errorResponse struct {
	Err string
}

// CreateHostNCApipaEndpointRequest describes request for create apipa endpoint
// for host container connectivity for the given network container
type CreateHostNCApipaEndpointRequest struct {
	NetworkContainerID string
}

// CreateHostNCApipaEndpointResponse describes response for create apipa endpoint request
// for host container connectivity.
type CreateHostNCApipaEndpointResponse struct {
	Response   Response
	EndpointID string
}

// DeleteHostNCApipaEndpointRequest describes request for deleting apipa endpoint created
// for host NC connectivity.
type DeleteHostNCApipaEndpointRequest struct {
	NetworkContainerID string
}

// DeleteHostNCApipaEndpointResponse describes response for delete host NC apipa endpoint request.
type DeleteHostNCApipaEndpointResponse struct {
	Response Response
}

type NmAgentSupportedApisRequest struct {
	GetNmAgentSupportedApisURL string
}

type NmAgentSupportedApisResponse struct {
	Response      Response
	SupportedApis []string
}
