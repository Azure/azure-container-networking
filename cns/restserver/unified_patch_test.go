// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolterrors "go.etcd.io/bbolt/errors"
)

func TestUnifiedPatchPersistsPlatformDetailsAndReplay(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
	}, nil)
	add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	add.DesiredIPAddresses = []string{"10.0.0.4"}
	_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
	require.NoError(t, err)
	before := requireUnifiedSnapshot(t, db)
	beforeAssignments := before.Assignments
	beforeOwners := before.IPOwners
	beforeIntents := before.DeleteIntents
	refresh := adapter.store.refreshMetrics
	refreshCalls := 0
	adapter.store.refreshMetrics = func(ctx context.Context) (state.Status, error) {
		refreshCalls++
		return refresh(ctx)
	}
	patch := map[string]*IPInfo{
		"eth0": {
			IPv4:               []net.IPNet{testIPNet("10.0.0.4/24")},
			HnsEndpointID:      "hns-endpoint-1",
			HnsNetworkID:       "hns-network-1",
			HostVethName:       "veth-1",
			MacAddress:         "AA-BB-CC-DD-EE-FF",
			NetworkContainerID: "nc-v4",
			NICType:            cns.DelegatedVMNIC,
		},
	}

	response, recorder := invokeEndpointPatch(t, service, context.Background(), "container-1", patch)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, "[updateEndpoint] updateEndpoint retruned successfully", response.Message)
	assert.Equal(t, types.Success.String(), recorder.Header().Get(cnsReturnCode))

	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
	assert.Equal(t, beforeAssignments, after.Assignments)
	assert.Equal(t, beforeOwners, after.IPOwners)
	assert.Equal(t, beforeIntents, after.DeleteIntents)
	info := after.Endpoints["container-1"].IfnameToIPMap["eth0"]
	assert.Equal(t, "hns-endpoint-1", info.HNSEndpointID)
	assert.Equal(t, "hns-network-1", info.HNSNetworkID)
	assert.Equal(t, "veth-1", info.HostVethName)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", info.MACAddress)
	assert.Equal(t, "nc-v4", info.NetworkContainerID)
	assert.Equal(t, cns.DelegatedVMNIC, info.NICType)
	require.Len(t, info.IPv4, 1)
	assert.Equal(t, "10.0.0.4", info.IPv4[0].IP.String())
	cacheInfo := service.EndpointState["container-1"].IfnameToIPMap["eth0"]
	assert.Equal(t, info.HNSEndpointID, cacheInfo.HnsEndpointID)
	assert.Equal(t, info.HNSNetworkID, cacheInfo.HnsNetworkID)
	assert.Equal(t, info.HostVethName, cacheInfo.HostVethName)
	assert.Equal(t, info.MACAddress, cacheInfo.MacAddress)
	assert.Equal(t, info.NetworkContainerID, cacheInfo.NetworkContainerID)
	assert.Equal(t, info.NICType, cacheInfo.NICType)
	generation, projected := adapter.cacheGeneration()
	assert.True(t, projected)
	assert.Equal(t, after.Metadata.Generation, generation)
	assert.Equal(t, 1, refreshCalls)

	response, _ = invokeEndpointPatch(t, service, context.Background(), "container-1", patch)
	assert.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, after, requireUnifiedSnapshot(t, db))
	assert.Equal(t, 1, refreshCalls)
}

