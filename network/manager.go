// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// Network store key.
	storeKey             = "Network"
	VlanIDKey            = "VlanID"
	AzureCNS             = "azure-cns"
	SNATIPKey            = "NCPrimaryIPKey"
	RoutesKey            = "RoutesKey"
	IPTablesKey          = "IPTablesKey"
	genericData          = "com.docker.network.generic"
	ipv6AddressMask      = 128
	cnsBaseURL           = "" // fallback to default http://localhost:10090
	cnsReqTimeout        = 15 * time.Second
	StateLessCNIIsNotSet = "StateLess CNI mode is not enabled"
	InfraInterfaceName   = "eth0"
	ContainerIDLength    = 8
	EndpointIfIndex      = 0 // Azure CNI supports only one interface
	DefaultNetworkID     = "azure"
	// TODO: Remove dummy GUID and come up with more permanent solution
	dummyGUID = "12345678-1234-1234-1234-123456789012" // guid to trigger hnsv2 in windows
)

var Ipv4DefaultRouteDstPrefix = net.IPNet{
	IP:   net.IPv4zero,
	Mask: net.IPv4Mask(0, 0, 0, 0),
}

var Ipv6DefaultRouteDstPrefix = net.IPNet{
	IP: net.IPv6zero,
	// This mask corresponds to a /0 subnet for IPv6
	Mask: net.CIDRMask(0, ipv6AddressMask),
}

type NetworkClient interface {
	CreateBridge() error
	DeleteBridge() error
	AddL2Rules(extIf *externalInterface) error
	DeleteL2Rules(extIf *externalInterface)
	SetBridgeMasterToHostInterface() error
	SetHairpinOnHostInterface(bool) error
}

type EndpointClient interface {
	AddEndpoints(epInfo *EndpointInfo) error
	AddEndpointRules(epInfo *EndpointInfo) error
	DeleteEndpointRules(ep *endpoint)
	MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error
	SetupContainerInterfaces(epInfo *EndpointInfo) error
	ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error
	DeleteEndpoints(ep *endpoint) error
}

// NetworkManager manages the set of container networking resources.
type networkManager struct {
	statelessCniMode   bool
	CnsClient          *cnsclient.Client
	Version            string
	TimeStamp          time.Time
	ExternalInterfaces map[string]*externalInterface
	store              store.KeyValueStore
	netlink            netlink.NetlinkInterface
	netio              netio.NetIOInterface
	plClient           platform.ExecClient
	nsClient           NamespaceClientInterface
	iptablesClient     ipTablesClient
	dhcpClient         dhcpClient
	sync.Mutex
}

// NetworkManager API.
type NetworkManager interface {
	Initialize(config *common.PluginConfig, isRehydrationRequired bool) error
	Uninitialize()

	AddExternalInterface(ifName, subnet, nicType string) error

	CreateNetwork(nwInfo *EndpointInfo) error
	DeleteNetwork(networkID string) error
	GetNetworkInfo(networkID string) (EndpointInfo, error)
	// FindNetworkIDFromNetNs returns the network name that contains an endpoint created for this netNS, errNetworkNotFound if no network is found
	FindNetworkIDFromNetNs(netNs string) (string, error)
	GetNumEndpointsByContainerID(containerID string) int

	CreateEndpoint(client apipaClient, networkID string, epInfo *EndpointInfo) error
	EndpointCreate(client apipaClient, epInfos []*EndpointInfo) error // TODO: change name
	DeleteEndpoint(networkID string, endpointID string, epInfo *EndpointInfo, mode string) error
	GetEndpointInfo(networkID string, endpointID string) (*EndpointInfo, error)
	GetAllEndpoints(networkID string) (map[string]*EndpointInfo, error)
	GetEndpointInfoBasedOnPODDetails(networkID string, podName string, podNameSpace string, doExactMatchForPodName bool) (*EndpointInfo, error)
	AttachEndpoint(networkID string, endpointID string, sandboxKey string) (*endpoint, error)
	DetachEndpoint(networkID string, endpointID string) error
	UpdateEndpoint(networkID string, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) error
	GetNumberOfEndpoints(ifName string, networkID string) int
	GetEndpointID(containerID, ifName string) string
	IsStatelessCNIMode() bool
	SaveState(eps []*endpoint) error
	DeleteState(epInfos []*EndpointInfo) error
	GetEndpointInfosFromContainerID(containerID string) []*EndpointInfo
	GetEndpointState(networkID, containerID, netns string) ([]*EndpointInfo, error)
	GetEndpointIDByNicType(containerID, ifName string, nicType cns.NICType) string
}

