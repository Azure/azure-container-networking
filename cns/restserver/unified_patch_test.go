// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var errInjectedPatchFailure = errors.New("injected patch failure")

const (
	patchTestSecondaryInterface = "net1"
	patchTestVeth               = "veth"
	patchTestCommit             = "commit"
)

func TestUnifiedPatchPersistsPlatformDetailsAndReplay(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
	}, nil)
	add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	add.DesiredIPAddresses = []string{delTestIPv4Address}
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
		InfraInterfaceName: {
			IPv4:               []net.IPNet{testIPNet(delTestIPv4Address + "/24")},
			HnsEndpointID:      "hns-endpoint-1",
			HnsNetworkID:       "hns-network-1",
			HostVethName:       "veth-1",
			MacAddress:         "AA-BB-CC-DD-EE-FF",
			NetworkContainerID: delTestNCIPv4,
			NICType:            cns.DelegatedVMNIC,
		},
	}

	response, recorder, err := invokeEndpointPatch(context.Background(), service, delTestContainerID, patch)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, "[updateEndpoint] updateEndpoint retruned successfully", response.Message)
	assert.Equal(t, types.Success.String(), recorder.Header().Get(cnsReturnCode))

	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
	assert.Equal(t, beforeAssignments, after.Assignments)
	assert.Equal(t, beforeOwners, after.IPOwners)
	assert.Equal(t, beforeIntents, after.DeleteIntents)
	info := after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName]
	assert.Equal(t, "hns-endpoint-1", info.HNSEndpointID)
	assert.Equal(t, "hns-network-1", info.HNSNetworkID)
	assert.Equal(t, "veth-1", info.HostVethName)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", info.MACAddress)
	assert.Equal(t, delTestNCIPv4, info.NetworkContainerID)
	assert.Equal(t, cns.DelegatedVMNIC, info.NICType)
	require.Len(t, info.IPv4, 1)
	assert.Equal(t, delTestIPv4Address, info.IPv4[0].IP.String())
	cacheInfo := service.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName]
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

	response, _, err = invokeEndpointPatch(context.Background(), service, delTestContainerID, patch)
	require.NoError(t, err)
	assert.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, after, requireUnifiedSnapshot(t, db))
	assert.Equal(t, 1, refreshCalls)
}