func TestUnifiedPatchMultiNICConcurrencyPreservesAssignments(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {
			{ID: "ip-primary", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1},
			{ID: "ip-secondary", IPAddress: "10.0.0.5", NCID: "nc-v4", NCVersion: 1},
		},
		"nc-v6": {{ID: "ip-v6", IPAddress: "2001:db8::4", NCID: "nc-v6", NCVersion: 1}},
	}, nil)
	primary := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	primary.DesiredIPAddresses = []string{"10.0.0.4", "2001:db8::4"}
	secondary := unifiedAddRequest("container-1", "interface-2", "net1", "pod-1", "namespace-1")
	secondary.DesiredIPAddresses = []string{"10.0.0.5"}
	secondary.SecondaryInterfacesExist = true
	_, err := service.requestIPConfigHandlerHelper(context.Background(), primary)
	require.NoError(t, err)
	_, err = service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	before := requireUnifiedSnapshot(t, db)

	patches := []map[string]*IPInfo{
		{"eth0": {
			HnsEndpointID: "primary-hns",
			IPv4:          []net.IPNet{testIPNet("10.0.0.4/24")},
			IPv6:          []net.IPNet{testIPNet("2001:db8::4/64")},
		}},
		{"net1": {
			HostVethName: "secondary-veth",
			MacAddress:   "00:11:22:33:44:66",
			IPv4:         []net.IPNet{testIPNet("10.0.0.5/24")},
		}},
	}
	start := make(chan struct{})
	results := make(chan cns.Response, len(patches))
	var ready sync.WaitGroup
	ready.Add(len(patches))
	for _, patch := range patches {
		go func() {
			ready.Done()
			<-start
			response, _ := invokeEndpointPatch(t, service, context.Background(), "container-1", patch)
			results <- response
		}()
	}
	ready.Wait()
	close(start)
	for range patches {
		assert.Equal(t, types.Success, (<-results).ReturnCode)
	}

	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+2, after.Metadata.Generation)
	assert.Equal(t, before.Assignments, after.Assignments)
	assert.Equal(t, before.IPOwners, after.IPOwners)
	assert.Equal(t, before.DeleteIntents, after.DeleteIntents)
	assert.Equal(t, "primary-hns", after.Endpoints["container-1"].IfnameToIPMap["eth0"].HNSEndpointID)
	assert.Equal(t, "secondary-veth", after.Endpoints["container-1"].IfnameToIPMap["net1"].HostVethName)
	require.Len(t, after.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4, 1)
	require.Len(t, after.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv6, 1)
	require.Len(t, after.Endpoints["container-1"].IfnameToIPMap["net1"].IPv4, 1)

	duplicate := map[string]*IPInfo{"net1": {
		HnsNetworkID: "network-once",
		NICType:      cns.InfraNIC,
	}}
	start = make(chan struct{})
	for range 2 {
		go func() {
			<-start
			response, _ := invokeEndpointPatch(t, service, context.Background(), "container-1", duplicate)
			results <- response
		}()
	}
	close(start)
	assert.Equal(t, types.Success, (<-results).ReturnCode)
	assert.Equal(t, types.Success, (<-results).ReturnCode)
	replayed := requireUnifiedSnapshot(t, db)
	assert.Equal(t, after.Metadata.Generation+1, replayed.Metadata.Generation)
	assert.Equal(t, "network-once", replayed.Endpoints["container-1"].IfnameToIPMap["net1"].HNSNetworkID)
	generation, _ := adapter.cacheGeneration()
	assert.Equal(t, replayed.Metadata.Generation, generation)
}

