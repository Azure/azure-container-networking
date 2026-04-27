package restserver

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindEndpointStateByMAC(t *testing.T) {
	tests := []struct {
		name            string
		endpointState   map[string]*EndpointInfo
		mac             string
		wantContainerID string
		wantFound       bool
	}{
		{
			name:          "empty state returns nothing",
			endpointState: map[string]*EndpointInfo{},
			mac:           "00:11:22:33:44:55",
		},
		{
			name: "no match returns nothing",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "aa:bb:cc:dd:ee:ff"},
					},
				},
			},
			mac: "00:11:22:33:44:55",
		},
		{
			name: "exact MAC match returns entry",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
			},
			mac:             "00:11:22:33:44:55",
			wantContainerID: "container-1",
			wantFound:       true,
		},
		{
			name: "case-insensitive MAC match",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:AA:BB:CC"},
					},
				},
			},
			mac:             "00:11:22:aa:bb:cc",
			wantContainerID: "container-1",
			wantFound:       true,
		},
		{
			name: "hyphenated MAC input normalized to match colon format",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
			},
			mac:             "00-11-22-33-44-55",
			wantContainerID: "container-1",
			wantFound:       true,
		},
		{
			name: "only FrontendNIC entries are matched, InfraNIC ignored",
			endpointState: map[string]*EndpointInfo{
				"container-infra": {
					PodName: "pod-infra", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.InfraNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
				"container-frontend": {
					PodName: "pod-frontend", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
			},
			mac:             "00:11:22:33:44:55",
			wantContainerID: "container-frontend",
			wantFound:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := getTestService(cns.AzureContainerInstance)
			svc.EndpointState = tt.endpointState
			containerID, ipInfo := svc.findEndpointStateByMAC(tt.mac)
			if tt.wantFound {
				assert.Equal(t, tt.wantContainerID, containerID)
				assert.NotNil(t, ipInfo)
			} else {
				assert.Empty(t, containerID)
				assert.Nil(t, ipInfo)
			}
		})
	}
}

func TestFindStaleContainerByApipaIP(t *testing.T) {
	tests := []struct {
		name            string
		endpointState   map[string]*EndpointInfo
		ncID            string
		apipaIP         string
		wantContainerID string
		wantIfName      string
		wantFound       bool
	}{
		{
			name:          "empty apipaIP returns nil",
			endpointState: map[string]*EndpointInfo{},
			ncID:          "Swift_new-nc",
			apipaIP:       "",
			wantFound:     false,
		},
		{
			name:          "empty endpoint state returns nil",
			endpointState: map[string]*EndpointInfo{},
			ncID:          "Swift_new-nc",
			apipaIP:       "169.254.128.4",
			wantFound:     false,
		},
		{
			name: "skips APIPA belonging to the same NC",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					IfnameToIPMap: map[string]*IPInfo{
						"HostNCApipaEndpoint-Swift_new-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_new-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.4").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-1",
						},
					},
				},
			},
			ncID:      "Swift_new-nc",
			apipaIP:   "169.254.128.4",
			wantFound: false,
		},
		{
			name: "skips APIPA with non-matching IP",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					IfnameToIPMap: map[string]*IPInfo{
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.5").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-1",
						},
					},
				},
			},
			ncID:      "Swift_new-nc",
			apipaIP:   "169.254.128.4",
			wantFound: false,
		},
		{
			name: "returns stale APIPA with matching IP from a different NC",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					IfnameToIPMap: map[string]*IPInfo{
						"Ethernet 4": {
							NICType:       cns.DelegatedVMNIC,
							MacAddress:    "00:22:48:b5:f5:11",
							HnsEndpointID: "ep-delegated",
							HnsNetworkID:  "net-delegated",
						},
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.4").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-1",
						},
					},
				},
			},
			ncID:            "Swift_new-nc",
			apipaIP:         "169.254.128.4",
			wantContainerID: "container-1",
			wantIfName:      "HostNCApipaEndpoint-Swift_old-nc",
			wantFound:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := getTestService(cns.AzureContainerInstance)
			svc.EndpointState = tt.endpointState
			containerID, ifName, ipInfo := svc.findStaleContainerByApipaIP(tt.ncID, tt.apipaIP)
			if tt.wantFound {
				assert.Equal(t, tt.wantContainerID, containerID)
				assert.Equal(t, tt.wantIfName, ifName)
				assert.NotNil(t, ipInfo)
			} else {
				assert.Empty(t, containerID)
				assert.Empty(t, ifName)
				assert.Nil(t, ipInfo)
			}
		})
	}
}