func TestUnifiedPatchMultiNICConcurrencyPreservesAssignments(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {
			{ID: "ip-primary", IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1},
			{ID: "ip-secondary", IPAddress: primaryIP, NCID: delTestNCIPv4, NCVersion: 1},
		},
		delTestNCIPv6: {{ID: delTestIPv6ID, IPAddress: delTestIPv6Address, NCID: delTestNCIPv6, NCVersion: 1}},
	}, nil)
	primary := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	primary.DesiredIPAddresses = []string{delTestIPv4Address, delTestIPv6Address}
	secondary := unifiedAddRequest(delTestContainerID, "interface-2", patchTestSecondaryInterface, delTestPodName, delTestNamespace)
	secondary.DesiredIPAddresses = []string{primaryIP}
	secondary.SecondaryInterfacesExist = true
	_, err := service.requestIPConfigHandlerHelper(context.Background(), primary)
	require.NoError(t, err)
	_, err = service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	before := requireUnifiedSnapshot(t, db)

	patches := []map[string]*IPInfo{
		{InfraInterfaceName: {
			HnsEndpointID: "primary-hns",
			IPv4:          []net.IPNet{testIPNet(delTestIPv4Address + "/24")},
			IPv6:          []net.IPNet{testIPNet(delTestIPv6Address + "/64")},
		}},
		{patchTestSecondaryInterface: {
			HostVethName: "secondary-veth",
			MacAddress:   "00:11:22:33:44:66",
			IPv4:         []net.IPNet{testIPNet(primaryIP + "/24")},
		}},
	}
	type patchResult struct {
		response cns.Response
		err      error
	}
	start := make(chan struct{})
	results := make(chan patchResult, len(patches))
	var ready sync.WaitGroup
	ready.Add(len(patches))
	for _, patch := range patches {
		go func() {
			ready.Done()
			<-start
			response, _, err := invokeEndpointPatch(context.Background(), service, delTestContainerID, patch)
			results <- patchResult{response: response, err: err}
		}()
	}
	ready.Wait()
	close(start)
	for range patches {
		result := <-results
		require.NoError(t, result.err)
		assert.Equal(t, types.Success, result.response.ReturnCode)
	}

	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+2, after.Metadata.Generation)
	assert.Equal(t, before.Assignments, after.Assignments)
	assert.Equal(t, before.IPOwners, after.IPOwners)
	assert.Equal(t, before.DeleteIntents, after.DeleteIntents)
	assert.Equal(t, "primary-hns", after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HNSEndpointID)
	assert.Equal(t, "secondary-veth", after.Endpoints[delTestContainerID].IfnameToIPMap[patchTestSecondaryInterface].HostVethName)
	require.Len(t, after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName].IPv4, 1)
	require.Len(t, after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName].IPv6, 1)
	require.Len(t, after.Endpoints[delTestContainerID].IfnameToIPMap[patchTestSecondaryInterface].IPv4, 1)

	duplicate := map[string]*IPInfo{patchTestSecondaryInterface: {
		HnsNetworkID: "network-once",
		NICType:      cns.InfraNIC,
	}}
	start = make(chan struct{})
	for range 2 {
		go func() {
			<-start
			response, _, err := invokeEndpointPatch(context.Background(), service, delTestContainerID, duplicate)
			results <- patchResult{response: response, err: err}
		}()
	}
	close(start)
	first := <-results
	require.NoError(t, first.err)
	assert.Equal(t, types.Success, first.response.ReturnCode)
	second := <-results
	require.NoError(t, second.err)
	assert.Equal(t, types.Success, second.response.ReturnCode)
	replayed := requireUnifiedSnapshot(t, db)
	assert.Equal(t, after.Metadata.Generation+1, replayed.Metadata.Generation)
	assert.Equal(t, "network-once", replayed.Endpoints[delTestContainerID].IfnameToIPMap[patchTestSecondaryInterface].HNSNetworkID)
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
			request:    map[string]*IPInfo{InfraInterfaceName: {HostVethName: patchTestVeth}},
			wantCode:   types.NotFound,
		},
		{
			name:       "mismatched interface",
			endpointID: delTestContainerID,
			request:    map[string]*IPInfo{patchTestSecondaryInterface: {HostVethName: patchTestVeth}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "assigned IP change",
			endpointID: delTestContainerID,
			request: map[string]*IPInfo{InfraInterfaceName: {
				HostVethName: patchTestVeth,
				IPv4:         []net.IPNet{testIPNet("10.0.0.99/24")},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "malformed prefix",
			endpointID: delTestContainerID,
			request: map[string]*IPInfo{InfraInterfaceName: {
				HostVethName: patchTestVeth,
				IPv4: []net.IPNet{{
					IP: net.ParseIP(delTestIPv4Address), Mask: net.IPMask{255, 0, 255, 0},
				}},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "wrong prefix family",
			endpointID: delTestContainerID,
			request: map[string]*IPInfo{InfraInterfaceName: {
				HostVethName: patchTestVeth,
				IPv4:         []net.IPNet{testIPNet(delTestIPv6Address + "/64")},
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "malformed MAC",
			endpointID: delTestContainerID,
			request:    map[string]*IPInfo{InfraInterfaceName: {MacAddress: "not-a-mac"}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "missing network container",
			endpointID: delTestContainerID,
			request: map[string]*IPInfo{InfraInterfaceName: {
				HostVethName:       patchTestVeth,
				NetworkContainerID: "missing-nc",
			}},
			wantCode: types.InvalidRequest,
		},
		{
			name:       "invalid NIC type",
			endpointID: delTestContainerID,
			request:    map[string]*IPInfo{InfraInterfaceName: {NICType: cns.NICType("invalid")}},
			wantCode:   types.InvalidRequest,
		},
		{
			name:       "null interface",
			endpointID: delTestContainerID,
			request:    map[string]*IPInfo{InfraInterfaceName: nil},
			wantCode:   types.InvalidRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
				delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
			}, nil)
			add := unifiedAddRequest(
				delTestContainerID,
				delTestInterfaceID,
				InfraInterfaceName,
				delTestPodName,
				delTestNamespace,
			)
			add.DesiredIPAddresses = []string{delTestIPv4Address}
			_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
			require.NoError(t, err)
			before := requireUnifiedSnapshot(t, db)
			beforeCache := durableCacheFingerprint(service, adapter)

			response, _, err := invokeEndpointPatch(context.Background(), service, tt.endpointID, tt.request)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, response.ReturnCode)
			assert.Equal(t, before, requireUnifiedSnapshot(t, db))
			assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		})
	}
}

func TestUnifiedPatchDeleteOrderingAndExpiry(t *testing.T) {
	t.Run("patch then delete retains details", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		add.DesiredIPAddresses = []string{delTestIPv4Address}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		response, _, err := invokeEndpointPatch(
			context.Background(),
			service,
			delTestContainerID,
			map[string]*IPInfo{InfraInterfaceName: {HostVethName: "patched-before-delete"}},
		)
		require.NoError(t, err)
		require.Equal(t, types.Success, response.ReturnCode)
		now := time.Date(2026, time.July, 24, 7, 0, 0, 0, time.UTC)
		adapter.now = func() time.Time { return now }
		_, err = service.ReleaseIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(
			t,
			"patched-before-delete",
			after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName,
		)
		assert.Contains(t, after.DeleteIntents, delTestContainerID)
		assert.Empty(t, after.Assignments)
		assert.Empty(t, after.IPOwners)
	})

	t.Run("delete then patch is blocked through expiry boundary", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		add.DesiredIPAddresses = []string{delTestIPv4Address}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		now := time.Date(2026, time.July, 24, 8, 0, 0, 0, time.UTC)
		adapter.now = func() time.Time { return now }
		_, err = service.ReleaseIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		patch := map[string]*IPInfo{InfraInterfaceName: {HostVethName: "must-not-apply"}}

		response, _, err := invokeEndpointPatch(context.Background(), service, delTestContainerID, patch)
		require.NoError(t, err)
		assert.Equal(t, types.UnexpectedError, response.ReturnCode)
		assert.Equal(t, before, requireUnifiedSnapshot(t, db))

		adapter.now = func() time.Time { return now.Add(unifiedDeleteIntentTTL) }
		response, _, err = invokeEndpointPatch(context.Background(), service, delTestContainerID, patch)
		require.NoError(t, err)
		assert.Equal(t, types.NotFound, response.ReturnCode)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(t, before, after)
		assert.Equal(t, now, after.DeleteIntents[delTestContainerID].CreatedAt)
	})
}