func TestUnifiedPatchValidationFailuresAreAtomic(t *testing.T) {
	tests := []struct {
		name       string
		endpointID string
		request    map[string]*IPInfo
		wantCode   types.ResponseCode
	}{
		{
			name:       "absent endpoint",
			endpointID: "missing",
			request:    map[string]*IPInfo{"eth0": {HostVethName: "veth"}},
			wantCode:   types.NotFound,
		},
		{
			name:       "mismatched interface",
			endpointID: "container-1",
			request:    map[string]*IPInfo{"net1": {HostVethName: "veth"}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "assigned IP change",
			endpointID: "container-1",
			request: map[string]*IPInfo{"eth0": {
				HostVethName: "veth",
				IPv4:         []net.IPNet{testIPNet("10.0.0.99/24")},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "malformed prefix",
			endpointID: "container-1",
			request: map[string]*IPInfo{"eth0": {
				HostVethName: "veth",
				IPv4: []net.IPNet{{
					IP: net.ParseIP("10.0.0.4"), Mask: net.IPMask{255, 0, 255, 0},
				}},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "wrong prefix family",
			endpointID: "container-1",
			request: map[string]*IPInfo{"eth0": {
				HostVethName: "veth",
				IPv4:         []net.IPNet{testIPNet("2001:db8::4/64")},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "malformed MAC",
			endpointID: "container-1",
			request:    map[string]*IPInfo{"eth0": {MacAddress: "not-a-mac"}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "missing network container",
			endpointID: "container-1",
			request: map[string]*IPInfo{"eth0": {
				HostVethName:       "veth",
				NetworkContainerID: "missing-nc",
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "invalid NIC type",
			endpointID: "container-1",
			request:    map[string]*IPInfo{"eth0": {NICType: cns.NICType("invalid")}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "null interface",
			endpointID: "container-1",
			request:    map[string]*IPInfo{"eth0": nil},
			wantCode:   types.InvalidRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
				"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
			}, nil)
			add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
			add.DesiredIPAddresses = []string{"10.0.0.4"}
			_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
			require.NoError(t, err)
			before := requireUnifiedSnapshot(t, db)
			beforeCache := durableCacheFingerprint(service, adapter)

			response, _ := invokeEndpointPatch(t, service, context.Background(), tt.endpointID, tt.request)
			assert.Equal(t, tt.wantCode, response.ReturnCode)
			assert.Equal(t, before, requireUnifiedSnapshot(t, db))
			assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		})
	}
}

func TestUnifiedPatchDeleteOrderingAndExpiry(t *testing.T) {
	t.Run("patch then delete retains details", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
		add.DesiredIPAddresses = []string{"10.0.0.4"}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		response, _ := invokeEndpointPatch(
			t,
			service,
			context.Background(),
			"container-1",
			map[string]*IPInfo{"eth0": {HostVethName: "patched-before-delete"}},
		)
		require.Equal(t, types.Success, response.ReturnCode)
		now := time.Date(2026, time.July, 24, 7, 0, 0, 0, time.UTC)
		adapter.now = func() time.Time { return now }
		_, err = service.ReleaseIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(t, "patched-before-delete", after.Endpoints["container-1"].IfnameToIPMap["eth0"].HostVethName)
		assert.Contains(t, after.DeleteIntents, "container-1")
		assert.Empty(t, after.Assignments)
		assert.Empty(t, after.IPOwners)
	})

	t.Run("delete then patch is blocked through expiry boundary", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
		add.DesiredIPAddresses = []string{"10.0.0.4"}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		now := time.Date(2026, time.July, 24, 8, 0, 0, 0, time.UTC)
		adapter.now = func() time.Time { return now }
		_, err = service.ReleaseIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		patch := map[string]*IPInfo{"eth0": {HostVethName: "must-not-apply"}}

		response, _ := invokeEndpointPatch(t, service, context.Background(), "container-1", patch)
		assert.Equal(t, types.UnexpectedError, response.ReturnCode)
		assert.Equal(t, before, requireUnifiedSnapshot(t, db))

		adapter.now = func() time.Time { return now.Add(unifiedDeleteIntentTTL) }
		response, _ = invokeEndpointPatch(t, service, context.Background(), "container-1", patch)
		assert.Equal(t, types.NotFound, response.ReturnCode)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(t, before, after)
		assert.Equal(t, now, after.DeleteIntents["container-1"].CreatedAt)
	})
}

func TestUnifiedPatchFailuresAndPostcommitRestore(t *testing.T) {
	injected := errors.New("injected patch failure")
	tests := []struct {
		name     string
		inject   func(*durableStateAdapter)
		ctx      func() context.Context
		wantCode types.ResponseCode
	}{
		{
			name: "commit",
			inject: func(adapter *durableStateAdapter) {
				adapter.store.patchEndpoint = func(
					context.Context,
					uint64,
					state.PodIdentity,
					state.EndpointRecord,
					time.Time,
					time.Duration,
					func(state.Snapshot) error,
				) (bool, error) {
					return false, injected
				}
			},
			wantCode: types.UnexpectedError,
		},
		{
			name: "stale generation",
			inject: func(adapter *durableStateAdapter) {
				adapter.store.patchEndpoint = func(
					context.Context,
					uint64,
					state.PodIdentity,
					state.EndpointRecord,
					time.Time,
					time.Duration,
					func(state.Snapshot) error,
				) (bool, error) {
					return false, state.ErrStaleGeneration
				}
			},
			wantCode: types.InconsistentIPConfigState,
		},
		{
			name: "candidate callback",
			inject: func(adapter *durableStateAdapter) {
				adapter.buildProjection = func(state.Snapshot) (durableCacheProjection, error) {
					return durableCacheProjection{}, injected
				}
			},
			wantCode: types.UnexpectedError,
		},
		{
			name:   "canceled",
			inject: func(*durableStateAdapter) {},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantCode: types.UnexpectedError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
				"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
			}, nil)
			add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
			add.DesiredIPAddresses = []string{"10.0.0.4"}
			_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
			require.NoError(t, err)
			before := requireUnifiedSnapshot(t, db)
			beforeCache := durableCacheFingerprint(service, adapter)
			tt.inject(adapter)
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}

			response, _ := invokeEndpointPatch(
				t,
				service,
				ctx,
				"container-1",
				map[string]*IPInfo{"eth0": {HostVethName: "patched"}},
			)
			assert.Equal(t, tt.wantCode, response.ReturnCode)
			assert.Equal(t, before, requireUnifiedSnapshot(t, db))
			assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		})
	}

	t.Run("cache does not match database", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
		add.DesiredIPAddresses = []string{"10.0.0.4"}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		service.EndpointState["container-1"].IfnameToIPMap["eth0"].HostVethName = "stale-cache-value"
		beforeCache := durableCacheFingerprint(service, adapter)

		response, _ := invokeEndpointPatch(
			t,
			service,
			context.Background(),
			"container-1",
			map[string]*IPInfo{"eth0": {HostVethName: "patched"}},
		)
		assert.Equal(t, types.InconsistentIPConfigState, response.ReturnCode)
		assert.Equal(t, before, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("postcommit cache restore", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
		add.DesiredIPAddresses = []string{"10.0.0.4"}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		adapter.applyPatchProjection = func(durableCacheProjection) error { return injected }

		response, _ := invokeEndpointPatch(
			t,
			service,
			context.Background(),
			"container-1",
			map[string]*IPInfo{"eth0": {HostVethName: "committed"}},
		)
		assert.Equal(t, types.UnexpectedError, response.ReturnCode)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
		assert.Equal(t, "committed", after.Endpoints["container-1"].IfnameToIPMap["eth0"].HostVethName)
		assert.Equal(t, "committed", service.EndpointState["container-1"].IfnameToIPMap["eth0"].HostVethName)
		generation, projected := adapter.cacheGeneration()
		assert.True(t, projected)
		assert.Equal(t, after.Metadata.Generation, generation)
	})
}

func TestUnifiedPatchReadOnlyClosedAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	_, err = db.ApplyNetworkContainer(context.Background(), r18NetworkContainer("nc-v4"), []state.IPRecord{{
		ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1,
	}})
	require.NoError(t, err)
	service := newUnifiedAddTestService(t)
	restore, closeState, err := NewDurableStateLifecycle(service, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	add := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	add.DesiredIPAddresses = []string{"10.0.0.4"}
	_, err = service.requestIPConfigHandlerHelper(context.Background(), add)
	require.NoError(t, err)
	response, _ := invokeEndpointPatch(
		t,
		service,
		context.Background(),
		"container-1",
		map[string]*IPInfo{"eth0": {
			HnsEndpointID: "restart-hns",
			HnsNetworkID:  "restart-network",
			HostVethName:  "restart-veth",
			MacAddress:    "00:11:22:33:44:77",
			NICType:       cns.ApipaNIC,
		}},
	)
	require.Equal(t, types.Success, response.ReturnCode)
	require.NoError(t, closeState())

	response, _ = invokeEndpointPatch(
		t,
		service,
		context.Background(),
		"container-1",
		map[string]*IPInfo{"eth0": {HostVethName: "closed"}},
	)
	assert.Equal(t, types.UnexpectedError, response.ReturnCode)

	reopened, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	restarted := newUnifiedAddTestService(t)
	restore, closeRestarted, err := NewDurableStateLifecycle(restarted, reopened, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeRestarted()) })
	info := restarted.EndpointState["container-1"].IfnameToIPMap["eth0"]
	assert.Equal(t, "restart-hns", info.HnsEndpointID)
	assert.Equal(t, "restart-network", info.HnsNetworkID)
	assert.Equal(t, "restart-veth", info.HostVethName)
	assert.Equal(t, "00:11:22:33:44:77", info.MacAddress)
	assert.Equal(t, cns.ApipaNIC, info.NICType)

	require.NoError(t, closeRestarted())
	readOnly, err := state.Open(path, state.Options{ReadOnly: true})
	require.NoError(t, err)
	readOnlyService := newUnifiedAddTestService(t)
	restore, closeReadOnly, err := NewDurableStateLifecycle(readOnlyService, readOnly, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeReadOnly()) })
	response, _ = invokeEndpointPatch(
		t,
		readOnlyService,
		context.Background(),
		"container-1",
		map[string]*IPInfo{"eth0": {HostVethName: "read-only"}},
	)
	assert.Equal(t, types.UnexpectedError, response.ReturnCode)
	assert.Equal(t, "restart-veth", readOnlyService.EndpointState["container-1"].IfnameToIPMap["eth0"].HostVethName)
}

