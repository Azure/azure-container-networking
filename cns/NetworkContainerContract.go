package cns

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// Container Network Service DNC Contract
const (
	SetOrchestratorType                      = "/network/setorchestratortype"
	GetHomeAz                                = "/homeaz"
	GetNCList                                = "/nclist"
	GetVMUniqueID                            = "/metadata/vmuniqueid"
	CreateOrUpdateNetworkContainer           = "/network/createorupdatenetworkcontainer"
	DeleteNetworkContainer                   = "/network/deletenetworkcontainer"
	PublishNetworkContainer                  = "/network/publishnetworkcontainer"
	UnpublishNetworkContainer                = "/network/unpublishnetworkcontainer"
	GetInterfaceForContainer                 = "/network/getinterfaceforcontainer"
	GetNetworkContainerByOrchestratorContext = "/network/getnetworkcontainerbyorchestratorcontext"
	GetAllNetworkContainers                  = "/network/getAllNetworkContainers"
	NetworkContainersURLPath                 = "/network/networkcontainers"
	AttachContainerToNetwork                 = "/network/attachcontainertonetwork"
	DetachContainerFromNetwork               = "/network/detachcontainerfromnetwork"
	RequestIPConfig                          = "/network/requestipconfig"
	RequestIPConfigs                         = "/network/requestipconfigs"
	ReleaseIPConfig                          = "/network/releaseipconfig"
	ReleaseIPConfigs                         = "/network/releaseipconfigs"
	PathDebugIPAddresses                     = "/debug/ipaddresses"
	PathDebugPodContext                      = "/debug/podcontext"
	PathDebugRestData                        = "/debug/restdata"
	NumberOfCPUCores                         = NumberOfCPUCoresPath
	NMAgentSupportedAPIs                     = NmAgentSupportedApisPath
	EndpointAPI                              = EndpointPath
)

// NetworkContainer Prefixes
const (
	SwiftPrefix = "Swift_"
)

// NetworkContainer Types
const (
	AzureContainerInstance = "AzureContainerInstance"
	WebApps                = "WebApps"
	Docker                 = "Docker"
	Basic                  = "Basic"
	JobObject              = "JobObject"
	COW                    = "COW" // Container on Windows
	BackendNICNC           = "BackendNICNC"
)

// Orchestrator Types
const (
	Kubernetes      = "Kubernetes"
	ServiceFabric   = "ServiceFabric"
	Batch           = "Batch"
	DBforPostgreSQL = "DBforPostgreSQL"
	AzureFirstParty = "AzureFirstParty"
	KubernetesCRD   = "KubernetesCRD"
	// TODO: Add OrchastratorType as CRD: https://msazure.visualstudio.com/One/_workitems/edit/7711872
)

// Encap Types
const (
	Vlan  = "Vlan"
	Vxlan = "Vxlan"
)

type NICType string

// NIC Types
const (
	InfraNIC NICType = "InfraNIC"
	// DelegatedVMNIC are projected from VM to container network namespace
	DelegatedVMNIC NICType = "FrontendNIC"
	// BackendNIC are used for infiniband NICs on a VM
	BackendNIC NICType = "BackendNIC"
	// NodeNetworkInterfaceAccelnetFrontendNIC is a type of front-end nic that offers accelerated networking performance
	NodeNetworkInterfaceAccelnetFrontendNIC NICType = "FrontendNIC_Accelnet"

	// TODO: These two const are currently unused due to version compatibility with DNC. DelegatedVMNIC and NodeNetworkInterfaceBackendNIC should be renamed to align with the naming convention with DNC
	// NodeNetworkInterfaceFrontendNIC is the new name for DelegatedVMNIC
	NodeNetworkInterfaceFrontendNIC NICType = "FrontendNIC"
	// NodeNetworkInterfaceBackendNIC is the new name for BackendNIC
	NodeNetworkInterfaceBackendNIC NICType = "BackendNIC"
)

// ChannelMode :- CNS channel modes
const (
	Direct         = "Direct"
	Managed        = "Managed"
	CRD            = "CRD"
	MultiTenantCRD = "MultiTenantCRD"
	AzureHost      = "AzureHost"
)