func TestUnifiedPatchFailuresAndPostcommitRestore(t *testing.T) {
	tests := []struct {
		name     string
		inject   func(*durableStateAdapter)
		ctx      func() context.Context
		wantCode types.ResponseCode
	}{
		{
			name: patchTestCommit,
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
					return false, errInjectedPatchFailure
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
					return durableCacheProjection{}, errInjectedPatchFailure
				}
			},
			wantCode: types.UnexpectedError,
		},
		{
			name:   delTestCanceled,
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
				delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
			}, nil)
			add := unifiedAddRequest(
				delTestContainerID,
				delTestInterfaceID,
				InfraInterfaceName,
				delTestPodName,
				delTestNamespace,
			)
			add.DesiredIPAddresses = []string{delTestIPv4Address}
			_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
			require.NoError(t, err)
			before := requireUnifiedSnapshot(t, db)
			beforeCache := durableCacheFingerprint(service, adapter)
			tt.inject(adapter)
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}

			response, _, err := invokeEndpointPatch(
				ctx,
				service,
				delTestContainerID,
				map[string]*IPInfo{InfraInterfaceName: {HostVethName: "patched"}},
			)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, response.ReturnCode)
			assert.Equal(t, before, requireUnifiedSnapshot(t, db))
			assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		})
	}

	t.Run("cache does not match database", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		add.DesiredIPAddresses = []string{delTestIPv4Address}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		service.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName = "stale-cache-value"
		beforeCache := durableCacheFingerprint(service, adapter)

		response, _, err := invokeEndpointPatch(
			context.Background(),
			service,
			delTestContainerID,
			map[string]*IPInfo{InfraInterfaceName: {HostVethName: "patched"}},
		)
		require.NoError(t, err)
		assert.Equal(t, types.InconsistentIPConfigState, response.ReturnCode)
		assert.Equal(t, before, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("postcommit cache restore", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		add.DesiredIPAddresses = []string{delTestIPv4Address}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), add)
		require.NoError(t, err)
		before := requireUnifiedSnapshot(t, db)
		adapter.applyPatchProjection = func(durableCacheProjection) error { return errInjectedPatchFailure }

		response, _, err := invokeEndpointPatch(
			context.Background(),
			service,
			delTestContainerID,
			map[string]*IPInfo{InfraInterfaceName: {HostVethName: "committed"}},
		)
		require.NoError(t, err)
		assert.Equal(t, types.UnexpectedError, response.ReturnCode)
		after := requireUnifiedSnapshot(t, db)
		assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
		assert.Equal(t, "committed", after.Endpoints[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName)
		assert.Equal(t, "committed", service.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName)
		generation, projected := adapter.cacheGeneration()
		assert.True(t, projected)
		assert.Equal(t, after.Metadata.Generation, generation)
	})
}

func TestUnifiedPatchReadOnlyClosedAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	_, err = db.ApplyNetworkContainer(context.Background(), r18NetworkContainer(delTestNCIPv4), []state.IPRecord{{
		ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1,
	}})
	require.NoError(t, err)
	service := newUnifiedAddTestService(t)
	restore, closeState, err := NewDurableStateLifecycle(service, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	add := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	add.DesiredIPAddresses = []string{delTestIPv4Address}
	_, err = service.requestIPConfigHandlerHelper(context.Background(), add)
	require.NoError(t, err)
	response, _, err := invokeEndpointPatch(
		context.Background(),
		service,
		delTestContainerID,
		map[string]*IPInfo{InfraInterfaceName: {
			HnsEndpointID: "restart-hns",
			HnsNetworkID:  "restart-network",
			HostVethName:  "restart-veth",
			MacAddress:    "00:11:22:33:44:77",
			NICType:       cns.ApipaNIC,
		}},
	)
	require.NoError(t, err)
	require.Equal(t, types.Success, response.ReturnCode)
	require.NoError(t, closeState())

	response, _, err = invokeEndpointPatch(
		context.Background(),
		service,
		delTestContainerID,
		map[string]*IPInfo{InfraInterfaceName: {HostVethName: "closed"}},
	)
	require.NoError(t, err)
	assert.Equal(t, types.UnexpectedError, response.ReturnCode)

	reopened, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	restarted := newUnifiedAddTestService(t)
	restore, closeRestarted, err := NewDurableStateLifecycle(restarted, reopened, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeRestarted()) })
	info := restarted.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName]
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
	response, _, err = invokeEndpointPatch(
		context.Background(),
		readOnlyService,
		delTestContainerID,
		map[string]*IPInfo{InfraInterfaceName: {HostVethName: "read-only"}},
	)
	require.NoError(t, err)
	assert.Equal(t, types.UnexpectedError, response.ReturnCode)
	assert.Equal(
		t,
		"restart-veth",
		readOnlyService.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName,
	)
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
	service.EndpointState[delTestContainerID] = &EndpointInfo{
		PodName:      delTestPodName,
		PodNamespace: delTestNamespace,
		IfnameToIPMap: map[string]*IPInfo{
			InfraInterfaceName: {IPv4: []net.IPNet{testIPNet(delTestIPv4Address + "/24")}},
		},
	}
	require.NoError(t, service.EndpointStateStore.Write(EndpointStoreKey, service.EndpointState))
	require.Nil(t, service.selectedUnifiedStateAdapter())

	response, _, err := invokeEndpointPatch(
		context.Background(),
		service,
		delTestContainerID,
		map[string]*IPInfo{InfraInterfaceName: {HostVethName: "json-veth"}},
	)
	require.NoError(t, err)
	require.Equal(t, types.Success, response.ReturnCode)
	assert.Equal(t, "json-veth", service.EndpointState[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName)
	var persisted map[string]*EndpointInfo
	require.NoError(t, service.EndpointStateStore.Read(EndpointStoreKey, &persisted))
	assert.Equal(t, "json-veth", persisted[delTestContainerID].IfnameToIPMap[InfraInterfaceName].HostVethName)
	assert.Nil(t, service.selectedUnifiedStateAdapter())
}

func invokeEndpointPatch(
	ctx context.Context,
	service *HTTPRestService,
	endpointID string,
	request map[string]*IPInfo,
) (cns.Response, *httptest.ResponseRecorder, error) {
	body, err := json.Marshal(request) //nolint:musttag // IPInfo is the existing endpoint API wire type.
	if err != nil {
		return cns.Response{}, nil, fmt.Errorf("encoding endpoint patch request: %w", err)
	}
	httpRequest := httptest.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		cns.EndpointPath+endpointID,
		bytes.NewReader(body),
	)
	recorder := httptest.NewRecorder()
	service.EndpointHandlerAPI(recorder, httpRequest)
	var response cns.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		return cns.Response{}, recorder, fmt.Errorf("decoding endpoint patch response: %w", err)
	}
	return response, recorder, nil
}

func TestUnifiedPatchReadOnlyErrorIdentity(t *testing.T) {
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(bolterrors.ErrDatabaseReadOnly))
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(context.Canceled))
	assert.Equal(t, types.UnexpectedError, unifiedPatchResponseCode(state.ErrDeleteIntent))
}