// Creates a new network manager.
func NewNetworkManager(nl netlink.NetlinkInterface, plc platform.ExecClient, netioCli netio.NetIOInterface, nsc NamespaceClientInterface,
	iptc ipTablesClient, dhcpc dhcpClient,
) (NetworkManager, error) {
	nm := &networkManager{
		ExternalInterfaces: make(map[string]*externalInterface),
		netlink:            nl,
		plClient:           plc,
		netio:              netioCli,
		nsClient:           nsc,
		iptablesClient:     iptc,
		dhcpClient:         dhcpc,
	}

	return nm, nil
}

// Initialize configures network manager.
func (nm *networkManager) Initialize(config *common.PluginConfig, isRehydrationRequired bool) error {
	nm.Version = config.Version
	nm.store = config.Store
	if config.Stateless {
		if err := nm.SetStatelessCNIMode(); err != nil {
			return errors.Wrapf(err, "Failed to initialize stateles CNI")
		}
		return nil
	}

	// Restore persisted state.
	err := nm.restore(isRehydrationRequired)
	return err
}

// Uninitialize cleans up network manager.
func (nm *networkManager) Uninitialize() {
}

// SetStatelessCNIMode enable the statelessCNI falg and inititlizes a CNSClient
func (nm *networkManager) SetStatelessCNIMode() error {
	nm.statelessCniMode = true
	// Create CNS client
	client, err := cnsclient.New(cnsBaseURL, cnsReqTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize CNS client")
	}
	nm.CnsClient = client
	return nil
}

// IsStatelessCNIMode checks if the Stateless CNI mode has been enabled or not
func (nm *networkManager) IsStatelessCNIMode() bool {
	return nm.statelessCniMode
}

// Restore reads network manager state from persistent store.
func (nm *networkManager) restore(isRehydrationRequired bool) error {
	// Skip if a store is not provided.
	if nm.store == nil {
		logger.Info("network store is nil")
		return nil
	}

	rebooted := false
	// After a reboot, all address resources are implicitly released.
	// Ignore the persisted state if it is older than the last reboot time.

	// Read any persisted state.
	err := nm.store.Read(storeKey, nm)
	if err != nil {
		if err == store.ErrKeyNotFound {
			logger.Info("network store key not found")
			// Considered successful.
			return nil
		} else if err == store.ErrStoreEmpty {
			logger.Info("network store empty")
			return nil
		} else {
			logger.Error("Failed to restore state", zap.Error(err))
			return err
		}
	}

	if isRehydrationRequired {
		modTime, err := nm.store.GetModificationTime()
		if err == nil {
			rebootTime, err := nm.plClient.GetLastRebootTime()
			logger.Info("reboot time, store mod time", zap.Any("rebootTime", rebootTime), zap.Any("modTime", modTime))
			if err == nil && rebootTime.After(modTime) {
				logger.Info("Detected Reboot")
				rebooted = true
				if clearNwConfig, err := nm.plClient.ClearNetworkConfiguration(); clearNwConfig {
					if err != nil {
						logger.Error("Failed to clear network configuration", zap.Error(err))
						return err
					}

					// Delete the networks left behind after reboot
					for _, extIf := range nm.ExternalInterfaces {
						for _, nw := range extIf.Networks {
							logger.Info("Deleting the network on reboot", zap.String("id", nw.Id))
							_ = nm.deleteNetwork(nw.Id)
						}
					}

					// Clear networkManager contents
					nm.TimeStamp = time.Time{}
					for extIfName := range nm.ExternalInterfaces {
						delete(nm.ExternalInterfaces, extIfName)
					}

					return nil
				}
			}
		}
	}
	// Populate pointers.
	for _, extIf := range nm.ExternalInterfaces {
		for _, nw := range extIf.Networks {
			nw.extIf = extIf
		}
	}

	// if rebooted recreate the network that existed before reboot.
	if rebooted {
		logger.Info("Rehydrating network state from persistent store")
		for _, extIf := range nm.ExternalInterfaces {
			for _, nw := range extIf.Networks {
				nwInfo, err := nm.GetNetworkInfo(nw.Id)
				if err != nil {
					logger.Error("Failed to fetch network info for network extif err. This should not happen",
						zap.Any("nw", nw), zap.Any("extIf", extIf), zap.Error(err))
					return err
				}

				extIf.BridgeName = ""

				_, err = nm.newNetworkImpl(&nwInfo, extIf)
				if err != nil {
					logger.Error("Restoring network failed for nwInfo extif. This should not happen",
						zap.Any("nwInfo", nwInfo), zap.Any("extIf", extIf), zap.Error(err))
					return err
				}
			}
		}
	}

	logger.Info("Restored state")
	return nil
}

