// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/types/bounded"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedAddSingleAndDualStack(t *testing.T) {
	tests := []struct {
		name       string
		containers map[string][]state.IPRecord
		desired    []string
		want       []string
	}{
		{
			name: "single",
			containers: map[string][]state.IPRecord{
				"nc-v4": {{ID: "ip-v4", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
			},
			want: []string{"10.0.0.4"},
		},
		{
			name: "dual stack desired order",
			containers: map[string][]state.IPRecord{
				"nc-v4": {{ID: "ip-v4", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
				"nc-v6": {{ID: "ip-v6", IPAddress: "2001:db8::4", NCID: "nc-v6", NCVersion: 1}},
			},
			desired: []string{"2001:db8::4", "10.0.0.4"},
			want:    []string{"2001:db8::4", "10.0.0.4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, tt.containers, nil)
			refreshMetrics := adapter.store.refreshMetrics
			refreshCalls := 0
			adapter.store.refreshMetrics = func(ctx context.Context) (state.Status, error) {
				refreshCalls++
				return refreshMetrics(ctx)
			}
			request := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
			request.DesiredIPAddresses = tt.desired
			before := requireUnifiedSnapshot(t, db)

			response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, types.Success, response.Response.ReturnCode)
			require.Len(t, response.PodIPInfo, len(tt.want))
			for index, address := range tt.want {
				assert.Equal(t, address, response.PodIPInfo[index].PodIPConfig.IPAddress)
			}

			after := requireUnifiedSnapshot(t, db)
			assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
			assignment := after.Assignments["interface-1"]
			require.Len(t, assignment.IPIDs, len(tt.want))
			assert.Equal(t, "container-1", assignment.Pod.InfraContainerID)
			assert.Equal(t, "interface-1", assignment.Pod.InterfaceID)
			require.Contains(t, after.Endpoints, "container-1")
			endpointInfo := after.Endpoints["container-1"].IfnameToIPMap["eth0"]
			assert.Len(t, endpointInfo.IPv4, 1)
			assert.Len(t, endpointInfo.IPv6, len(tt.want)-1)
			for _, ipID := range assignment.IPIDs {
				assert.Equal(t, "interface-1", after.IPOwners[ipID])
				status := service.PodIPConfigState[ipID]
				assert.Equal(t, types.Assigned, status.GetState())
			}
			generation, projected := adapter.cacheGeneration()
			assert.True(t, projected)
			assert.Equal(t, after.Metadata.Generation, generation)
			assert.Equal(t, assignment.IPIDs, service.PodIPIDByPodInterfaceKey["interface-1"])
			assert.Equal(t, 1, refreshCalls)
		})
	}
}

func TestUnifiedAddMultiNICAndReplayIdentity(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {
			{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1},
			{ID: "ip-2", IPAddress: "10.0.0.5", NCID: "nc-v4", NCVersion: 1},
		},
	}, nil)
	primary := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	primary.DesiredIPAddresses = []string{"10.0.0.4"}
	secondary := unifiedAddRequest("container-1", "interface-2", "net1", "pod-1", "namespace-1")
	secondary.DesiredIPAddresses = []string{"10.0.0.5"}
	secondary.SecondaryInterfacesExist = true

	_, err := service.requestIPConfigHandlerHelper(context.Background(), primary)
	require.NoError(t, err)
	afterPrimary := requireUnifiedSnapshot(t, db)
	conflictingInterface := secondary
	conflictingInterface.Ifname = "eth0"
	response, err := service.requestIPConfigHandlerHelper(context.Background(), conflictingInterface)
	require.ErrorIs(t, err, state.ErrInvalidInput)
	assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
	assert.Equal(t, afterPrimary, requireUnifiedSnapshot(t, db))

	_, err = service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	afterAdds := requireUnifiedSnapshot(t, db)
	require.Len(t, afterAdds.Assignments, 2)
	require.Len(t, afterAdds.Endpoints["container-1"].IfnameToIPMap, 2)
	assert.Contains(t, afterAdds.Endpoints["container-1"].IfnameToIPMap, "eth0")
	assert.Contains(t, afterAdds.Endpoints["container-1"].IfnameToIPMap, "net1")

	replay, err := service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	require.Len(t, replay.PodIPInfo, 1)
	assert.Equal(t, "10.0.0.5", replay.PodIPInfo[0].PodIPConfig.IPAddress)
	assert.Equal(t, afterAdds.Metadata.Generation, requireUnifiedSnapshot(t, db).Metadata.Generation)
	generation, _ := adapter.cacheGeneration()
	assert.Equal(t, afterAdds.Metadata.Generation, generation)

	changed := secondary
	changed.OrchestratorContext = mustPodContext(t, "other-pod", "namespace-1")
	response, err = service.requestIPConfigHandlerHelper(context.Background(), changed)
	require.Error(t, err)
	assert.ErrorIs(t, err, state.ErrInvalidInput)
	assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
	assert.Equal(t, afterAdds.Metadata.Generation, requireUnifiedSnapshot(t, db).Metadata.Generation)
}