var (
	ErrInvalidNCID = errors.New("invalid NetworkContainerID")
	ErrInvalidIP   = errors.New("invalid IP")
)

// CreateNetworkContainerRequest specifies request to create a network container or network isolation boundary.
type CreateNetworkContainerRequest struct {
	HostPrimaryIP              string
	Version                    string
	NetworkContainerType       string
	NetworkContainerid         string // Mandatory input.
	PrimaryInterfaceIdentifier string // Primary CA.
	AuthorizationToken         string
	LocalIPConfiguration       IPConfiguration
	OrchestratorContext        json.RawMessage
	IPConfiguration            IPConfiguration
	SecondaryIPConfigs         map[string]SecondaryIPConfig // uuid is key
	MultiTenancyInfo           MultiTenancyInfo
	CnetAddressSpace           []IPSubnet // To setup SNAT (should include service endpoint vips).
	Routes                     []Route
	AllowHostToNCCommunication bool
	AllowNCToHostCommunication bool
	EndpointPolicies           []NetworkContainerRequestPolicies
	NCStatus                   v1alpha.NCStatus
	NetworkInterfaceInfo       NetworkInterfaceInfo //nolint // introducing new field for backendnic, to be used later by cni code
}

func (req *CreateNetworkContainerRequest) Validate() error {
	if req.NetworkContainerid == "" {
		return errors.Wrap(ErrInvalidNCID, "NetworkContainerID is empty")
	}
	if _, err := uuid.Parse(strings.TrimPrefix(req.NetworkContainerid, SwiftPrefix)); err != nil {
		return errors.Wrapf(ErrInvalidNCID, "NetworkContainerID %s is not a valid UUID: %s", req.NetworkContainerid, err.Error())
	}
	if req.PrimaryInterfaceIdentifier != "" && !isValidIP(req.PrimaryInterfaceIdentifier) {
		return errors.Wrapf(ErrInvalidIP, "PrimaryInterfaceIdentifier %s is not a valid ip address", req.PrimaryInterfaceIdentifier)
	}
	if req.IPConfiguration.GatewayIPAddress != "" && !isValidIP(req.IPConfiguration.GatewayIPAddress) {
		return errors.Wrapf(ErrInvalidIP, "GatewayIPAddress %s is not a valid ip address", req.IPConfiguration.GatewayIPAddress)
	}
	return nil
}

func isValidIP(ipStr string) bool {
	// if can parse (i.e. not nil), then valid ip
	if ip, _, err := net.ParseCIDR(ipStr); err == nil {
		return ip != nil
	}
	ip := net.ParseIP(ipStr)
	return ip != nil
}

// CreateNetworkContainerRequest implements fmt.Stringer for logging
func (req *CreateNetworkContainerRequest) String() string {
	return fmt.Sprintf("CreateNetworkContainerRequest"+
		"{Version: %s, NetworkContainerType: %s, NetworkContainerid: %s, PrimaryInterfaceIdentifier: %s, "+
		"LocalIPConfiguration: %+v, IPConfiguration: %+v, SecondaryIPConfigs: %+v, MultitenancyInfo: %+v, "+
		"AllowHostToNCCommunication: %t, AllowNCToHostCommunication: %t, NCStatus: %s, NetworkInterfaceInfo: %+v}",
		req.Version, req.NetworkContainerType, req.NetworkContainerid, req.PrimaryInterfaceIdentifier, req.LocalIPConfiguration,
		req.IPConfiguration, req.SecondaryIPConfigs, req.MultiTenancyInfo, req.AllowHostToNCCommunication, req.AllowNCToHostCommunication,
		string(req.NCStatus), req.NetworkInterfaceInfo)
}

// NetworkContainerRequestPolicies - specifies policies associated with create network request
type NetworkContainerRequestPolicies struct {
	Type         string
	EndpointType string
	Settings     json.RawMessage
}

// ConfigureContainerNetworkingRequest - specifies request to attach/detach container to network.
type ConfigureContainerNetworkingRequest struct {
	Containerid        string
	NetworkContainerid string
}

// ErrDuplicateIP indicates that a duplicate IP has been detected during a reconcile.
var ErrDuplicateIP = errors.New("duplicate IP detected in CNS initialization")