// Save writes network manager state to persistent store.
func (nm *networkManager) save() error {
	// CNI is not maintaining the state in Steless Mode.
	if nm.IsStatelessCNIMode() {
		return nil
	}
	// Skip if a store is not provided.
	if nm.store == nil {
		return nil
	}

	// Update time stamp.
	nm.TimeStamp = time.Now()

	err := nm.store.Write(storeKey, nm)
	if err == nil {
		logger.Info("Save succeeded")
	} else {
		logger.Error("Save failed", zap.Error(err))
	}
	return err
}

//
// NetworkManager API
//
// Provides atomic stateful wrappers around core networking functionality.
//

// AddExternalInterface adds a host interface to the list of available external interfaces.
func (nm *networkManager) AddExternalInterface(ifName, subnet, nicType string) error {
	nm.Lock()
	defer nm.Unlock()

	err := nm.newExternalInterface(ifName, subnet, nicType)
	if err != nil {
		return err
	}

	return nil
}

// CreateNetwork creates a new container network.
func (nm *networkManager) CreateNetwork(epInfo *EndpointInfo) error {
	nm.Lock()
	defer nm.Unlock()

	_, err := nm.newNetwork(epInfo)
	if err != nil {
		return err
	}

	return nil
}

// DeleteNetwork deletes an existing container network.
func (nm *networkManager) DeleteNetwork(networkID string) error {
	nm.Lock()
	defer nm.Unlock()

	err := nm.deleteNetwork(networkID)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// GetNetworkInfo returns information about the given network.
func (nm *networkManager) GetNetworkInfo(networkID string) (EndpointInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return EndpointInfo{}, err
	}

	nwInfo := EndpointInfo{
		NetworkID:        networkID,
		Subnets:          nw.Subnets,
		Mode:             nw.Mode,
		EnableSnatOnHost: nw.EnableSnatOnHost,
		Options:          make(map[string]interface{}),
	}

	getNetworkInfoImpl(&nwInfo, nw)

	if nw.extIf != nil {
		nwInfo.BridgeName = nw.extIf.BridgeName
	}

	return nwInfo, nil
}

func (nm *networkManager) createEndpoint(cli apipaClient, networkID string, epInfo *EndpointInfo) (*endpoint, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return nil, err
	}

	if nw.VlanId != 0 {
		if epInfo.Data[VlanIDKey] == nil {
			logger.Info("overriding endpoint vlanid with network vlanid")
			epInfo.Data[VlanIDKey] = nw.VlanId
		}
	}

	ep, err := nw.newEndpoint(cli, nm.netlink, nm.plClient, nm.netio, nm.nsClient, nm.iptablesClient, nm.dhcpClient, epInfo)
	if err != nil {
		return nil, err
	}
	// any error after this point should also clean up the endpoint we created above
	defer func() {
		if err != nil {
			logger.Error("Create endpoint failure", zap.Error(err))
			logger.Info("Cleanup resources")
			delErr := nw.deleteEndpoint(nm.netlink, nm.plClient, nm.netio, nm.nsClient, nm.iptablesClient, nm.dhcpClient, ep.Id, epInfo.Mode)
			if delErr != nil {
				logger.Error("Deleting endpoint after create endpoint failure failed with", zap.Error(delErr))
			}
		}
	}()

	return ep, nil
}