func TestUnifiedPatchImportedEndpointPreservesOwnership(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "state.db"), state.Options{})
	require.NoError(t, err)
	_, err = db.ApplyNetworkContainer(context.Background(), r18NetworkContainer("nc-v4"), []state.IPRecord{{
		ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1,
	}})
	require.NoError(t, err)
	records := []cns.CNIEndpointState{{
		InfraContainerID: "container-1",
		PodEndpointID:    "interface-1",
		PodName:          "pod-1",
		PodNamespace:     "namespace-1",
		InterfaceKey:     "container-1-eth0",
		InterfaceName:    "eth0",
		IPAddresses:      []net.IPNet{testIPNet("10.0.0.4/24")},
	}}
	plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
	require.NoError(t, err)
	changed, err := db.ImportCNIEndpointState(context.Background(), records, plan)
	require.NoError(t, err)
	require.True(t, changed)
	before := requireUnifiedSnapshot(t, db)
	service := newUnifiedAddTestService(t)
	restore, closeState, err := NewDurableStateLifecycle(service, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeState()) })

	response, _ := invokeEndpointPatch(
		t,
		service,
		context.Background(),
		"container-1",
		map[string]*IPInfo{"eth0": {
			HnsEndpointID:      "imported-hns",
			NetworkContainerID: "nc-v4",
			NICType:            cns.InfraNIC,
		}},
	)
	require.Equal(t, types.Success, response.ReturnCode)
	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Assignments, after.Assignments)
	assert.Equal(t, before.IPOwners, after.IPOwners)
	assert.Equal(t, before.DeleteIntents, after.DeleteIntents)
	assert.Equal(t, "imported-hns", after.Endpoints["container-1"].IfnameToIPMap["eth0"].HNSEndpointID)
	assert.Equal(t, "nc-v4", after.Endpoints["container-1"].IfnameToIPMap["eth0"].NetworkContainerID)
}