// PodInfoByIPProvider to be implemented by initializers which provide a map
// of PodInfos by IP.
type PodInfoByIPProvider interface {
	PodInfoByIP() (map[string]PodInfo, error)
}

var _ PodInfoByIPProvider = (PodInfoByIPProviderFunc)(nil)

// PodInfoByIPProviderFunc functional type which implements PodInfoByIPProvider.
// Allows one-off functional implementations of the PodInfoByIPProvider
// interface when a custom type definition is not necessary.
type PodInfoByIPProviderFunc func() (map[string]PodInfo, error)

// PodInfoByIP implements PodInfoByIPProvider on PodInfByIPProviderFunc.
func (f PodInfoByIPProviderFunc) PodInfoByIP() (map[string]PodInfo, error) {
	return f()
}

var GlobalPodInfoScheme = InterfaceIDPodInfoScheme

// podInfoScheme indicates which schema should be used when generating
// the map key in the Key() function on a podInfo object.
type podInfoScheme int

const (
	_ podInfoScheme = iota
	InterfaceIDPodInfoScheme
	InfraIDPodInfoScheme
)

// PodInfo represents the object that we are providing network for.
type PodInfo interface {
	// InfraContainerID the CRI infra container for the pod namespace.
	InfraContainerID() string
	// InterfaceID a short hash of the infra container and the primary network
	// interface of the pod net ns.
	InterfaceID() string
	// Key is a unique string representation of the PodInfo.
	Key() string
	// Name is the orchestrator pod name.
	Name() string
	// Namespace is the orchestrator pod namespace.
	Namespace() string
	// OrchestratorContext is a JSON KubernetesPodInfo
	OrchestratorContext() (json.RawMessage, error)
	// Equals implements a functional equals for PodInfos
	Equals(PodInfo) bool
	// String implements string for logging PodInfos
	String() string
	// SecondaryInterfacesExist returns true if there exist a secondary interface for this pod
	SecondaryInterfacesExist() bool
}

type KubernetesPodInfo struct {
	PodName      string
	PodNamespace string
}

var _ PodInfo = (*podInfo)(nil)

// podInfo implements PodInfo for multiple schemas of Key
type podInfo struct {
	KubernetesPodInfo
	PodInfraContainerID   string
	PodInterfaceID        string
	Version               podInfoScheme
	SecondaryInterfaceSet bool
}

func (p podInfo) String() string {
	return fmt.Sprintf("InfraContainerID: [%s], InterfaceID: [%s], Key: [%s], Name: [%s], Namespace: [%s]",
		p.InfraContainerID(), p.InterfaceID(), p.Key(), p.Name(), p.Namespace())
}

func (p *podInfo) Equals(o PodInfo) bool {
	if (p == nil) != (o == nil) {
		return false
	}
	if p == nil {
		return true
	}
	return p.Key() == o.Key()
}

func (p *podInfo) InfraContainerID() string {
	return p.PodInfraContainerID
}

func (p *podInfo) InterfaceID() string {
	return p.PodInterfaceID
}

// Key is a unique string representation of the PodInfo.
// If the PodInfo.Version == kubernetes, the Key is composed of the
// orchestrator pod name and namespace. if the Version is interfaceID, key is
// composed of the CNI interfaceID, which is generated from the CRI infra
// container ID and the pod net ns primary interface name.
// If the version in InfraContainerID then the key is containerID.
func (p *podInfo) Key() string {
	switch p.Version {
	case InfraIDPodInfoScheme:
		return p.PodInfraContainerID
	default:
		return p.PodInterfaceID
	}
}

func (p *podInfo) Name() string {
	return p.PodName
}

func (p *podInfo) Namespace() string {
	return p.PodNamespace
}

func (p *podInfo) OrchestratorContext() (json.RawMessage, error) {
	jsonContext, err := json.Marshal(p.KubernetesPodInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PodInfo, error: %w", err)
	}
	return jsonContext, nil
}

func (p *podInfo) SecondaryInterfacesExist() bool {
	return p.SecondaryInterfaceSet
}

