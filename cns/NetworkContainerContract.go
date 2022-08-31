package cns

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// Container Network Service DNC Contract
const (
	SetOrchestratorType                      = "/network/setorchestratortype"
	CreateOrUpdateNetworkContainer           = "/network/createorupdatenetworkcontainer"
	DeleteNetworkContainer                   = "/network/deletenetworkcontainer"
	GetNetworkContainerStatus                = "/network/getnetworkcontainerstatus"
	PublishNetworkContainer                  = "/network/publishnetworkcontainer"
	UnpublishNetworkContainer                = "/network/unpublishnetworkcontainer"
	GetInterfaceForContainer                 = "/network/getinterfaceforcontainer"
	GetNetworkContainerByOrchestratorContext = "/network/getnetworkcontainerbyorchestratorcontext"
	AttachContainerToNetwork                 = "/network/attachcontainertonetwork"
	DetachContainerFromNetwork               = "/network/detachcontainerfromnetwork"
	RequestIPConfig                          = "/network/requestipconfig"
	ReleaseIPConfig                          = "/network/releaseipconfig"
	PathDebugIPAddresses                     = "/debug/ipaddresses"
	PathDebugPodContext                      = "/debug/podcontext"
	PathDebugRestData                        = "/debug/restdata"
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

// ChannelMode :- CNS channel modes
const (
	Direct         = "Direct"
	Managed        = "Managed"
	CRD            = "CRD"
	MultiTenantCRD = "MultiTenantCRD"
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
}

// CreateNetworkContainerRequest implements fmt.Stringer for logging
func (req *CreateNetworkContainerRequest) String() string {
	return fmt.Sprintf("CreateNetworkContainerRequest"+
		"{Version: %s, NetworkContainerType: %s, NetworkContainerid: %s, PrimaryInterfaceIdentifier: %s, "+
		"LocalIPConfiguration: %+v, IPConfiguration: %+v, SecondaryIPConfigs: %+v, MultitenancyInfo: %+v, "+
		"AllowHostToNCCommunication: %t, AllowNCToHostCommunication: %t}",
		req.Version, req.NetworkContainerType, req.NetworkContainerid, req.PrimaryInterfaceIdentifier, req.LocalIPConfiguration,
		req.IPConfiguration, req.SecondaryIPConfigs, req.MultiTenancyInfo, req.AllowHostToNCCommunication, req.AllowNCToHostCommunication)
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

var GlobalPodInfoScheme podInfoScheme

// podInfoScheme indicates which schema should be used when generating
// the map key in the Key() function on a podInfo object.
type podInfoScheme int

const (
	KubernetesPodInfoScheme podInfoScheme = iota
	InterfaceIDPodInfoScheme
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
}

type KubernetesPodInfo struct {
	PodName      string
	PodNamespace string
}

var _ PodInfo = (*podInfo)(nil)

// podInfo implements PodInfo for multiple schemas of Key
type podInfo struct {
	KubernetesPodInfo
	PodInfraContainerID string
	PodInterfaceID      string
	Version             podInfoScheme
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
func (p *podInfo) Key() string {
	if p.Version == InterfaceIDPodInfoScheme {
		return p.PodInterfaceID
	}
	return p.PodName + ":" + p.PodNamespace
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

// UnmarshalPodInfo wraps json.Unmarshal to return an implementation of
// PodInfo.
func UnmarshalPodInfo(b []byte) (PodInfo, error) {
	p := &podInfo{}
	if err := json.Unmarshal(b, p); err != nil {
		return nil, err
	}
	p.Version = GlobalPodInfoScheme
	return p, nil
}

// NewPodInfoFromIPConfigRequest builds and returns an implementation of
// PodInfo from the provided IPConfigRequest.
func NewPodInfoFromIPConfigRequest(req IPConfigRequest) (PodInfo, error) {
	p, err := UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		return nil, err
	}
	if GlobalPodInfoScheme == InterfaceIDPodInfoScheme && req.PodInterfaceID == "" {
		return nil, fmt.Errorf("need interfaceID for pod info but request was empty")
	}
	p.(*podInfo).PodInfraContainerID = req.InfraContainerID
	p.(*podInfo).PodInterfaceID = req.PodInterfaceID
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

// IPConfiguration contains details about ip config to provision in the VM.
type IPConfiguration struct {
	IPSubnet         IPSubnet
	DNSServers       []string
	GatewayIPAddress string
}

// SecondaryIPConfig contains IP info of SecondaryIP
type SecondaryIPConfig struct {
	IPAddress string
	// NCVesion will help in determining whether IP is in pending programming or available when reconciling.
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

// GetNetworkContainerStatusRequest specifies the details about the request to retrieve status of a specifc network container.
type GetNetworkContainerStatusRequest struct {
	NetworkContainerid string
}

// GetNetworkContainerStatusResponse specifies response of retriving a network container status.
type GetNetworkContainerStatusResponse struct {
	NetworkContainerid string
	Version            string
	AzureHostVersion   string
	Response           Response
}

// GetNetworkContainerRequest specifies the details about the request to retrieve a specifc network container.
type GetNetworkContainerRequest struct {
	NetworkContainerid  string
	OrchestratorContext json.RawMessage
}

// GetNetworkContainerResponse describes the response to retrieve a specifc network container.
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
}

// DeleteNetworkContainerRequest specifies the details about the request to delete a specifc network container.
type PodIpInfo struct {
	PodIPConfig                     IPSubnet
	NetworkContainerPrimaryIPConfig IPConfiguration
	HostPrimaryIPInfo               HostIPInfo
}

// DeleteNetworkContainerRequest specifies the details about the request to delete a specifc network container.
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

func (i IPConfigRequest) String() string {
	return fmt.Sprintf("[IPConfigRequest: DesiredIPAddress %s, PodInterfaceID %s, InfraContainerID %s, OrchestratorContext %s]",
		i.DesiredIPAddress, i.PodInterfaceID, i.InfraContainerID, string(i.OrchestratorContext))
}

// IPConfigResponse is used in CNS IPAM mode as a response to CNI ADD
type IPConfigResponse struct {
	PodIpInfo PodIpInfo
	Response  Response
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

// GetPodContextResponse is used in CNS Client debug mode to get mapping of Orchestrator Context to Pod IP UUID
type GetPodContextResponse struct {
	PodContext map[string]string
	Response   Response
}

// IPAddressState Only used in the GetIPConfig API to return IPs that match a filter
type IPAddressState struct {
	IPAddress string
	State     string
}

// DeleteNetworkContainerRequest specifies the details about the request to delete a specifc network container.
type DeleteNetworkContainerRequest struct {
	NetworkContainerid string
}

// DeleteNetworkContainerResponse describes the response to delete a specifc network container.
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

// NetworkInterface specifies the information that can be used to unquely identify an interface.
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

// NetworkContainerParameters parameters available in network container operations
type NetworkContainerParameters struct {
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

// UnpublishNetworkContainerRequest specifies request to unpublish network container via NMAgent.
type UnpublishNetworkContainerRequest struct {
	NetworkID                 string
	NetworkContainerID        string
	JoinNetworkURL            string
	DeleteNetworkContainerURL string
}

// UnpublishNetworkContainerResponse specifies the response to unpublish network container request.
type UnpublishNetworkContainerResponse struct {
	Response              Response
	UnpublishErrorStr     string
	UnpublishStatusCode   int
	UnpublishResponseBody []byte
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