func TestJSONPatchPathRemainsIsolated(t *testing.T) {
	service := getTestService(cns.KubernetesCRD)
	enableManagedEndpointState(service)
	service.EndpointState["container-1"] = &EndpointInfo{
		PodName:      "pod-1",
		PodNamespace: "namespace-1",
		IfnameToIPMap: map[string]*IPInfo{
			"eth0": {IPv4: []net.IPNet{testIPNet("10.0.0.4/24")}},
		},
	}
	require.NoError(t, service.EndpointStateStore.Write(EndpointStoreKey, service.EndpointState))
	require.Nil(t, service.selectedUnifiedStateAdapter())

	response, _ := invokeEndpointPatch(
		t,
		service,
		context.Background(),
		"container-1",
		map[string]*IPInfo{"eth0": {HostVethName: "json-veth"}},
	)
	require.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, "json-veth", service.EndpointState["container-1"].IfnameToIPMap["eth0"].HostVethName)
	var persisted map[string]*EndpointInfo
	require.NoError(t, service.EndpointStateStore.Read(EndpointStoreKey, &persisted))
	assert.Equal(t, "json-veth", persisted["container-1"].IfnameToIPMap["eth0"].HostVethName)
	assert.Nil(t, service.selectedUnifiedStateAdapter())
}

func invokeEndpointPatch(
	t *testing.T,
	service *HTTPRestService,
	ctx context.Context,
	endpointID string,
	request map[string]*IPInfo,
) (cns.Response, *httptest.ResponseRecorder) {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	httpRequest := httptest.NewRequest(
		http.MethodPatch,
		cns.EndpointPath+endpointID,
		bytes.NewReader(body),
	).WithContext(ctx)
	recorder := httptest.NewRecorder()
	service.EndpointHandlerAPI(recorder, httpRequest)
	var response cns.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response, recorder
}

func TestUnifiedPatchReadOnlyErrorIdentity(t *testing.T) {
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(bolterrors.ErrDatabaseReadOnly))
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(context.Canceled))
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(state.ErrDeleteIntent))
}