func (p *podInfo) UnmarshalJSON(b []byte) error {
	type alias podInfo
	// Unmarshal into a temporary struct to avoid infinite recursion
	a := &struct {
		*alias
	}{
		alias: (*alias)(p),
	}
	if err := json.Unmarshal(b, a); err != nil {
		return errors.Wrap(err, "failed to unmarshal podInfo")
	}
	p.Version = GlobalPodInfoScheme
	return nil
}

// NewPodInfo returns an implementation of PodInfo that returns the passed
// configuration for their namesake functions.
func NewPodInfo(infraContainerID, interfaceID, name, namespace string) PodInfo {
	return &podInfo{
		KubernetesPodInfo: KubernetesPodInfo{
			PodName:      name,
			PodNamespace: namespace,
		},
		PodInfraContainerID: infraContainerID,
		PodInterfaceID:      interfaceID,
		Version:             GlobalPodInfoScheme,
	}
}

func UnmarshalPodInfo(b []byte) (PodInfo, error) {
	p := &podInfo{}
	err := json.Unmarshal(b, p)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NewPodInfoFromIPConfigsRequest builds and returns an implementation of
// PodInfo from the provided IPConfigsRequest.
func NewPodInfoFromIPConfigsRequest(req IPConfigsRequest) (PodInfo, error) {
	p, err := UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		return nil, err
	}
	if GlobalPodInfoScheme == InterfaceIDPodInfoScheme && req.PodInterfaceID == "" {
		return nil, fmt.Errorf("need interfaceID for pod info but request was empty")
	}
	p.(*podInfo).PodInfraContainerID = req.InfraContainerID
	p.(*podInfo).PodInterfaceID = req.PodInterfaceID
	p.(*podInfo).SecondaryInterfaceSet = req.SecondaryInterfacesExist
	return p, nil
}

func KubePodsToPodInfoByIP(pods []corev1.Pod) (map[string]PodInfo, error) {
	podInfoByIP := map[string]PodInfo{}
	for i := range pods {
		if pods[i].Spec.HostNetwork {
			// ignore host network pods.
			continue
		}
		if strings.TrimSpace(pods[i].Status.PodIP) == "" {
			// ignore pods without an assigned IP.
			continue
		}
		// error if we have already recorded that this IP is assigned to a Pod.
		if _, ok := podInfoByIP[pods[i].Status.PodIP]; ok {
			return nil, errors.Wrap(ErrDuplicateIP, pods[i].Status.PodIP)
		}
		// record the PodInfo by assigned IP.
		podInfoByIP[pods[i].Status.PodIP] = NewPodInfo("", "", pods[i].Name, pods[i].Namespace)
	}
	return podInfoByIP, nil
}

// MultiTenancyInfo contains encap type and id.
type MultiTenancyInfo struct {
	EncapType string
	ID        int // This can be vlanid, vxlanid, gre-key etc. (depends on EnacapType).
}

type NetworkInterfaceInfo struct {
	NICType    NICType
	MACAddress string
}

// IPConfiguration contains details about ip config to provision in the VM.
type IPConfiguration struct {
	IPSubnet         IPSubnet
	DNSServers       []string
	GatewayIPAddress string
}

// SecondaryIPConfig contains IP info of SecondaryIP
type SecondaryIPConfig struct {
	IPAddress string
	// NCVersion will help in determining whether IP is in pending programming or available when reconciling.
	NCVersion int
}

// IPSubnet contains ip subnet.
type IPSubnet struct {
	IPAddress    string
	PrefixLength uint8
}

// GetIPNet converts the IPSubnet to the standard net type
func (ips *IPSubnet) GetIPNet() (net.IP, *net.IPNet, error) {
	prefix := strconv.Itoa(int(ips.PrefixLength))
	return net.ParseCIDR(ips.IPAddress + "/" + prefix)
}

// Route describes an entry in routing table.
type Route struct {
	IPAddress        string
	GatewayIPAddress string
	InterfaceToUse   string
}

// SetOrchestratorTypeRequest specifies the orchestrator type for the node.
type SetOrchestratorTypeRequest struct {
	OrchestratorType string
	DncPartitionKey  string
	NodeID           string
}

// CreateNetworkContainerResponse specifies response of creating a network container.
type CreateNetworkContainerResponse struct {
	Response Response
}

