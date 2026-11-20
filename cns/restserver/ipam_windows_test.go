package restserver

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customerNetwork builds an HNS network the way CNI does for a VLAN-isolated NC.
func customerNetwork(t *testing.T, id, prefix string, vlanID uint32) hcn.HostComputeNetwork {
	t.Helper()
	settings, err := json.Marshal(hcn.VlanPolicySetting{IsolationId: vlanID})
	require.NoError(t, err)
	policy, err := json.Marshal(hcn.SubnetPolicy{Type: hcn.VLAN, Settings: settings})
	require.NoError(t, err)
	return hcn.HostComputeNetwork{Id: id, Ipams: []hcn.Ipam{{Subnets: []hcn.Subnet{{IpAddressPrefix: prefix, Policies: []json.RawMessage{policy}}}}}}
}

func hnsEndpoint(id, name, network, ip string) hcn.HostComputeEndpoint {
	return hcn.HostComputeEndpoint{Id: id, Name: name, HostComputeNetwork: network, IpConfigurations: []hcn.IpConfig{{IpAddress: ip}}}
}

// apipaEndpoint builds the host APIPA endpoint CNS creates for ncID.
func apipaEndpoint(id, ncID, ip string) hcn.HostComputeEndpoint {
	return hnsEndpoint(id, hnsclient.GetHostNCApipaEndpointName(ncID), "apipa-network", ip)
}

// containerEndpoint builds a container endpoint CNI creates in a customer network.
func containerEndpoint(id, network, ip string) hcn.HostComputeEndpoint {
	return hnsEndpoint(id, id+"-eth0", network, ip)
}

