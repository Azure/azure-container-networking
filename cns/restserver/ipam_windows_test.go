package restserver

import (
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

func TestCleanupStaleHNSResources(t *testing.T) {
	tests := []struct {
		name            string
		endpointState   map[string]*EndpointInfo
		containerStatus map[string]containerstatus // NC goal state (azure-cns.json)
		mac             string
		ncID            string // incoming NC ID for the CreateNC request
		hnsErr          error  // error to return from mock HNS client
		wantErr         bool
		wantRemaining   int
		wantRemovedKey  string
	}{
		{
			name:          "no stale entries",
			endpointState: map[string]*EndpointInfo{},
			mac:           "00:11:22:33:44:55",
			ncID:          "Swift_new-nc",
			wantRemaining: 0,
		},
		{
			name: "match found, cleanup succeeds",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			mac:            "00:11:22:33:44:55",
			ncID:           "Swift_new-nc",
			wantRemaining:  0,
			wantRemovedKey: "stale-container",
		},
		{
			name: "match found, HNS delete fails",
			endpointState: map[string]*EndpointInfo{
				"stale-container": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "00:11:22:33:44:55", HnsEndpointID: "ep-1", HnsNetworkID: "net-1"},
					},
				},
			},
			mac:           "00:11:22:33:44:55",
			ncID:          "Swift_new-nc",
			hnsErr:        errors.New("HNS access denied"),
			wantErr:       true,
			wantRemaining: 1,
		},
		{
			name: "no match, different MAC",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.DelegatedVMNIC, MacAddress: "aa:bb:cc:dd:ee:ff"},
					},
				},
			},
			mac:           "00:11:22:33:44:55",
			ncID:          "Swift_new-nc",
			wantRemaining: 1,
		},
		{
			name: "no match, wrong NIC type",
			endpointState: map[string]*EndpointInfo{
				"container-1": {
					PodName: "pod1", PodNamespace: "ns1",
					IfnameToIPMap: map[string]*IPInfo{
						"eth0": {NICType: cns.InfraNIC, MacAddress: "00:11:22:33:44:55"},
					},
				},
			},
			mac:           "00:11:22:33:44:55",
			ncID:          "Swift_new-nc",
			wantRemaining: 1,
		},
		{
			name: "different NC ID exists in NC goal state for the same MAC, cleanup proceeds",
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
			mac:            "00:11:22:33:44:55",
			ncID:           "Swift_new-nc", // different NC ID → MAC was reassigned, cleanup should proceed
			wantRemaining:  0,
			wantRemovedKey: "stale-container",
		},
		{
			name: "same NC ID exists in NC goal state for the same MAC, skip cleanup",
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
			mac:           "00:11:22:33:44:55",
			ncID:          "Swift_test-nc", // same NC ID as in NC goal state
			wantRemaining: 1,               // endpoint state NOT deleted because same NC+MAC exists
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

			err := svc.cleanupStaleHNSResources(tt.ncID, tt.mac)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, svc.EndpointState, tt.wantRemaining)

			if tt.wantRemovedKey != "" {
				_, exists := svc.EndpointState[tt.wantRemovedKey]
				assert.False(t, exists, "expected key %s to be removed from EndpointState", tt.wantRemovedKey)

				// Verify state was persisted to the store
				var persisted map[string]*EndpointInfo
				readErr := svc.EndpointStateStore.Read(EndpointStoreKey, &persisted)
				require.NoError(t, readErr)
				_, existsInStore := persisted[tt.wantRemovedKey]
				assert.False(t, existsInStore, "expected key %s to be removed from persisted store", tt.wantRemovedKey)

				// Verify mock was called with correct IDs
				assert.Equal(t, []string{"ep-1"}, mockClient.deletedEndpointIDs)
				assert.Equal(t, []string{"net-1"}, mockClient.deletedNetworkIDs)
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
