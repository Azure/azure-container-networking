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

var (
	errUnifiedAddCommitFailure     = errors.New("injected commit failure")
	errUnifiedAddProjectionFailure = errors.New("injected projection build failure")
	errUnifiedAddCacheFailure      = errors.New("injected cache application failure")
)

const (
	unifiedTestIPv6        = "2001:db8::4"
	unifiedTestIPID1       = "ip-1"
	unifiedTestIPv6Network = "2001:db8::"
	unifiedTestIPv4ID      = "ip-v4"
	unifiedTestIPv6ID      = "ip-v6"
	unifiedTestNCIPv4      = "nc-v4"
	unifiedTestNCIPv6      = "nc-v6"
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
				unifiedTestNCIPv4: {{ID: unifiedTestIPv4ID, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
			},
			want: []string{adapterTestIPv4},
		},
		{
			name: "dual stack desired order",
			containers: map[string][]state.IPRecord{
				unifiedTestNCIPv4: {{ID: unifiedTestIPv4ID, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
				unifiedTestNCIPv6: {{ID: unifiedTestIPv6ID, IPAddress: unifiedTestIPv6, NCID: unifiedTestNCIPv6, NCVersion: 1}},
			},
			desired: []string{unifiedTestIPv6, adapterTestIPv4},
			want:    []string{unifiedTestIPv6, adapterTestIPv4},
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
			request := unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1")
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
			assert.Equal(t, adapterTestContainerID, assignment.Pod.InfraContainerID)
			assert.Equal(t, "interface-1", assignment.Pod.InterfaceID)
			require.Contains(t, after.Endpoints, adapterTestContainerID)
			endpointInfo := after.Endpoints[adapterTestContainerID].IfnameToIPMap[InfraInterfaceName]
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
		unifiedTestNCIPv4: {
			{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1},
			{ID: "ip-2", IPAddress: primaryIP, NCID: unifiedTestNCIPv4, NCVersion: 1},
		},
	}, nil)
	primary := unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1")
	primary.DesiredIPAddresses = []string{adapterTestIPv4}
	secondary := unifiedAddRequest(adapterTestContainerID, "interface-2", "net1", "pod-1", "namespace-1")
	secondary.DesiredIPAddresses = []string{primaryIP}
	secondary.SecondaryInterfacesExist = true

	_, err := service.requestIPConfigHandlerHelper(context.Background(), primary)
	require.NoError(t, err)
	afterPrimary := requireUnifiedSnapshot(t, db)
	conflictingInterface := secondary
	conflictingInterface.Ifname = InfraInterfaceName
	response, err := service.requestIPConfigHandlerHelper(context.Background(), conflictingInterface)
	require.ErrorIs(t, err, state.ErrInvalidInput)
	assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
	assert.Equal(t, afterPrimary, requireUnifiedSnapshot(t, db))

	_, err = service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	afterAdds := requireUnifiedSnapshot(t, db)
	require.Len(t, afterAdds.Assignments, 2)
	require.Len(t, afterAdds.Endpoints[adapterTestContainerID].IfnameToIPMap, 2)
	assert.Contains(t, afterAdds.Endpoints[adapterTestContainerID].IfnameToIPMap, InfraInterfaceName)
	assert.Contains(t, afterAdds.Endpoints[adapterTestContainerID].IfnameToIPMap, "net1")

	replay, err := service.requestIPConfigHandlerHelper(context.Background(), secondary)
	require.NoError(t, err)
	require.Len(t, replay.PodIPInfo, 1)
	assert.Equal(t, primaryIP, replay.PodIPInfo[0].PodIPConfig.IPAddress)
	assert.Equal(t, afterAdds.Metadata.Generation, requireUnifiedSnapshot(t, db).Metadata.Generation)
	generation, _ := adapter.cacheGeneration()
	assert.Equal(t, afterAdds.Metadata.Generation, generation)

	changed := secondary
	changed.OrchestratorContext = mustPodContext(t, "other-pod", "namespace-1")
	response, err = service.requestIPConfigHandlerHelper(context.Background(), changed)
	require.ErrorIs(t, err, state.ErrInvalidInput)
	assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
	assert.Equal(t, afterAdds.Metadata.Generation, requireUnifiedSnapshot(t, db).Metadata.Generation)
}

func TestUnifiedAddDuplicateConcurrencyIsAtomic(t *testing.T) {
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
	}, nil)
	requests := []cns.IPConfigsRequest{
		unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		unifiedAddRequest("container-2", "interface-2", InfraInterfaceName, "pod-2", "namespace-1"),
	}
	for index := range requests {
		requests[index].DesiredIPAddresses = []string{adapterTestIPv4}
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
				unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
			}, func(db *state.DB) {
				require.NoError(t, db.Update(context.Background(), func(tx *state.WriteTx) error {
					return tx.PutDeleteIntent(adapterTestContainerID, state.DeleteIntent{CreatedAt: tt.createdAt})
				}))
			})
			adapter.now = func() time.Time { return now }
			before := requireUnifiedSnapshot(t, db)

			response, err := service.requestIPConfigHandlerHelper(
				context.Background(),
				unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
			)
			assert.Equal(t, tt.wantCode, response.Response.ReturnCode)
			after := requireUnifiedSnapshot(t, db)
			if tt.wantSuccess {
				require.NoError(t, err)
				assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
				assert.NotContains(t, after.DeleteIntents, adapterTestContainerID)
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
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		adapter.store.assignEndpoint = func(
			context.Context,
			uint64,
			state.AssignmentRecord,
			state.EndpointRecord,
			time.Time,
			time.Duration,
			func(state.Snapshot) error,
		) (bool, error) {
			return false, errUnifiedAddCommitFailure
		}

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, errUnifiedAddCommitFailure)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("projection prebuild callback", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		adapter.buildProjection = func(state.Snapshot) (durableCacheProjection, error) {
			return durableCacheProjection{}, errUnifiedAddProjectionFailure
		}

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, errUnifiedAddProjectionFailure)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("later invalid multi-IP candidate", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPv4ID, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
			unifiedTestNCIPv6: {{ID: unifiedTestIPv6ID, IPAddress: unifiedTestIPv6, NCID: unifiedTestNCIPv6, NCVersion: 1}},
		}, nil)
		nc := service.state.ContainerStatus[unifiedTestNCIPv6]
		nc.CreateNetworkContainerRequest.IPConfiguration.IPSubnet.PrefixLength = 200
		service.state.ContainerStatus[unifiedTestNCIPv6] = nc
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		request := unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1")
		request.DesiredIPAddresses = []string{adapterTestIPv4, unifiedTestIPv6}

		response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
		require.Error(t, err)
		assert.Equal(t, types.InvalidRequest, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("canceled before transaction", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		response, err := service.requestIPConfigHandlerHelper(
			ctx,
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, types.FailedToAllocateIPConfig, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("deadline before transaction", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
		}, nil)
		beforeSnapshot := requireUnifiedSnapshot(t, db)
		beforeCache := durableCacheFingerprint(service, adapter)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		response, err := service.requestIPConfigHandlerHelper(
			ctx,
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, types.FailedToAllocateIPConfig, response.Response.ReturnCode)
		assert.Equal(t, beforeSnapshot, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})
}