func TestUnifiedAddDuplicateConcurrencyIsAtomic(t *testing.T) {
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
	}, nil)
	requests := []cns.IPConfigsRequest{
		unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		unifiedAddRequest("container-2", "interface-2", "eth0", "pod-2", "namespace-1"),
	}
	for index := range requests {
		requests[index].DesiredIPAddresses = []string{"10.0.0.4"}
	}

	start := make(chan struct{})
	results := make(chan *cns.IPConfigsResponse, len(requests))
	errs := make(chan error, len(requests))
	var ready sync.WaitGroup
	ready.Add(len(requests))
	for _, request := range requests {
		go func() {
			ready.Done()
			<-start
			response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			results <- response
			errs <- err
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for range requests {
		response := <-results
		err := <-errs
		if err == nil {
			successes++
			assert.Equal(t, types.Success, response.Response.ReturnCode)
			continue
		}
		conflicts++
		assert.Equal(t, types.AddressUnavailable, response.Response.ReturnCode)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	snapshot := requireUnifiedSnapshot(t, db)
	assert.Len(t, snapshot.Assignments, 1)
	assert.Len(t, snapshot.IPOwners, 1)
	assert.Len(t, snapshot.Endpoints, 1)
}

func TestUnifiedAddDeleteIntentAndExpiry(t *testing.T) {
	now := time.Date(2026, time.July, 24, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		createdAt   time.Time
		wantCode    types.ResponseCode
		wantSuccess bool
	}{
		{
			name:      "active",
			createdAt: now.Add(-unifiedDeleteIntentTTL + time.Nanosecond),
			wantCode:  types.AddressUnavailable,
		},
		{
			name:        "expired",
			createdAt:   now.Add(-unifiedDeleteIntentTTL),
			wantCode:    types.Success,
			wantSuccess: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
				"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
			}, func(db *state.DB) {
				require.NoError(t, db.Update(context.Background(), func(tx *state.WriteTx) error {
					return tx.PutDeleteIntent("container-1", state.DeleteIntent{CreatedAt: tt.createdAt})
				}))
			})
			adapter.now = func() time.Time { return now }
			before := requireUnifiedSnapshot(t, db)

			response, err := service.requestIPConfigHandlerHelper(
				context.Background(),
				unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
			)
			assert.Equal(t, tt.wantCode, response.Response.ReturnCode)
			after := requireUnifiedSnapshot(t, db)
			if tt.wantSuccess {
				require.NoError(t, err)
				assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
				assert.NotContains(t, after.DeleteIntents, "container-1")
				assert.Contains(t, after.Assignments, "interface-1")
			} else {
				require.ErrorIs(t, err, state.ErrDeleteIntent)
				assert.Equal(t, before, after)
				assert.Empty(t, service.PodIPIDByPodInterfaceKey)
			}
		})
	}
}

func TestUnifiedAddFailuresDoNotPartiallyAllocate(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		injected := errors.New("injected commit failure")
		adapter.store.assignEndpoint = func(
			context.Context,
			uint64,
			state.AssignmentRecord,
			state.EndpointRecord,
			time.Time,
			time.Duration,
			func(state.Snapshot) error,
		) (bool, error) {
			return false, injected
		}

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, injected)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("projection prebuild callback", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		injected := errors.New("injected projection build failure")
		adapter.buildProjection = func(state.Snapshot) (durableCacheProjection, error) {
			return durableCacheProjection{}, injected
		}

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, injected)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("later invalid multi-IP candidate", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-v4", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
			"nc-v6": {{ID: "ip-v6", IPAddress: "2001:db8::4", NCID: "nc-v6", NCVersion: 1}},
		}, nil)
		nc := service.state.ContainerStatus["nc-v6"]
		nc.CreateNetworkContainerRequest.IPConfiguration.IPSubnet.PrefixLength = 200
		service.state.ContainerStatus["nc-v6"] = nc
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		request := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
		request.DesiredIPAddresses = []string{"10.0.0.4", "2001:db8::4"}

		response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
		require.Error(t, err)
		assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("canceled before transaction", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		response, err := service.requestIPConfigHandlerHelper(
			ctx,
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, types.FailedToAllocateIPConfig, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("deadline before transaction", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		response, err := service.requestIPConfigHandlerHelper(
			ctx,
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, types.FailedToAllocateIPConfig, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})
}

func TestUnifiedAddProjectionFailureRestoresCommittedState(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
	}, nil)
	beforeGeneration := requireUnifiedSnapshot(t, db).Metadata.Generation
	injected := errors.New("injected cache application failure")
	refreshMetrics := adapter.store.refreshMetrics
	refreshCalls := 0
	adapter.store.refreshMetrics = func(ctx context.Context) (state.Status, error) {
		refreshCalls++
		return refreshMetrics(ctx)
	}
	adapter.applyAddProjection = func(durableCacheProjection) error {
		service.PodIPConfigState = map[string]cns.IPConfigurationStatus{}
		return injected
	}

	response, err := service.requestIPConfigHandlerHelper(
		context.Background(),
		unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
	)
	var committedErr *unifiedAddCommittedError
	require.ErrorAs(t, err, &committedErr)
	require.ErrorIs(t, err, injected)
	assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)

	snapshot := requireUnifiedSnapshot(t, db)
	assert.Equal(t, beforeGeneration+1, snapshot.Metadata.Generation)
	assert.Contains(t, snapshot.Assignments, "interface-1")
	status := service.PodIPConfigState["ip-1"]
	assert.Equal(t, types.Assigned, status.GetState())
	assert.Equal(t, []string{"ip-1"}, service.PodIPIDByPodInterfaceKey["interface-1"])
	generation, projected := adapter.cacheGeneration()
	assert.True(t, projected)
	assert.Equal(t, snapshot.Metadata.Generation, generation)
	assert.Equal(t, 1, refreshCalls)
}