// CreateEndpoint creates a new container endpoint (this is for compatibility-- add flow should no longer use this).
func (nm *networkManager) CreateEndpoint(cli apipaClient, networkID string, epInfo *EndpointInfo) error {
	_, err := nm.createEndpoint(cli, networkID, epInfo)
	return err
}

// UpdateEndpointState will make a call to CNS updatEndpointState API in the stateless CNI mode
// It will add HNSEndpointID or HostVeth name to the endpoint state
func (nm *networkManager) UpdateEndpointState(eps []*endpoint) error {
	if len(eps) == 0 {
		return nil
	}

	ifnameToIPInfoMap := generateCNSIPInfoMap(eps) // key : interface name, value : IPInfo
	for key, ipinfo := range ifnameToIPInfoMap {
		logger.Info("Update endpoint state", zap.String("ifname", key), zap.String("hnsEndpointID", ipinfo.HnsEndpointID), zap.String("hnsNetworkID", ipinfo.HnsNetworkID),
			zap.String("hostVethName", ipinfo.HostVethName), zap.String("macAddress", ipinfo.MacAddress), zap.String("nicType", string(ipinfo.NICType)),
			zap.String("networkContainerID", ipinfo.NetworkContainerID))
	}
	// we assume all endpoints have the same container id
	cnsEndpointID := eps[0].ContainerID

	if err := validateUpdateEndpointState(cnsEndpointID, ifnameToIPInfoMap); err != nil {
		return errors.Wrap(err, "failed to validate update endpoint state that will be sent to cns")
	}
	response, err := nm.CnsClient.UpdateEndpoint(context.TODO(), cnsEndpointID, ifnameToIPInfoMap)
	if err != nil {
		return errors.Wrapf(err, "Update endpoint API returend with error")
	}
	logger.Info("Update endpoint API returend ", zap.String("podname: ", response.ReturnCode.String()))
	return nil
}

func validateUpdateEndpointState(endpointID string, ifNameToIPInfoMap map[string]*restserver.IPInfo) error {
	if endpointID == "" {
		return errors.New("endpoint id empty while validating update endpoint state")
	}
	for ifName := range ifNameToIPInfoMap {
		if ifName == "" {
			return errors.New("an interface name is empty while validating update endpoint state")
		}
	}
	return nil
}

// GetEndpointState will make a call to CNS GetEndpointState API in the stateless CNI mode to fetch the endpointInfo
// TODO unit tests need to be added, WorkItem: 26606939
// In stateless cni, container id is the endpoint id, so you can pass in either
func (nm *networkManager) GetEndpointState(networkID, containerID, netns string) ([]*EndpointInfo, error) {
	endpointResponse, err := nm.CnsClient.GetEndpoint(context.TODO(), containerID)
	if err != nil {
		emptyEpInfos := []*EndpointInfo{}
		if endpointResponse.Response.ReturnCode == types.NotFound {
			logger.Info("Endpoint Not found", zap.String("containerID", containerID), zap.Error(err))
			return emptyEpInfos, ErrEndpointStateNotFound
		}
		if endpointResponse.Response.ReturnCode == types.ConnectionError {
			logger.Info("failed to connect to CNS", zap.String("containerID", containerID), zap.Error(err))
			return emptyEpInfos, ErrConnectionFailure
		}
		return emptyEpInfos, ErrGetEndpointStateFailure
	}
	epInfos := cnsEndpointInfotoCNIEpInfos(endpointResponse.EndpointInfo, containerID, netns)

	for i := 0; i < len(epInfos); i++ {
		if epInfos[i].NICType == cns.InfraNIC {
			if epInfos[i].IsEndpointStateIncomplete() { // assume false for swift v2 for now
				if networkID == "" {
					networkID = DefaultNetworkID
				}
				epInfos[i], err = epInfos[i].GetEndpointInfoByIPImpl(epInfos[i].IPAddresses, networkID)
				if err != nil {
					logger.Info("Endpoint State is incomlete for endpoint: ", zap.Error(err), zap.String("endpointID", epInfos[i].EndpointID))
				}
			}
		}
	}
	return epInfos, nil
}