func TestUnifiedAddProjectionFailureRestoresCommittedState(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
	}, nil)
	beforeGeneration := requireUnifiedSnapshot(t, db).Metadata.Generation
	refreshMetrics := adapter.store.refreshMetrics
	refreshCalls := 0
	adapter.store.refreshMetrics = func(ctx context.Context) (state.Status, error) {
		refreshCalls++
		return refreshMetrics(ctx)
	}
	adapter.applyAddProjection = func(durableCacheProjection) error {
		service.PodIPConfigState = map[string]cns.IPConfigurationStatus{}
		return errUnifiedAddCacheFailure
	}

	response, err := service.requestIPConfigHandlerHelper(
		context.Background(),
		unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
	)
	var committedErr *unifiedAddCommittedError
	require.ErrorAs(t, err, &committedErr)
	require.ErrorIs(t, err, errUnifiedAddCacheFailure)
	assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)

	snapshot := requireUnifiedSnapshot(t, db)
	assert.Equal(t, beforeGeneration+1, snapshot.Metadata.Generation)
	assert.Contains(t, snapshot.Assignments, "interface-1")
	status := service.PodIPConfigState[unifiedTestIPID1]
	assert.Equal(t, types.Assigned, status.GetState())
	assert.Equal(t, []string{unifiedTestIPID1}, service.PodIPIDByPodInterfaceKey["interface-1"])
	generation, projected := adapter.cacheGeneration()
	assert.True(t, projected)
	assert.Equal(t, snapshot.Metadata.Generation, generation)
	assert.Equal(t, 1, refreshCalls)
}

func TestUnifiedAddStaleGenerationAndClosedDatabase(t *testing.T) {
	t.Run("stale adapter", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
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
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.ErrorIs(t, err, state.ErrStaleGeneration)
		assert.Equal(t, types.InconsistentIPConfigState, response.Response.ReturnCode)
		assert.Empty(t, external.Assignments)
		assert.Equal(t, external, requireUnifiedSnapshot(t, db))
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
	})

	t.Run("closed", func(t *testing.T) {
		service, adapter, db, closeState := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
		}, nil)
		beforeCache := durableCacheFingerprint(service, adapter)
		require.NoError(t, closeState())

		response, err := service.requestIPConfigHandlerHelper(
			context.Background(),
			unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1"),
		)
		require.Error(t, err)
		assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		_, snapshotErr := db.Snapshot(context.Background())
		require.Error(t, snapshotErr)
	})
}