func TestCleanupStaleHNSResources(t *testing.T) {
	tests := []struct {
		name                   string
		endpointState          map[string]*EndpointInfo
		containerStatus        map[string]containerstatus // NC goal state (azure-cns.json)
		mac                    string
		ncID                   string // incoming NC ID for the CreateNC request
		apipaIP                string // incoming APIPA IP for the CreateNC request
		hnsErr                 error  // error to return from mock HNS client
		wantErr                bool
		wantRemainingEndpoints int
		wantRemovedKey         string
		wantDeletedEndpoints   []string
		wantDeletedNetworks    []string
	}{
		{
			name:                   "no-op when endpoint state is empty",
			endpointState:          map[string]*EndpointInfo{},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			wantRemainingEndpoints: 0,
		},
		{
			name: "deletes stale delegated NIC endpoint and network by MAC",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "stale-container",
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
		{
			name: "returns error when HNS endpoint delete fails",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			hnsErr:                 errors.New("HNS access denied"),
			wantErr:                true,
			wantRemainingEndpoints: 1,
		},
		{
			name: "no-op when no endpoint matches the MAC",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "aa:bb:cc:dd:ee:ff"},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			wantRemainingEndpoints: 1,
		},
		{
			name: "no-op when MAC matches only an InfraNIC",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.InfraNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			wantRemainingEndpoints: 1,
		},
		{
			name: "cleans up when NC goal state has a different NC for the same MAC",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth1": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			containerStatus: map[string]containerstatus{
				"Swift_old-nc": {
					ID: "Swift_old-nc",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
							MACAddress: "00:11:22:33:44:55",
							NICType:    cns.DelegatedVMNIC,
						},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc", // different NC ID → MAC was reassigned, cleanup should proceed
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "stale-container",
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
		{
			name: "skips cleanup when NC goal state has the same NC for the MAC",
			endpointState: map[string]*EndpointInfo{
				"live-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth1": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-live", HnsNetworkID: "net-live"},
					},
				},
			},
			containerStatus: map[string]containerstatus{
				"Swift_test-nc": {
					ID: "Swift_test-nc",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
							MACAddress: "00:11:22:33:44:55",
							NICType:    cns.DelegatedVMNIC,
						},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_test-nc", // same NC ID as in NC goal state
			wantRemainingEndpoints: 1,               // endpoint state NOT deleted because same NC+MAC exists
		},
		{
			name: "deletes both stale APIPA and delegated NIC in the same container",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"Ethernet 4": {NICType: cns.DelegatedVMNIC, MacAddress: "00:22:48:b5:f5:11", HnsEndpointID: "ep-delegated", HnsNetworkID: "net-delegated"},
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.4").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-1",
						},
					},
				},
			},
			mac:                    "00:22:48:b5:f5:11",
			ncID:                   "Swift_new-nc",
			apipaIP:                "169.254.128.4",
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "stale-container",
			wantDeletedEndpoints:   []string{"apipa-ep-1", "ep-delegated"},
			wantDeletedNetworks:    []string{"net-delegated"},
		},
		{
			name: "deletes stale APIPA container HNS resources when APIPA IP matches but MAC does not",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"Ethernet 4": {NICType: cns.DelegatedVMNIC, MacAddress: "aa:bb:cc:dd:ee:ff", HnsEndpointID: "ep-other", HnsNetworkID: "net-other"},
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.5").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-2",
						},
					},
				},
			},
			mac:                    "00:22:48:b5:f5:11", // no match for this MAC
			ncID:                   "Swift_new-nc",
			apipaIP:                "169.254.128.5",
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "container-1",
			wantDeletedEndpoints:   []string{"apipa-ep-2", "ep-other"},
			wantDeletedNetworks:    []string{"net-other"},
		},
		{
			name: "returns error on APIPA HNS delete failure without cleaning delegated NIC",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"Ethernet 4": {NICType: cns.DelegatedVMNIC, MacAddress: "00:22:48:b5:f5:11", HnsEndpointID: "ep-delegated", HnsNetworkID: "net-delegated"},
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.4").To4(), Mask: net.CIDRMask(16, 32)}},
							HnsEndpointID:      "apipa-ep-1",
						},
					},
				},
			},
			mac:                    "00:22:48:b5:f5:11",
			ncID:                   "Swift_new-nc",
			apipaIP:                "169.254.128.4",
			hnsErr:                 errors.New("HNS access denied"),
			wantErr:                true,
			wantRemainingEndpoints: 1, // nothing removed
		},
		{
			name: "deletes orphaned APIPA via MAC-based container cleanup when APIPA IP differs",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"Ethernet 4": {NICType: cns.DelegatedVMNIC, MacAddress: "00:22:48:b5:f5:11", HnsEndpointID: "ep-delegated", HnsNetworkID: "net-delegated"},
						"HostNCApipaEndpoint-Swift_old-nc": {
							NICType:            cns.ApipaNIC,
							NetworkContainerID: "Swift_old-nc",
							IPv4:               []net.IPNet{{IP: net.ParseIP("169.254.128.6").To4(), Mask: net.CIDRMask(17, 32)}},
							HnsEndpointID:      "apipa-ep-orphan",
						},
					},
				},
			},
			mac:                    "00:22:48:b5:f5:11",
			ncID:                   "Swift_new-nc",
			apipaIP:                "169.254.128.4", // different from the stale APIPA IP 128.6
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "stale-container",
			wantDeletedEndpoints:   []string{"apipa-ep-orphan", "ep-delegated"},
			wantDeletedNetworks:    []string{"net-delegated"},
		},
		{
			name: "cleans up delegated NIC when no APIPA IP is provided",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			mac:                    "00:11:22:33:44:55",
			ncID:                   "Swift_new-nc",
			apipaIP:                "",
			wantRemainingEndpoints: 0,
			wantRemovedKey:         "stale-container",
			wantDeletedEndpoints:   []string{"ep-1"},
			wantDeletedNetworks:    []string{"net-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := getTestService(cns.AzureContainerInstance)
			svc.EndpointStateStore = store.NewMockStore("")
			svc.EndpointState = tt.endpointState
			require.NoError(t, svc.EndpointStateStore.Write(EndpointStoreKey, svc.EndpointState))

			if tt.containerStatus != nil {
				svc.state.ContainerStatus = tt.containerStatus
			}

			mockClient := &mockHNSClient{err: tt.hnsErr}
			orig := defaultHNSClient
			t.Cleanup(func() { defaultHNSClient = orig })
			defaultHNSClient = mockClient

			err := svc.cleanupStaleHNSResources(tt.ncID, tt.mac, tt.apipaIP)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, svc.EndpointState, tt.wantRemainingEndpoints)

			if tt.wantRemovedKey != "" {
				_, exists := svc.EndpointState[tt.wantRemovedKey]
				assert.False(t, exists, "expected key %s to be removed from EndpointState", tt.wantRemovedKey)

				// Verify state was persisted to the store
				var persisted map[string]*EndpointInfo
				readErr := svc.EndpointStateStore.Read(EndpointStoreKey, &persisted)
				require.NoError(t, readErr)
				_, existsInStore := persisted[tt.wantRemovedKey]
				assert.False(t, existsInStore, "expected key %s to be removed from persisted store", tt.wantRemovedKey)
			}

			if tt.wantDeletedEndpoints != nil {
				assert.ElementsMatch(t, tt.wantDeletedEndpoints, mockClient.deletedEndpointIDs)
			}
			if tt.wantDeletedNetworks != nil {
				assert.ElementsMatch(t, tt.wantDeletedNetworks, mockClient.deletedNetworkIDs)
			}
		})
	}
}

type mockHNSClient struct {
	err                error
	deletedEndpointIDs []string
	deletedNetworkIDs  []string
}

func (m *mockHNSClient) DeleteEndpointByID(id string) error {
	if m.err != nil {
		return m.err
	}
	m.deletedEndpointIDs = append(m.deletedEndpointIDs, id)
	return nil
}

func (m *mockHNSClient) DeleteNetworkByID(id string) error {
	if m.err != nil {
		return m.err
	}
	m.deletedNetworkIDs = append(m.deletedNetworkIDs, id)
	return nil
}