// DeleteEndpoint deletes an existing container endpoint.
func (nm *networkManager) DeleteEndpoint(networkID, endpointID string, epInfo *EndpointInfo, mode string) error {
	nm.Lock()
	defer nm.Unlock()

	if nm.IsStatelessCNIMode() {
		// Calls deleteEndpointImpl directly, skipping the get network check; does not call cns
		return nm.DeleteEndpointStateless(networkID, epInfo, mode)
	}

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return err
	}

	err = nw.deleteEndpoint(nm.netlink, nm.plClient, nm.netio, nm.nsClient, nm.iptablesClient, nm.dhcpClient, endpointID, mode)
	if err != nil {
		return err
	}

	return nil
}

func (nm *networkManager) DeleteEndpointStateless(networkID string, epInfo *EndpointInfo, mode string) error {
	// we want to always use hnsv2 in stateless
	// hnsv2 is only enabled if NetNs has a valid guid and the hnsv2 api is supported
	// by passing in a dummy guid, we satisfy the first condition
	nw := &network{
		Id:           networkID, // currently unused in stateless cni
		HnsId:        epInfo.HNSNetworkID,
		Mode:         opModeTransparent,
		SnatBridgeIP: "",
		NetNs:        dummyGUID, // to trigger hns v2, windows
		extIf: &externalInterface{
			Name:       InfraInterfaceName,
			MacAddress: nil,
		},
	}

	ep := &endpoint{
		Id:                       epInfo.EndpointID,
		HnsId:                    epInfo.HNSEndpointID,
		HNSNetworkID:             epInfo.HNSNetworkID, // unused (we use nw.HnsId for deleting the network)
		HostIfName:               epInfo.HostIfName,
		LocalIP:                  "",
		IPAddresses:              epInfo.IPAddresses,
		NetworkNameSpace:         epInfo.NetNsPath,
		VlanID:                   0,
		AllowInboundFromHostToNC: false, // stateless currently does not support apipa
		AllowInboundFromNCToHost: false,
		EnableSnatOnHost:         false,
		EnableMultitenancy:       false,
		NetworkContainerID:       epInfo.NetworkContainerID, // we don't use this as long as AllowInboundFromHostToNC and AllowInboundFromNCToHost are false
		NetNs:                    dummyGUID,                 // to trigger hnsv2, windows
		NICType:                  epInfo.NICType,
		IfName:                   epInfo.IfName, // TODO: For stateless cni linux populate IfName here to use in deletion in secondary endpoint client
	}
	logger.Info("Deleting endpoint with", zap.String("Endpoint Info: ", epInfo.PrettyString()), zap.String("HNISID : ", ep.HnsId))

	err := nw.deleteEndpointImpl(nm.netlink, nm.plClient, nil, nm.netio, nm.nsClient, nm.iptablesClient, nm.dhcpClient, ep, nw.Mode)
	if err != nil {
		return err
	}

	err = nm.deleteNetworkImpl(nw, ep.NICType)
	// no need to clean up state in stateless
	if err != nil {
		return errors.Wrap(err, "Failed to delete HNS Network")
	}

	return nil
}

// GetEndpointInfo returns information about the given endpoint.
func (nm *networkManager) GetEndpointInfo(networkID, endpointID string) (*EndpointInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	if nm.IsStatelessCNIMode() {
		logger.Info("calling cns getEndpoint API")
		epInfos, err := nm.GetEndpointState(networkID, endpointID, "")
		if err != nil {
			return nil, err
		}
		for _, epInfo := range epInfos {
			if epInfo.NICType == cns.InfraNIC {
				return epInfo, nil
			}
		}

		return nil, err
	}

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpoint(endpointID)
	if err != nil {
		return nil, err
	}

	return ep.getInfo(), nil
}

func (nm *networkManager) GetAllEndpoints(networkId string) (map[string]*EndpointInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	eps := make(map[string]*EndpointInfo)

	// Special case when CNS invokes CNI, but there is no state, but return gracefully
	if len(nm.ExternalInterfaces) == 0 {
		logger.Info("Network manager has no external interfaces, is the state file populated?")
		return eps, store.ErrStoreEmpty
	}

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	for epid, ep := range nw.Endpoints {
		eps[epid] = ep.getInfo()
	}

	return eps, nil
}

// GetEndpointInfoBasedOnPODDetails returns information about the given endpoint.
// It returns an error if a single pod has multiple endpoints.
func (nm *networkManager) GetEndpointInfoBasedOnPODDetails(networkID string, podName string, podNameSpace string, doExactMatchForPodName bool) (*EndpointInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpointByPOD(podName, podNameSpace, doExactMatchForPodName)
	if err != nil {
		return nil, err
	}

	return ep.getInfo(), nil
}