// GetNetworkContainerStatusRequest specifies the details about the request to retrieve status of a specific network container.
type GetNetworkContainerStatusRequest struct {
	NetworkContainerid string
}

// GetNetworkContainerStatusResponse specifies response of retrieving a network container status.
type GetNetworkContainerStatusResponse struct {
	NetworkContainerid string
	Version            string
	AzureHostVersion   string
	Response           Response
}

// GetAllNetworkContainersResponse specifies response of retrieving all NCs from CNS during the process of NC refresh association.
type GetAllNetworkContainersResponse struct {
	NetworkContainers []GetNetworkContainerResponse
	Response          Response
}

// PostNetworkContainersRequest specifies the request of creating all NCs that are sent from DNC.
type PostNetworkContainersRequest struct {
	CreateNetworkContainerRequests []CreateNetworkContainerRequest
}

func (req *PostNetworkContainersRequest) Validate() error {
	for i := range req.CreateNetworkContainerRequests {
		if err := req.CreateNetworkContainerRequests[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PostNetworkContainersResponse specifies response of creating all NCs that are sent from DNC.
type PostNetworkContainersResponse struct {
	Response Response
}

// GetNetworkContainerRequest specifies the details about the request to retrieve a specific network container.
type GetNetworkContainerRequest struct {
	NetworkContainerid  string
	OrchestratorContext json.RawMessage
}

// GetNetworkContainerResponse describes the response to retrieve a specific network container.
type GetNetworkContainerResponse struct {
	NetworkContainerID         string
	IPConfiguration            IPConfiguration
	Routes                     []Route
	CnetAddressSpace           []IPSubnet
	MultiTenancyInfo           MultiTenancyInfo
	PrimaryInterfaceIdentifier string
	LocalIPConfiguration       IPConfiguration
	Response                   Response
	AllowHostToNCCommunication bool
	AllowNCToHostCommunication bool
	NetworkInterfaceInfo       NetworkInterfaceInfo
}

type PodIpInfo struct {
	PodIPConfig                     IPSubnet
	NetworkContainerPrimaryIPConfig IPConfiguration
	HostPrimaryIPInfo               HostIPInfo
	NICType                         NICType
	InterfaceName                   string
	// MacAddress of interface
	MacAddress string
	// SkipDefaultRoutes is true if default routes should not be added on interface
	SkipDefaultRoutes bool
	// Routes to configure on interface
	Routes []Route
	// PnpId is set for backend interfaces, Pnp Id identifies VF. Plug and play id(pnp) is also called as PCI ID
	PnPID string
	// Default Deny ACL's to configure on HNS endpoints for Swiftv2 window nodes
	EndpointPolicies []policy.Policy
}

type HostIPInfo struct {
	Gateway   string
	PrimaryIP string
	Subnet    string
}

type IPConfigRequest struct {
	DesiredIPAddress    string
	PodInterfaceID      string
	InfraContainerID    string
	OrchestratorContext json.RawMessage
	Ifname              string // Used by delegated IPAM
}

// Same as IPConfigRequest except that DesiredIPAddresses is passed in as a slice
type IPConfigsRequest struct {
	DesiredIPAddresses           []string        `json:"desiredIPAddresses"`
	PodInterfaceID               string          `json:"podInterfaceID"`
	InfraContainerID             string          `json:"infraContainerID"`
	OrchestratorContext          json.RawMessage `json:"orchestratorContext"`
	Ifname                       string          `json:"ifname"`                   // Used by delegated IPAM
	SecondaryInterfacesExist     bool            `json:"secondaryInterfacesExist"` // will be set by SWIFT v2 validator func
	BackendInterfaceExist        bool            `json:"BackendInterfaceExist"`    // will be set by SWIFT v2 validator func
	BackendInterfaceMacAddresses []string        `json:"BacknendInterfaceMacAddress"`
}

// IPConfigResponse is used in CNS IPAM mode as a response to CNI ADD
type IPConfigResponse struct {
	PodIpInfo PodIpInfo
	Response  Response
}

// IPConfigsResponse is used in CNS IPAM mode to return a slice of IP configs as a response to CNI ADD
type IPConfigsResponse struct {
	PodIPInfo []PodIpInfo `json:"podIPInfo"`
	Response  Response    `json:"response"`
}

// GetIPAddressesRequest is used in CNS IPAM mode to get the states of IPConfigs
// The IPConfigStateFilter is a slice of IPs to fetch from CNS that match those states
type GetIPAddressesRequest struct {
	IPConfigStateFilter []types.IPState
}

// GetIPAddressStateResponse is used in CNS IPAM mode as a response to get IP address state
type GetIPAddressStateResponse struct {
	IPAddresses []IPAddressState
	Response    Response
}

// GetIPAddressStatusResponse is used in CNS IPAM mode as a response to get IP address, state and Pod info
type GetIPAddressStatusResponse struct {
	IPConfigurationStatus []IPConfigurationStatus
	Response              Response
}

// GetPodContextResponse is used in CNS Client debug mode to get mapping of Orchestrator Context to Pod IP UUIDs
type GetPodContextResponse struct {
	PodContext map[string][]string // Can have multiple Pod IP UUIDs in the case of dualstack
	Response   Response
}

// IPAddressState Only used in the GetIPConfig API to return IPs that match a filter
type IPAddressState struct {
	IPAddress string
	State     string
}

// DeleteNetworkContainerRequest specifies the details about the request to delete a specific network container.
type DeleteNetworkContainerRequest struct {
	NetworkContainerid string
}

// DeleteNetworkContainerResponse describes the response to delete a specific network container.
type DeleteNetworkContainerResponse struct {
	Response Response
}

// GetInterfaceForContainerRequest specifies the container ID for which interface needs to be identified.
type GetInterfaceForContainerRequest struct {
	NetworkContainerID string
}

// GetInterfaceForContainerResponse specifies the interface for a given container ID.
type GetInterfaceForContainerResponse struct {
	NetworkContainerVersion string
	NetworkInterface        NetworkInterface
	CnetAddressSpace        []IPSubnet
	DNSServers              []string
	Response                Response
}

// AttachContainerToNetworkResponse specifies response of attaching network container to network.
type AttachContainerToNetworkResponse struct {
	Response Response
}

// DetachContainerFromNetworkResponse specifies response of detaching network container from network.
type DetachContainerFromNetworkResponse struct {
	Response Response
}

// NetworkInterface specifies the information that can be used to uniquely identify an interface.
type NetworkInterface struct {
	Name      string
	IPAddress string
}

// PublishNetworkContainerRequest specifies request to publish network container via NMAgent.
type PublishNetworkContainerRequest struct {
	NetworkID                         string
	NetworkContainerID                string
	JoinNetworkURL                    string
	CreateNetworkContainerURL         string
	CreateNetworkContainerRequestBody []byte
}

func (p PublishNetworkContainerRequest) String() string {
	// %q as a verb on a byte slice prints safely escaped text instead of individual bytes
	return fmt.Sprintf("{NetworkID:%s NetworkContainerID:%s JoinNetworkURL:%s CreateNetworkContainerURL:%s CreateNetworkContainerRequestBody:%q}",
		p.NetworkID, p.NetworkContainerID, p.JoinNetworkURL, p.CreateNetworkContainerURL, p.CreateNetworkContainerRequestBody)
}

// NetworkContainerParameters parameters available in network container operations
type NetworkContainerParameters struct {
	NCID                  string
	AuthToken             string
	AssociatedInterfaceID string
}

// PublishNetworkContainerResponse specifies the response to publish network container request.
type PublishNetworkContainerResponse struct {
	Response            Response
	PublishErrorStr     string
	PublishStatusCode   int
	PublishResponseBody []byte
}

func (p PublishNetworkContainerResponse) String() string {
	// %q as a verb on a byte slice prints safely escaped text instead of individual bytes
	return fmt.Sprintf("{Response:%+v PublishErrStr:%s PublishStatusCode:%d PublishResponseBody:%q}",
		p.Response, p.PublishErrorStr, p.PublishStatusCode, p.PublishResponseBody)
}

// UnpublishNetworkContainerRequest specifies request to unpublish network container via NMAgent.
type UnpublishNetworkContainerRequest struct {
	NetworkID                         string
	NetworkContainerID                string
	JoinNetworkURL                    string
	DeleteNetworkContainerURL         string
	DeleteNetworkContainerRequestBody []byte
}

func (u UnpublishNetworkContainerRequest) String() string {
	return fmt.Sprintf("{NetworkID:%s NetworkContainerID:%s JoinNetworkURL:%s DeleteNetworkContainerURL:%s DeleteNetworkContainerRequestBody:%q}",
		u.NetworkID, u.NetworkContainerID, u.JoinNetworkURL, u.DeleteNetworkContainerURL, u.DeleteNetworkContainerRequestBody)
}

// UnpublishNetworkContainerResponse specifies the response to unpublish network container request.
type UnpublishNetworkContainerResponse struct {
	Response              Response
	UnpublishErrorStr     string
	UnpublishStatusCode   int
	UnpublishResponseBody []byte
}

func (u UnpublishNetworkContainerResponse) String() string {
	// %q as a verb on a byte slice prints safely escaped text instead of individual bytes
	return fmt.Sprintf("{Response:%+v UnpublishErrorStr:%s UnpublishStatusCode:%d UnpublishResponseBody:%q}",
		u.Response, u.UnpublishErrorStr, u.UnpublishStatusCode, u.UnpublishResponseBody)
}

// ValidAclPolicySetting - Used to validate ACL policy
type ValidAclPolicySetting struct {
	Protocols       string `json:","`
	Action          string `json:","`
	Direction       string `json:","`
	LocalAddresses  string `json:","`
	RemoteAddresses string `json:","`
	LocalPorts      string `json:","`
	RemotePorts     string `json:","`
	RuleType        string `json:","`
	Priority        uint16 `json:","`
}

const (
	ActionTypeAllow  string = "Allow"
	ActionTypeBlock  string = "Block"
	DirectionTypeIn  string = "In"
	DirectionTypeOut string = "Out"
)

// Validate - Validates network container request policies
func (networkContainerRequestPolicy *NetworkContainerRequestPolicies) Validate() error {
	// validate ACL policy
	if networkContainerRequestPolicy != nil {
		if strings.EqualFold(networkContainerRequestPolicy.Type, "ACLPolicy") && strings.EqualFold(networkContainerRequestPolicy.EndpointType, "APIPA") {
			var requestedAclPolicy ValidAclPolicySetting
			if err := json.Unmarshal(networkContainerRequestPolicy.Settings, &requestedAclPolicy); err != nil {
				return fmt.Errorf("ACL policy failed to pass validation with error: %+v ", err)
			}
			// Deny request if ACL Action is empty
			if len(strings.TrimSpace(string(requestedAclPolicy.Action))) == 0 {
				return fmt.Errorf("Action field cannot be empty in ACL Policy")
			}
			// Deny request if ACL Action is not Allow or Deny
			if !strings.EqualFold(requestedAclPolicy.Action, ActionTypeAllow) && !strings.EqualFold(requestedAclPolicy.Action, ActionTypeBlock) {
				return fmt.Errorf("Only Allow or Block is supported in Action field")
			}
			// Deny request if ACL Direction is empty
			if len(strings.TrimSpace(string(requestedAclPolicy.Direction))) == 0 {
				return fmt.Errorf("Direction field cannot be empty in ACL Policy")
			}
			// Deny request if ACL direction is not In or Out
			if !strings.EqualFold(requestedAclPolicy.Direction, DirectionTypeIn) && !strings.EqualFold(requestedAclPolicy.Direction, DirectionTypeOut) {
				return fmt.Errorf("Only In or Out is supported in Direction field")
			}
			if requestedAclPolicy.Priority == 0 {
				return fmt.Errorf("Priority field cannot be empty in ACL Policy")
			}
		} else {
			return fmt.Errorf("Only ACL Policies on APIPA endpoint supported")
		}
	}
	return nil
}

// NodeInfoResponse - Struct to hold the node info response.
type NodeInfoResponse struct {
	NetworkContainers []CreateNetworkContainerRequest
}

// NodeRegisterRequest - Struct to hold the node register request.
type NodeRegisterRequest struct {
	NumCores             int
	NmAgentSupportedApis []string
}