func TestUnifiedAddImportedOwnershipReplayAndConflict(t *testing.T) {
	request := unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1")
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		unifiedTestNCIPv4: {{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4, NCVersion: 1}},
	}, func(db *state.DB) {
		_, err := db.AssignEndpoint(
			context.Background(),
			state.AssignmentRecord{
				Pod: state.PodIdentity{
					PodKey:           "interface-1",
					InfraContainerID: adapterTestContainerID,
					InterfaceID:      "interface-1",
					PodName:          "pod-1",
					PodNamespace:     "namespace-1",
				},
				IPIDs: []string{unifiedTestIPID1},
			},
			state.EndpointRecord{
				PodName:      "pod-1",
				PodNamespace: "namespace-1",
				IfnameToIPMap: map[string]*state.IPInfoRecord{
					InfraInterfaceName: {
						IPv4:               []net.IPNet{testIPNet("10.0.0.4/24")},
						NetworkContainerID: unifiedTestNCIPv4,
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
	service.state.ContainerStatus[unifiedTestNCIPv4] = containerstatus{
		ID:                            unifiedTestNCIPv4,
		HostVersion:                   "1",
		CreateNetworkContainerRequest: r18NetworkContainer(unifiedTestNCIPv4).Request,
	}
	status := cns.IPConfigurationStatus{ID: unifiedTestIPID1, IPAddress: adapterTestIPv4, NCID: unifiedTestNCIPv4}
	status.SetState(types.Available)
	service.PodIPConfigState[unifiedTestIPID1] = status
	request := unifiedAddRequest(adapterTestContainerID, "interface-1", InfraInterfaceName, "pod-1", "namespace-1")
	request.DesiredIPAddresses = []string{adapterTestIPv4}

	beforeAdapter := service.unifiedStateAdapter
	response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	expected := &cns.IPConfigsResponse{
		Response: cns.Response{ReturnCode: types.Success},
		PodIPInfo: []cns.PodIpInfo{{
			PodIPConfig: cns.IPSubnet{IPAddress: adapterTestIPv4, PrefixLength: 24},
			NetworkContainerPrimaryIPConfig: service.state.ContainerStatus[unifiedTestNCIPv4].
				CreateNetworkContainerRequest.IPConfiguration,
			HostPrimaryIPInfo: cns.HostIPInfo{
				PrimaryIP: "192.0.2.10",
				Subnet:    "192.0.2.0/24",
				Gateway:   "192.0.2.1",
			},
			MacAddress: adapterTestMAC,
			NICType:    cns.InfraNIC,
		}},
	}
	assert.Equal(t, expected, response)
	gotJSON, err := json.Marshal(response) //nolint:musttag // IPConfigsResponse is the existing CNS API wire type.
	require.NoError(t, err)
	wantJSON, err := json.Marshal(expected) //nolint:musttag // IPConfigsResponse is the existing CNS API wire type.
	require.NoError(t, err)
	assert.JSONEq(t, string(wantJSON), string(gotJSON))
	assert.Same(t, beforeAdapter, service.unifiedStateAdapter)
	assert.Equal(t, []string{unifiedTestIPID1}, service.PodIPIDByPodInterfaceKey["interface-1"])
	assigned := service.PodIPConfigState[unifiedTestIPID1]
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
		_, applyErr := db.ApplyNetworkContainer(context.Background(), r18NetworkContainer(id), ips)
		require.NoError(t, applyErr)
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
	prefix := cns.IPSubnet{IPAddress: adapterTestNetwork, PrefixLength: 24}
	if strings.Contains(id, "v6") {
		prefix = cns.IPSubnet{IPAddress: unifiedTestIPv6Network, PrefixLength: 64}
	}
	return state.NewNetworkContainerRecord(id, "1", "1", true, cns.CreateNetworkContainerRequest{
		NetworkContainerid: id,
		Version:            "1",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         prefix,
			GatewayIPAddress: "10.0.0.1",
			DNSServers:       []string{"168.63.129.16"},
		},
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{MACAddress: adapterTestMAC},
	})
}

//nolint:unparam // Downstream DEL and PATCH slices exercise distinct namespaces.
func unifiedAddRequest(containerID, interfaceID, ifname, podName, namespace string) cns.IPConfigsRequest {
	orchestratorContext, err := json.Marshal(cns.KubernetesPodInfo{PodName: podName, PodNamespace: namespace})
	if err != nil {
		panic(err)
	}
	return cns.IPConfigsRequest{
		PodInterfaceID:      interfaceID,
		InfraContainerID:    containerID,
		Ifname:              ifname,
		OrchestratorContext: orchestratorContext,
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