func TestUnifiedAddStaleGenerationAndClosedDatabase(t *testing.T) {
	t.Run("stale adapter", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeCache := durableCacheFingerprint(service, adapter)
		_, err := db.ApplyNetworkContainer(
			context.Background(),
			r18NetworkContainer("external"),
			[]state.IPRecord{{ID: "external-ip", IPAddress: "10.1.0.4", NCID: "external", NCVersion: 1}},
		)
		require.NoError(t, err)
		external := requireUnifiedSnapshot(t, db)

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, state.ErrStaleGeneration)
		assert.Equal(t, types.InconsistentIPConfigState, response.Response.ReturnCode)
		assert.Empty(t, external.Assignments)
		assert.Equal(t, external, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("closed", func(t *testing.T) {
		service, adapter, db, closeState := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
		}, nil)
		beforeCache := durableCacheFingerprint(service, adapter)
		require.NoError(t, closeState())

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1"),
		)
		require.Error(t, err)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		_, snapshotErr := db.Snapshot(context.Background())
		require.Error(t, snapshotErr)
	})
}

func TestUnifiedAddImportedOwnershipReplayAndConflict(t *testing.T) {
	request := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		"nc-v4": {{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4", NCVersion: 1}},
	}, func(db *state.DB) {
		_, err := db.AssignEndpoint(
			context.Background(),
			state.AssignmentRecord{
				Pod: state.PodIdentity{
					PodKey:           "interface-1",
					InfraContainerID: "container-1",
					InterfaceID:      "interface-1",
					PodName:          "pod-1",
					PodNamespace:     "namespace-1",
				},
				IPIDs: []string{"ip-1"},
			},
			state.EndpointRecord{
				PodName:      "pod-1",
				PodNamespace: "namespace-1",
				IfnameToIPMap: map[string]*state.IPInfoRecord{
					"eth0": {
						IPv4:               []net.IPNet{testIPNet("10.0.0.4/24")},
						NetworkContainerID: "nc-v4",
						NICType:            cns.InfraNIC,
					},
				},
			},
			time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC),
			unifiedDeleteIntentTTL,
		)
		require.NoError(t, err)
	})
	before := requireUnifiedSnapshot(t, db)

	response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, types.Success, response.Response.ReturnCode)
	assert.Equal(t, before.Metadata.Generation, requireUnifiedSnapshot(t, db).Metadata.Generation)

	conflict := request
	conflict.InfraContainerID = "container-2"
	response, err = service.requestIPConfigHandlerHelper(context.Background(), conflict)
	require.ErrorIs(t, err, state.ErrInvalidInput)
	assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
	assert.Equal(t, before, requireUnifiedSnapshot(t, db))
}