// AttachEndpoint attaches an endpoint to a sandbox.
func (nm *networkManager) AttachEndpoint(networkId string, endpointId string, sandboxKey string) (*endpoint, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return nil, err
	}

	err = ep.attach(sandboxKey)
	if err != nil {
		return nil, err
	}

	err = nm.save()
	if err != nil {
		return nil, err
	}

	return ep, nil
}

// DetachEndpoint detaches an endpoint from its sandbox.
func (nm *networkManager) DetachEndpoint(networkId string, endpointId string) error {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return err
	}

	err = ep.detach()
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// UpdateEndpoint updates an existing container endpoint.
func (nm *networkManager) UpdateEndpoint(networkID string, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) error {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkID)
	if err != nil {
		return err
	}

	err = nm.updateEndpoint(nw, existingEpInfo, targetEpInfo)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

func (nm *networkManager) GetNumberOfEndpoints(ifName string, networkId string) int {
	if ifName == "" {
		for key := range nm.ExternalInterfaces {
			ifName = key
			break
		}
	}

	if nm.ExternalInterfaces != nil {
		extIf := nm.ExternalInterfaces[ifName]
		if extIf != nil && extIf.Networks != nil {
			nw := extIf.Networks[networkId]
			if nw != nil && nw.Endpoints != nil {
				return len(nw.Endpoints)
			}
		}
	}

	return 0
}

// GetEndpointID returns a unique endpoint ID based on the CNI mode.
func (nm *networkManager) GetEndpointID(containerID, ifName string) string {
	if nm.IsStatelessCNIMode() {
		return containerID
	}
	if len(containerID) > ContainerIDLength {
		containerID = containerID[:ContainerIDLength]
	} else {
		log.Printf("Container ID is not greater than 8 ID: %v", containerID)
		return ""
	}
	return containerID + "-" + ifName
}

// saves the map of network ids to endpoints to the state file
func (nm *networkManager) SaveState(eps []*endpoint) error {
	nm.Lock()
	defer nm.Unlock()

	logger.Info("Saving state")
	// If we fail half way, we'll propagate an error up which should clean everything up
	if nm.IsStatelessCNIMode() {
		err := nm.UpdateEndpointState(eps)
		return err
	}

	// once endpoints and networks are in-memory, save once
	return nm.save()
}

func (nm *networkManager) DeleteState(epInfos []*EndpointInfo) error {
	nm.Lock()
	defer nm.Unlock()

	// For AKS stateless cni Windows, plugin.ipamInvoker.Delete takes care of removing the state in the main Delete function.
	// For swiftv2 stateless cni in Windows, this call will delete the endpoint state from CNS.
	// For SwiftV2 stateless cni in Linux, plugin.ipamInvoker.Delete already deletes the endpoint state from CNS.
	if nm.IsStatelessCNIMode() {
		for _, epInfo := range epInfos {
			if epInfo.NICType == cns.InfraNIC {
				// If the scanrio contains InfraNic it means the state has already been deleted in CNS by plugin.ipamInvoker.Delete
				return nil
			}
		}
		logger.Info("Deleting delegated NIC from state")
		for _, epInfo := range epInfos {
			// this cleanup happens only for standalone swiftv2 Windows to delete endpoint state from CNS.
			if epInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC || epInfo.NICType == cns.NodeNetworkInterfaceAccelnetFrontendNIC {
				// swiftv2 multitenancy does not call plugin.ipamInvoker.Delete and so state does not automatically clean up. this call is required to
				// cleanup state in CNS
				// One Delete call for endpointID will remove all interface info associated with that endpointID in CNS
				response, err := nm.CnsClient.DeleteEndpointState(context.TODO(), epInfo.EndpointID)
				if err != nil {
					if response != nil && response.ReturnCode == types.NotFound {
						logger.Info("Endpoint state not found in CNS", zap.String("endpointID", epInfo.EndpointID))
						return nil
					}
					return errors.Wrapf(err, "Delete endpoint API returned with error for endpoint %s", epInfo.EndpointID)
				}
				logger.Info("Delete endpoint succeeded", zap.String("endpointID", epInfo.EndpointID), zap.String("returnCode", response.ReturnCode.String()))
				break
			}
		}
		return nil
	}

	// once endpoints and networks are deleted in-memory, save once
	return nm.save()
}