func TestCleanupStaleHNSResourcesSwiftV1(t *testing.T) {
	const (
		ncID    = "Swift_new"
		ncIP    = "10.0.0.5"
		apipaIP = "169.254.128.5"

		leakedApipa     = "leaked-apipa"
		leakedContainer = "leaked-container"
	)
	// The NC below belongs to "customer-network". The other two networks hold the same IP for a different customer.
	networks := []hcn.HostComputeNetwork{
		customerNetwork(t, "customer-network", "10.0.0.0/24", 2),
		customerNetwork(t, "other-vlan-network", "10.0.0.0/24", 3),
		customerNetwork(t, "other-subnet-network", "10.0.0.0/16", 2),
	}

	// A new SwiftV1 NC in "customer-network" with host-NC communication, so it gets an APIPA endpoint.
	nc := cns.CreateNetworkContainerRequest{
		NetworkContainerid:         ncID,
		IPConfiguration:            cns.IPConfiguration{IPSubnet: cns.IPSubnet{IPAddress: ncIP, PrefixLength: 24}},
		LocalIPConfiguration:       cns.IPConfiguration{IPSubnet: cns.IPSubnet{IPAddress: apipaIP, PrefixLength: 17}},
		MultiTenancyInfo:           cns.MultiTenancyInfo{EncapType: cns.Vlan, ID: 2},
		AllowHostToNCCommunication: true,
	}
	ncWithoutVLAN := nc
	ncWithoutVLAN.MultiTenancyInfo = cns.MultiTenancyInfo{}
	// DNC still gives these NCs an APIPA IP, but CNS never creates their APIPA endpoint.
	ncWithoutHostNCCommunication := nc
	ncWithoutHostNCCommunication.AllowHostToNCCommunication = false
	ncWithNCToHostCommunication := ncWithoutHostNCCommunication
	ncWithNCToHostCommunication.AllowNCToHostCommunication = true
	ncOnDelegatedNIC := nc
	ncOnDelegatedNIC.NetworkInterfaceInfo = cns.NetworkInterfaceInfo{NICType: cns.DelegatedVMNIC, MACAddress: testEndpointMACAddress}

	tests := []struct {
		name                string
		req                 cns.CreateNetworkContainerRequest
		knownNC             bool // CNS already has the NC, so this is a repeat create
		manageEndpointState bool
		endpoints           []hcn.HostComputeEndpoint
		hnsErr              error
		wantErr             bool
		wantDeleted         []string
	}{
		// APIPA: every NC's APIPA endpoint is in one shared network, so the IP alone identifies a leak.
		{
			name:        "deletes another NC's APIPA endpoint holding the APIPA IP",
			req:         nc,
			endpoints:   []hcn.HostComputeEndpoint{apipaEndpoint(leakedApipa, "Swift_old", apipaIP)},
			wantDeleted: []string{leakedApipa},
		},
		{
			name:      "keeps the NC's own APIPA endpoint",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{apipaEndpoint("own-apipa", ncID, apipaIP)},
		},
		{
			name:      "keeps APIPA endpoints holding other IPs",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{apipaEndpoint("other-apipa", "Swift_other", "169.254.128.6")},
		},
		{
			name:      "keeps non-APIPA endpoints holding the APIPA IP",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("container", "apipa-network", apipaIP)},
		},
		{
			name:      "NC without host-NC communication keeps APIPA endpoints, since it gets none to collide with",
			req:       ncWithoutHostNCCommunication,
			endpoints: []hcn.HostComputeEndpoint{apipaEndpoint(leakedApipa, "Swift_old", apipaIP)},
		},
		{
			name:        "NC with only NC-to-host communication deletes the leaked APIPA endpoint",
			req:         ncWithNCToHostCommunication,
			endpoints:   []hcn.HostComputeEndpoint{apipaEndpoint(leakedApipa, "Swift_old", apipaIP)},
			wantDeleted: []string{leakedApipa},
		},

		// NC IP: IPs are only unique within a customer network, so only look in the NC's.
		{
			name:        "new NC deletes the endpoint holding its IP in its customer network",
			req:         nc,
			endpoints:   []hcn.HostComputeEndpoint{containerEndpoint(leakedContainer, "customer-network", ncIP)},
			wantDeleted: []string{leakedContainer},
		},
		{
			name:      "keeps endpoints holding other IPs in the customer network",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("other-ip", "customer-network", "10.0.0.6")},
		},
		{
			name:      "keeps endpoints holding the NC IP in a network with another VLAN",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("other-vlan", "other-vlan-network", ncIP)},
		},
		{
			name:      "keeps endpoints holding the NC IP in a network with another subnet",
			req:       nc,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("other-subnet", "other-subnet-network", ncIP)},
		},
		{
			name:      "known NC keeps the endpoint holding its IP, which may be its own container",
			req:       nc,
			knownNC:   true,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("own", "customer-network", ncIP)},
		},
		{
			name:      "NC without VLAN has no customer network to look in",
			req:       ncWithoutVLAN,
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("unscoped", "customer-network", ncIP)},
		},
		{
			name:    "known NC without host-NC communication makes no HNS calls",
			req:     ncWithoutHostNCCommunication,
			knownNC: true,
			hnsErr:  errors.New("HNS should not be called"),
		},

		// SwiftV1 vs SwiftV2: only delegated NICs with managed endpoint state use EndpointState.
		{
			name: "delegated NIC without managed endpoint state scans HNS",
			req:  ncOnDelegatedNIC,
			endpoints: []hcn.HostComputeEndpoint{
				apipaEndpoint(leakedApipa, "Swift_old", apipaIP),
				containerEndpoint(leakedContainer, "customer-network", ncIP),
			},
			wantDeleted: []string{leakedApipa, leakedContainer},
		},
		{
			name:                "delegated NIC with managed endpoint state uses EndpointState, not HNS",
			req:                 ncOnDelegatedNIC,
			manageEndpointState: true,
			endpoints:           []hcn.HostComputeEndpoint{containerEndpoint(leakedContainer, "customer-network", ncIP)},
		},

		{
			name:    "HNS failure fails the cleanup",
			req:     nc,
			hnsErr:  errors.New("HNS down"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockHNSClient{endpoints: tt.endpoints, networks: networks, err: tt.hnsErr}
			useMockHNSClient(t, mockClient)

			svc := getTestService(cns.AzureContainerInstance)
			svc.Options[acn.OptManageEndpointState] = tt.manageEndpointState
			if tt.knownNC {
				svc.state.ContainerStatus = map[string]containerstatus{ncID: {ID: ncID, CreateNetworkContainerRequest: tt.req}}
			}
			err := svc.cleanupStaleHNSResources(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantDeleted, mockClient.deletedEndpointIDs)
		})
	}
}