func TestJSONAddPathDoesNotSelectUnifiedState(t *testing.T) {
	service := newUnifiedAddTestService(t)
	assert.Nil(t, service.selectedUnifiedStateAdapter())
	service.state.ContainerStatus["nc-v4"] = containerstatus{
		ID:                            "nc-v4",
		HostVersion:                   "1",
		CreateNetworkContainerRequest: r18NetworkContainer("nc-v4").Request,
	}
	status := cns.IPConfigurationStatus{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-v4"}
	status.SetState(types.Available)
	service.PodIPConfigState["ip-1"] = status
	request := unifiedAddRequest("container-1", "interface-1", "eth0", "pod-1", "namespace-1")
	request.DesiredIPAddresses = []string{"10.0.0.4"}

	beforeAdapter := service.unifiedStateAdapter
	response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	expected := &cns.IPConfigsResponse{
		Response: cns.Response{ReturnCode: types.Success},
		PodIPInfo: []cns.PodIpInfo{{
			PodIPConfig: cns.IPSubnet{IPAddress: "10.0.0.4", PrefixLength: 24},
			NetworkContainerPrimaryIPConfig: service.state.ContainerStatus["nc-v4"].
				CreateNetworkContainerRequest.IPConfiguration,
			HostPrimaryIPInfo: cns.HostIPInfo{
				PrimaryIP: "192.0.2.10",
				Subnet:    "192.0.2.0/24",
				Gateway:   "192.0.2.1",
			},
			MacAddress: "00:11:22:33:44:55",
			NICType:    cns.InfraNIC,
		}},
	}
	assert.Equal(t, expected, response)
	gotJSON, err := json.Marshal(response)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(expected)
	require.NoError(t, err)
	assert.Equal(t, wantJSON, gotJSON)
	assert.Same(t, beforeAdapter, service.unifiedStateAdapter)
	assert.Equal(t, []string{"ip-1"}, service.PodIPIDByPodInterfaceKey["interface-1"])
	assigned := service.PodIPConfigState["ip-1"]
	assert.Equal(t, types.Assigned, assigned.GetState())
}

func newUnifiedAddFixture(
	t *testing.T,
	containers map[string][]state.IPRecord,
	beforeAttach func(*state.DB),
) (*HTTPRestService, *durableStateAdapter, *state.DB, func() error) {
	t.Helper()
	db, err := state.Open(filepath.Join(t.TempDir(), "state.db"), state.Options{})
	require.NoError(t, err)
	for id, ips := range containers {
		_, err := db.ApplyNetworkContainer(context.Background(), r18NetworkContainer(id), ips)
		require.NoError(t, err)
	}
	if beforeAttach != nil {
		beforeAttach(db)
	}
	service := newUnifiedAddTestService(t)
	restore, closeState, err := NewDurableStateLifecycle(service, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeState()) })
	return service, service.unifiedStateAdapter, db, closeState
}

func newUnifiedAddTestService(t *testing.T) *HTTPRestService {
	t.Helper()
	base, err := cns.NewService("test", "test", cns.Managed, nil)
	require.NoError(t, err)
	return &HTTPRestService{
		Service: base,
		state: &httpRestServiceState{
			ContainerStatus:                  map[string]containerstatus{},
			ContainerIDByOrchestratorContext: map[string]*ncList{},
			Networks:                         map[string]*networkInfo{},
			joinedNetworks:                   map[string]struct{}{},
			PnpIDByMacAddress:                map[string]string{},
			primaryInterface: &wireserver.InterfaceInfo{
				PrimaryIP: "192.0.2.10",
				Subnet:    "192.0.2.0/24",
				Gateway:   "192.0.2.1",
			},
		},
		PodIPConfigState:         map[string]cns.IPConfigurationStatus{},
		PodIPIDByPodInterfaceKey: map[string][]string{},
		EndpointState:            map[string]*EndpointInfo{},
		podsPendingIPAssignment:  bounded.NewTimedSet(10),
	}
}

func r18NetworkContainer(id string) state.NetworkContainerRecord {
	prefix := cns.IPSubnet{IPAddress: "10.0.0.0", PrefixLength: 24}
	if strings.Contains(id, "v6") {
		prefix = cns.IPSubnet{IPAddress: "2001:db8::", PrefixLength: 64}
	}
	return state.NewNetworkContainerRecord(id, "1", "1", true, cns.CreateNetworkContainerRequest{
		NetworkContainerid: id,
		Version:            "1",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         prefix,
			GatewayIPAddress: "10.0.0.1",
			DNSServers:       []string{"168.63.129.16"},
		},
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{MACAddress: "00:11:22:33:44:55"},
	})
}

func unifiedAddRequest(containerID, interfaceID, ifname, podName, namespace string) cns.IPConfigsRequest {
	context, err := json.Marshal(cns.KubernetesPodInfo{PodName: podName, PodNamespace: namespace})
	if err != nil {
		panic(err)
	}
	return cns.IPConfigsRequest{
		PodInterfaceID:      interfaceID,
		InfraContainerID:    containerID,
		Ifname:              ifname,
		OrchestratorContext: context,
	}
}

func mustPodContext(t *testing.T, podName, namespace string) json.RawMessage {
	t.Helper()
	value, err := json.Marshal(cns.KubernetesPodInfo{PodName: podName, PodNamespace: namespace})
	require.NoError(t, err)
	return value
}

func requireUnifiedSnapshot(t *testing.T, db *state.DB) state.Snapshot {
	t.Helper()
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	return snapshot
}