// called to convert a cns restserver EndpointInfo into a network EndpointInfo
func cnsEndpointInfotoCNIEpInfos(endpointInfo restserver.EndpointInfo, endpointID, netns string) []*EndpointInfo {
	ret := []*EndpointInfo{}

	for ifName, ipInfo := range endpointInfo.IfnameToIPMap {
		epInfo := &EndpointInfo{
			EndpointID:   endpointID,      // endpoint id is always the same, but we shouldn't use it in the stateless path
			IfIndex:      EndpointIfIndex, // Azure CNI supports only one interface
			ContainerID:  endpointID,
			PODName:      endpointInfo.PodName,
			PODNameSpace: endpointInfo.PodNamespace,
		}

		// If we create an endpoint state with stateful cni and then swap to a stateless cni binary, ifname would not be populated
		// triggered in migration to stateless only, assuming no incomplete state for delegated
		if ifName == "" {
			ifName = InfraInterfaceName
			ipInfo.NICType = cns.InfraNIC
		}

		// filling out the InfraNIC from the state
		epInfo.IPAddresses = ipInfo.IPv4
		epInfo.IPAddresses = append(epInfo.IPAddresses, ipInfo.IPv6...)
		epInfo.IfName = ifName // epInfo.IfName is set to the value of ep.IfName when the endpoint was added
		// sidenote: ifname doesn't seem to be used in linux (or even windows) deletion
		epInfo.HostIfName = ipInfo.HostVethName
		epInfo.HNSEndpointID = ipInfo.HnsEndpointID
		epInfo.NICType = ipInfo.NICType
		epInfo.HNSNetworkID = ipInfo.HnsNetworkID
		epInfo.MacAddress = net.HardwareAddr(ipInfo.MacAddress)
		epInfo.NetworkContainerID = ipInfo.NetworkContainerID
		epInfo.NetNsPath = netns

		ret = append(ret, epInfo)
	}
	return ret
}

// gets all endpoint infos associated with a container id and populates the network id field in Stateful CNI mode
// nictype may be empty in which case it is likely of type "infra"
func (nm *networkManager) GetEndpointInfosFromContainerID(containerID string) []*EndpointInfo {
	ret := []*EndpointInfo{}
	for _, extIf := range nm.ExternalInterfaces {
		for networkID, nw := range extIf.Networks {
			for _, ep := range nw.Endpoints {
				if ep.ContainerID == containerID {
					val := ep.getInfo()
					val.NetworkID = networkID // endpoint doesn't contain the network id
					ret = append(ret, val)
				}
			}
		}
	}
	return ret
}

// generates a map of interface names to IPInfo structs for updating CNS endpoint state
func generateCNSIPInfoMap(eps []*endpoint) map[string]*restserver.IPInfo {
	ifNametoIPInfoMap := make(map[string]*restserver.IPInfo) // key : interface name, value : IPInfo

	for _, ep := range eps {
		ifNametoIPInfoMap[ep.IfName] = &restserver.IPInfo{ // in windows, the nicname is args ifname, in linux, it's ethX
			NICType:            ep.NICType,
			HnsEndpointID:      ep.HnsId,
			HnsNetworkID:       ep.HNSNetworkID,
			HostVethName:       ep.HostIfName,
			MacAddress:         ep.MacAddress.String(),
			NetworkContainerID: ep.NetworkContainerID,
		}
	}

	return ifNametoIPInfoMap
}

// GetEndpointIDByNicType returns a unique endpoint ID based on the CNI mode and NIC type
func (nm *networkManager) GetEndpointIDByNicType(containerID, ifName string, nicType cns.NICType) string {
	// For stateless CNI, secondary NICs use #containerID-#ifName as endpointID such as 12345678-eth1
	if nm.IsStatelessCNIMode() && nicType != cns.InfraNIC {
		return containerID + "-" + ifName
	}
	// For InfraNIC, use GetEndpointID() logic.
	return nm.GetEndpointID(containerID, ifName)
}