// HNS calls can be slow, so cleanup must never hold the service lock across them.
func TestCleanupStaleHNSResourcesDoesNotHoldServiceLockDuringHNSCalls(t *testing.T) {
	const ncIP, staleEndpoint, staleNetwork = "10.0.0.5", "stale-ep", "stale-net"
	assertUnlocked := func(t *testing.T, svc *HTTPRestService) func() {
		return func() {
			if !svc.TryLock() {
				t.Error("service lock held during HNS call")
				return
			}
			svc.Unlock()
		}
	}

	t.Run("SwiftV1", func(t *testing.T) {
		svc := getTestService(cns.AzureContainerInstance)
		mockClient := &mockHNSClient{
			networks:  []hcn.HostComputeNetwork{customerNetwork(t, "customer-network", "10.0.0.0/24", 2)},
			endpoints: []hcn.HostComputeEndpoint{containerEndpoint("leaked", "customer-network", ncIP)},
		}
		mockClient.onCall = assertUnlocked(t, svc)
		useMockHNSClient(t, mockClient)

		require.NoError(t, svc.cleanupStaleHNSResources(cns.CreateNetworkContainerRequest{
			NetworkContainerid: "Swift_new",
			IPConfiguration:    cns.IPConfiguration{IPSubnet: cns.IPSubnet{IPAddress: ncIP, PrefixLength: 24}},
			MultiTenancyInfo:   cns.MultiTenancyInfo{EncapType: cns.Vlan, ID: 2},
		}))
		assert.Equal(t, []string{"leaked"}, mockClient.deletedEndpointIDs)
	})

	t.Run("SwiftV2 tolerates a concurrent CNI DEL removing the container", func(t *testing.T) {
		svc := getTestService(cns.AzureContainerInstance)
		svc.Options[acn.OptManageEndpointState] = true
		svc.EndpointStateStore = store.NewMockStore("")
		svc.EndpointState = map[string]*EndpointInfo{
			staleContainer: {IfnameToIPMap: map[string]*IPInfo{
				"delegated": {NICType: cns.DelegatedVMNIC, MacAddress: testEndpointMACAddress, HnsEndpointID: staleEndpoint, HnsNetworkID: staleNetwork},
			}},
		}
		mockClient := &mockHNSClient{}
		checkUnlocked := assertUnlocked(t, svc)
		mockClient.onCall = func() {
			checkUnlocked()
			// Simulate CNI DEL removing the container while HNS cleanup is in flight.
			svc.Lock()
			delete(svc.EndpointState, staleContainer)
			svc.Unlock()
		}
		useMockHNSClient(t, mockClient)

		require.NoError(t, svc.cleanupStaleHNSResources(cns.CreateNetworkContainerRequest{
			NetworkContainerid:   "Swift_new",
			NetworkInterfaceInfo: cns.NetworkInterfaceInfo{NICType: cns.DelegatedVMNIC, MACAddress: testEndpointMACAddress},
		}))
		assert.Equal(t, []string{staleEndpoint}, mockClient.deletedEndpointIDs)
		assert.Equal(t, []string{staleNetwork}, mockClient.deletedNetworkIDs)
		assert.Empty(t, svc.EndpointState)
	})
}

func TestCreateNetworkContainerScopesNCIPCleanupToNewNCs(t *testing.T) {
	podInfo, err := json.Marshal(cns.KubernetesPodInfo{PodName: "nc-pod", PodNamespace: "ns"})
	require.NoError(t, err)
	req := cns.CreateNetworkContainerRequest{
		NetworkContainerid:   "Swift_6aa05227-0271-43f3-908e-b1906460b875",
		NetworkContainerType: cns.AzureContainerInstance,
		Version:              "0",
		OrchestratorContext:  podInfo,
		IPConfiguration:      cns.IPConfiguration{IPSubnet: cns.IPSubnet{IPAddress: "10.0.0.5", PrefixLength: 24}},
		MultiTenancyInfo:     cns.MultiTenancyInfo{EncapType: cns.Vlan, ID: 2},
	}
	for _, tt := range []struct {
		name        string
		knownNC     bool // CNS already has the NC, so this is a repeat create
		wantDeleted []string
	}{
		{
			name:        "new NC deletes the leaked endpoint holding its IP",
			knownNC:     false,
			wantDeleted: []string{"leaked"},
		},
		{
			name:        "repeat create keeps the endpoint, which may be the NC's own container",
			knownNC:     true,
			wantDeleted: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockHNSClient{
				networks:  []hcn.HostComputeNetwork{customerNetwork(t, "customer-network", "10.0.0.0/24", 2)},
				endpoints: []hcn.HostComputeEndpoint{containerEndpoint("leaked", "customer-network", "10.0.0.5")},
			}
			useMockHNSClient(t, mockClient)

			svc := getTestService(cns.ServiceFabric)
			svc.store = store.NewMockStore("")
			svc.Options[acn.OptEnableStaleHNSCleanupOnNCCreate] = true
			if tt.knownNC {
				svc.state.ContainerStatus = map[string]containerstatus{req.NetworkContainerid: {ID: req.NetworkContainerid, CreateNetworkContainerRequest: req}}
			}
			body, err := json.Marshal(req) //nolint:musttag // legacy CNS type
			require.NoError(t, err)
			w := httptest.NewRecorder()
			svc.createOrUpdateNetworkContainer(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, cns.CreateOrUpdateNetworkContainer, bytes.NewReader(body)))

			var resp cns.CreateNetworkContainerResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp)) //nolint:musttag // legacy CNS type
			assert.Equal(t, types.Success, resp.Response.ReturnCode, resp.Response.Message)
			assert.Equal(t, tt.wantDeleted, mockClient.deletedEndpointIDs)
		})
	}
}

// Container and interface names used in EndpointState fixtures.
const (
	testContainer   = "container-1"
	staleContainer  = "stale-container"
	apipaIfName     = "apipa"
	delegatedIfName = "Ethernet 4"
)

// delegatedNICInfo is the EndpointState CNI records for a SwiftV2 delegated NIC.
func delegatedNICInfo(mac, endpointID, networkID string) *IPInfo {
	return &IPInfo{NICType: cns.DelegatedVMNIC, MacAddress: mac, HnsEndpointID: endpointID, HnsNetworkID: networkID}
}

// apipaNICInfo is the EndpointState CNI records for an NC's host APIPA endpoint.
func apipaNICInfo(ncID, ip, endpointID string) *IPInfo {
	return &IPInfo{
		NICType:            cns.ApipaNIC,
		NetworkContainerID: ncID,
		IPv4:               []net.IPNet{{IP: net.ParseIP(ip).To4(), Mask: net.CIDRMask(17, 32)}},
		HnsEndpointID:      endpointID,
	}
}

func TestCleanupStaleHNSResourcesSwiftV2(t *testing.T) {
	const (
		mac      = "00:22:48:ab:cd:ef"
		otherMAC = "aa:bb:cc:dd:ee:ff"
		newNC    = "Swift_new-nc"
		oldNC    = "Swift_old-nc"
		apipaIP  = "169.254.128.4"
	)
	// ncOnMAC is the NC goal state (azure-cns.json) for ncID on the delegated NIC mac.
	ncOnMAC := func(ncID string) map[string]containerstatus {
		return map[string]containerstatus{ncID: {
			ID: ncID,
			CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
				NetworkInterfaceInfo: cns.NetworkInterfaceInfo{MACAddress: mac, NICType: cns.DelegatedVMNIC},
			},
		}}
	}

	tests := []struct {
		name                   string
		endpointState          map[string]*EndpointInfo
		containerStatus        map[string]containerstatus // NC goal state (azure-cns.json)
		ncID                   string                     // incoming NC, on the delegated NIC mac
		apipaIP                string                     // incoming NC's APIPA IP
		hnsErr                 error
		wantErr                bool
		wantRemainingEndpoints int
		wantRemovedKey         string // container removed from EndpointState and its store
		wantDeletedEndpoints   []string
		wantDeletedNetworks    []string
	}{
		{
			name:                   "no-op when endpoint state is empty",
			endpointState:          map[string]*EndpointInfo{},
			ncID:                   newNC,
			wantRemainingEndpoints: 0,
		},
		{
			name: "deletes stale delegated NIC endpoint and network by MAC",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-1", "net-1")}},
			},
			ncID:                   newNC,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
		{
			name: "returns error when HNS endpoint delete fails",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-1", "net-1")}},
			},
			ncID:                   newNC,
			hnsErr:                 errors.New("HNS access denied"),
			wantErr:                true,
			wantRemainingEndpoints: 1,
		},
		{
			name: "no-op when no endpoint matches the MAC",
			endpointState: map[string]*EndpointInfo{
				testContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(otherMAC, "", "")}},
			},
			ncID:                   newNC,
			wantRemainingEndpoints: 1,
		},
		{
			name: "no-op when MAC matches only an InfraNIC",
			endpointState: map[string]*EndpointInfo{
				testContainer: {IfnameToIPMap: map[string]*IPInfo{InfraInterfaceName: {NICType: cns.InfraNIC, MacAddress: mac}}},
			},
			ncID:                   newNC,
			wantRemainingEndpoints: 1,
		},
		{
			name: "matches the MAC ignoring case and hyphens",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo("00-22-48-AB-CD-EF", "ep-1", "net-1")}},
			},
			ncID:                   newNC,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
		{
			name: "keeps a container whose APIPA endpoint is the incoming NC's own",
			endpointState: map[string]*EndpointInfo{
				testContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(otherMAC, "ep-other", "net-other"),
					apipaIfName:     apipaNICInfo(newNC, apipaIP, "apipa-ep"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 1,
		},
		{
			name: "keeps a container whose APIPA endpoint holds another IP",
			endpointState: map[string]*EndpointInfo{
				testContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(otherMAC, "ep-other", "net-other"),
					apipaIfName:     apipaNICInfo(oldNC, "169.254.128.6", "apipa-ep"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 1,
		},
		{
			name: "deletes both the container on the MAC and another container on the APIPA IP",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-1", "net-1")}},
				testContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(otherMAC, "ep-other", "net-other"),
					apipaIfName:     apipaNICInfo(oldNC, apipaIP, "apipa-ep"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 0,
			wantDeletedEndpoints:   []string{"ep-1", "apipa-ep", "ep-other"},
			wantDeletedNetworks:    []string{"net-1", "net-other"},
		},
		{
			name: "cleans up when the MAC was reassigned from another NC",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-1", "net-1")}},
			},
			containerStatus:        ncOnMAC(oldNC),
			ncID:                   newNC,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
		{
			name: "skips cleanup on a repeat create of the same NC on the same MAC",
			endpointState: map[string]*EndpointInfo{
				"live-container": {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-live", "net-live")}},
			},
			containerStatus:        ncOnMAC(newNC),
			ncID:                   newNC,
			wantRemainingEndpoints: 1,
		},
		{
			name: "deletes both stale APIPA and delegated NIC in the same container",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(mac, "ep-delegated", "net-delegated"),
					apipaIfName:     apipaNICInfo(oldNC, apipaIP, "apipa-ep-1"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"apipa-ep-1", "ep-delegated"},
			wantDeletedNetworks:    []string{"net-delegated"},
		},
		{
			name: "deletes the container holding the APIPA IP even when its MAC differs",
			endpointState: map[string]*EndpointInfo{
				testContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(otherMAC, "ep-other", "net-other"),
					apipaIfName:     apipaNICInfo(oldNC, apipaIP, "apipa-ep-2"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         testContainer,
			wantDeletedEndpoints:   []string{"apipa-ep-2", "ep-other"},
			wantDeletedNetworks:    []string{"net-other"},
		},
		{
			name: "returns error on APIPA HNS delete failure without cleaning delegated NIC",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(mac, "ep-delegated", "net-delegated"),
					apipaIfName:     apipaNICInfo(oldNC, apipaIP, "apipa-ep-1"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			hnsErr:                 errors.New("HNS access denied"),
			wantErr:                true,
			wantRemainingEndpoints: 1,
		},
		{
			name: "MAC cleanup also deletes the container's APIPA endpoint on another IP",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{
					delegatedIfName: delegatedNICInfo(mac, "ep-delegated", "net-delegated"),
					apipaIfName:     apipaNICInfo(oldNC, "169.254.128.6", "apipa-ep-orphan"),
				}},
			},
			ncID:                   newNC,
			apipaIP:                apipaIP,
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"apipa-ep-orphan", "ep-delegated"},
			wantDeletedNetworks:    []string{"net-delegated"},
		},
		{
			name: "cleans up delegated NIC when the incoming NC has no APIPA IP",
			endpointState: map[string]*EndpointInfo{
				staleContainer: {IfnameToIPMap: map[string]*IPInfo{delegatedIfName: delegatedNICInfo(mac, "ep-1", "net-1")}},
			},
			ncID:                   newNC,
			apipaIP:                "",
			wantRemainingEndpoints: 0,
			wantRemovedKey:         staleContainer,
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := getTestService(cns.AzureContainerInstance)
			svc.Options[acn.OptManageEndpointState] = true
			svc.EndpointStateStore = store.NewMockStore("")
			svc.EndpointState = tt.endpointState
			require.NoError(t, svc.EndpointStateStore.Write(EndpointStoreKey, svc.EndpointState))
			if tt.containerStatus != nil {
				svc.state.ContainerStatus = tt.containerStatus
			}
			mockClient := &mockHNSClient{err: tt.hnsErr}
			useMockHNSClient(t, mockClient)

			err := svc.cleanupStaleHNSResources(cns.CreateNetworkContainerRequest{
				NetworkContainerid:   tt.ncID,
				NetworkInterfaceInfo: cns.NetworkInterfaceInfo{NICType: cns.DelegatedVMNIC, MACAddress: mac},
				LocalIPConfiguration: cns.IPConfiguration{IPSubnet: cns.IPSubnet{IPAddress: tt.apipaIP}},
			})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, svc.EndpointState, tt.wantRemainingEndpoints)
			assert.ElementsMatch(t, tt.wantDeletedEndpoints, mockClient.deletedEndpointIDs)
			assert.ElementsMatch(t, tt.wantDeletedNetworks, mockClient.deletedNetworkIDs)

			if tt.wantRemovedKey != "" {
				assert.NotContains(t, svc.EndpointState, tt.wantRemovedKey)
				var persisted map[string]*EndpointInfo
				require.NoError(t, svc.EndpointStateStore.Read(EndpointStoreKey, &persisted))
				assert.NotContains(t, persisted, tt.wantRemovedKey)
			}
		})
	}
}

// useMockHNSClient swaps m in as the HNS client for the test.
func useMockHNSClient(t *testing.T, m *mockHNSClient) {
	t.Helper()
	orig := defaultHNSClient
	t.Cleanup(func() { defaultHNSClient = orig })
	defaultHNSClient = m
}

type mockHNSClient struct {
	err                error
	deletedEndpointIDs []string
	deletedNetworkIDs  []string
	endpoints          []hcn.HostComputeEndpoint
	networks           []hcn.HostComputeNetwork
	onCall             func() // runs at the start of every HNS call
}

func (m *mockHNSClient) called() {
	if m.onCall != nil {
		m.onCall()
	}
}

func (m *mockHNSClient) ListNetworks() ([]hcn.HostComputeNetwork, error) {
	m.called()
	return m.networks, m.err
}

func (m *mockHNSClient) ListEndpoints() ([]hcn.HostComputeEndpoint, error) {
	m.called()
	return m.endpoints, m.err
}

func (m *mockHNSClient) DeleteEndpointByID(id string) error {
	m.called()
	if m.err != nil {
		return m.err
	}
	m.deletedEndpointIDs = append(m.deletedEndpointIDs, id)
	return nil
}

func (m *mockHNSClient) DeleteNetworkByID(id string) error {
	m.called()
	if m.err != nil {
		return m.err
	}
	m.deletedNetworkIDs = append(m.deletedNetworkIDs, id)
	return nil
}
