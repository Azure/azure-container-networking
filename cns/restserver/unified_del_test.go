// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errInjectedReleaseFailure    = errors.New("injected release failure")
	errInjectedProjectionFailure = errors.New("injected projection failure")
)

const (
	delTestIPv4Address = "10.0.0.4"
	delTestIPv6Address = "2001:db8::4"
	delTestIPv4ID      = "ip-v4"
	delTestIPv6ID      = "ip-v6"
	delTestIPID1       = "ip-1"
	delTestCanceled    = "canceled"
	delTestInterfaceID = "interface-1"
	delTestContainerID = "container-1"
	delTestPodName     = "pod-1"
	delTestNamespace   = "namespace-1"
	delTestNCIPv4      = "nc-v4"
	delTestNCIPv6      = "nc-v6"
)

func TestUnifiedReleaseSingleAndMultiIP(t *testing.T) {
	tests := []struct {
		name       string
		containers map[string][]state.IPRecord
		desired    []string
	}{
		{
			name: "single",
			containers: map[string][]state.IPRecord{
				delTestNCIPv4: {{ID: delTestIPv4ID, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
			},
			desired: []string{delTestIPv4Address},
		},
		{
			name: "dual stack",
			containers: map[string][]state.IPRecord{
				delTestNCIPv4: {{ID: delTestIPv4ID, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
				delTestNCIPv6: {{ID: delTestIPv6ID, IPAddress: delTestIPv6Address, NCID: delTestNCIPv6, NCVersion: 1}},
			},
			desired: []string{delTestIPv6Address, delTestIPv4Address},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, tt.containers, nil)
			now := time.Date(2026, time.July, 24, 2, 0, 0, 0, time.UTC)
			adapter.now = func() time.Time { return now }
			request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
			request.DesiredIPAddresses = tt.desired
			_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			require.NoError(t, err)
			beforeRelease := requireUnifiedSnapshot(t, db)

			response, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, types.Success, response.Response.ReturnCode)
			released := requireUnifiedSnapshot(t, db)
			assert.Equal(t, beforeRelease.Metadata.Generation+1, released.Metadata.Generation)
			assert.Empty(t, released.Assignments)
			assert.Empty(t, released.IPOwners)
			assert.Contains(t, released.Endpoints, delTestContainerID)
			assert.Equal(t, now, released.DeleteIntents[delTestContainerID].CreatedAt)
			assert.Empty(t, service.PodIPIDByPodInterfaceKey)
			for ipID := range released.IPs {
				status := service.PodIPConfigState[ipID]
				assert.Equal(t, types.Available, status.GetState())
			}

			adapter.now = func() time.Time { return now.Add(time.Minute) }
			response, err = service.ReleaseIPConfigHandlerHelper(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, types.Success, response.Response.ReturnCode)
			repeated := requireUnifiedSnapshot(t, db)
			assert.Equal(t, released.Metadata.Generation, repeated.Metadata.Generation)
			assert.Equal(t, now, repeated.DeleteIntents[delTestContainerID].CreatedAt)

			require.NoError(t, adapter.deleteEndpointRecord(context.Background(), delTestContainerID))
			cleaned := requireUnifiedSnapshot(t, db)
			assert.Equal(t, released.Metadata.Generation+1, cleaned.Metadata.Generation)
			assert.NotContains(t, cleaned.Endpoints, delTestContainerID)
			assert.Equal(t, now, cleaned.DeleteIntents[delTestContainerID].CreatedAt)
		})
	}
}

func TestUnifiedReleaseStaleAndInvalidRequestsAreAtomic(t *testing.T) {
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {
			{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1},
			{ID: "ip-2", IPAddress: primaryIP, NCID: delTestNCIPv4, NCVersion: 1},
		},
	}, nil)
	current := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	current.DesiredIPAddresses = []string{delTestIPv4Address, primaryIP}
	_, err := service.requestIPConfigHandlerHelper(context.Background(), current)
	require.NoError(t, err)

	tests := []struct {
		name     string
		request  cns.IPConfigsRequest
		wantCode types.ResponseCode
		wantErr  bool
	}{
		{
			name: "old container is stale success",
			request: func() cns.IPConfigsRequest {
				request := current
				request.InfraContainerID = "old-container"
				return request
			}(),
			wantCode: types.Success,
		},
		{
			name: "old IP set is stale success",
			request: func() cns.IPConfigsRequest {
				request := current
				request.DesiredIPAddresses = []string{"10.0.0.99"}
				return request
			}(),
			wantCode: types.Success,
		},
		{
			name: "partial multi-IP set is invalid",
			request: func() cns.IPConfigsRequest {
				request := current
				request.DesiredIPAddresses = []string{delTestIPv4Address}
				return request
			}(),
			wantCode: types.InvalidRequest,
			wantErr:  true,
		},
		{
			name: "mixed valid and invalid IP is invalid",
			request: func() cns.IPConfigsRequest {
				request := current
				request.DesiredIPAddresses = []string{delTestIPv4Address, "not-an-ip"}
				return request
			}(),
			wantCode: types.InvalidRequest,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := requireUnifiedSnapshot(t, db)
			response, err := service.ReleaseIPConfigHandlerHelper(context.Background(), tt.request)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCode, response.Response.ReturnCode)
			assert.Equal(t, before, requireUnifiedSnapshot(t, db))
		})
	}
}

func TestUnifiedReleaseCrashRecoveryAndPrune(t *testing.T) {
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
	request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	request.DesiredIPAddresses = []string{delTestIPv4Address}
	_, err = service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	now := time.Date(2026, time.July, 24, 3, 0, 0, 0, time.UTC)
	service.unifiedStateAdapter.now = func() time.Time { return now }
	_, err = service.ReleaseIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, closeState())

	db, err = state.Open(path, state.Options{})
	require.NoError(t, err)
	reopenedService := newUnifiedAddTestService(t)
	restore, closeState, err = NewDurableStateLifecycle(reopenedService, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	t.Cleanup(func() { require.NoError(t, closeState()) })
	reopened := requireUnifiedSnapshot(t, db)
	assert.Empty(t, reopened.Assignments)
	assert.Empty(t, reopened.IPOwners)
	assert.Contains(t, reopened.Endpoints, delTestContainerID)
	assert.Equal(t, now, reopened.DeleteIntents[delTestContainerID].CreatedAt)

	reopenedService.unifiedStateAdapter.now = func() time.Time { return now.Add(time.Minute) }
	response, err := reopenedService.requestIPConfigHandlerHelper(context.Background(), request)
	require.ErrorIs(t, err, state.ErrDeleteIntent)
	assert.Equal(t, types.AddressUnavailable, response.Response.ReturnCode)
	require.NoError(t, reopenedService.unifiedStateAdapter.deleteEndpointRecord(context.Background(), delTestContainerID))
	require.NoError(t, closeState())

	db, err = state.Open(path, state.Options{})
	require.NoError(t, err)
	afterDeleteService := newUnifiedAddTestService(t)
	restore, closeState, err = NewDurableStateLifecycle(afterDeleteService, db, true)
	require.NoError(t, err)
	require.NoError(t, restore(context.Background()))
	afterDelete := requireUnifiedSnapshot(t, db)
	assert.NotContains(t, afterDelete.Endpoints, delTestContainerID)
	assert.Equal(t, now, afterDelete.DeleteIntents[delTestContainerID].CreatedAt)

	count, err := afterDeleteService.unifiedStateAdapter.pruneDeleteIntents(
		context.Background(),
		now.Add(unifiedDeleteIntentTTL),
	)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	response, err = afterDeleteService.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, types.Success, response.Response.ReturnCode)
}

func TestUnifiedReleaseConcurrentDuplicate(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
	}, nil)
	request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	request.DesiredIPAddresses = []string{delTestIPv4Address}
	_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	now := time.Date(2026, time.July, 24, 4, 0, 0, 0, time.UTC)
	adapter.now = func() time.Time { return now }
	before := requireUnifiedSnapshot(t, db)

	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			_, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	for range callers {
		require.NoError(t, <-errs)
	}
	after := requireUnifiedSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
	assert.Equal(t, now, after.DeleteIntents[delTestContainerID].CreatedAt)
	assert.Empty(t, after.Assignments)
	assert.Empty(t, after.IPOwners)
}

func TestUnifiedReleaseLinearizesWithAdd(t *testing.T) {
	t.Run("ADD commits first and DEL releases it", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		request.DesiredIPAddresses = []string{delTestIPv4Address}
		realAssign := adapter.store.assignEndpoint
		entered := make(chan struct{})
		unblock := make(chan struct{})
		adapter.store.assignEndpoint = func(
			ctx context.Context,
			generation uint64,
			assignment state.AssignmentRecord,
			endpoint state.EndpointRecord,
			now time.Time,
			ttl time.Duration,
			beforeCommit func(state.Snapshot) error,
		) (bool, error) {
			close(entered)
			<-unblock
			return realAssign(ctx, generation, assignment, endpoint, now, ttl, beforeCommit)
		}

		addResult := make(chan error, 1)
		go func() {
			_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			addResult <- err
		}()
		<-entered
		delResult := make(chan error, 1)
		go func() {
			_, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
			delResult <- err
		}()
		close(unblock)
		require.NoError(t, <-addResult)
		require.NoError(t, <-delResult)
		snapshot := requireUnifiedSnapshot(t, db)
		assert.Empty(t, snapshot.Assignments)
		assert.Empty(t, snapshot.IPOwners)
		assert.Contains(t, snapshot.DeleteIntents, delTestContainerID)
	})

	t.Run("DEL commits first and late ADD is blocked", func(t *testing.T) {
		service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
			delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
		}, nil)
		request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
		request.DesiredIPAddresses = []string{delTestIPv4Address}
		_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
		require.NoError(t, err)
		now := time.Date(2026, time.July, 24, 5, 0, 0, 0, time.UTC)
		adapter.now = func() time.Time { return now }
		realRelease := adapter.store.releaseEndpoint
		entered := make(chan struct{})
		unblock := make(chan struct{})
		adapter.store.releaseEndpoint = func(
			ctx context.Context,
			generation uint64,
			pod state.PodIdentity,
			now time.Time,
			beforeCommit func(state.Snapshot) error,
		) (bool, error) {
			close(entered)
			<-unblock
			return realRelease(ctx, generation, pod, now, beforeCommit)
		}

		delResult := make(chan error, 1)
		go func() {
			_, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
			delResult <- err
		}()
		<-entered
		type addResult struct {
			response *cns.IPConfigsResponse
			err      error
		}
		addDone := make(chan addResult, 1)
		go func() {
			response, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			addDone <- addResult{response: response, err: err}
		}()
		close(unblock)
		require.NoError(t, <-delResult)
		add := <-addDone
		require.ErrorIs(t, add.err, state.ErrDeleteIntent)
		assert.Equal(t, types.AddressUnavailable, add.response.Response.ReturnCode)
		snapshot := requireUnifiedSnapshot(t, db)
		assert.Empty(t, snapshot.Assignments)
		assert.Empty(t, snapshot.IPOwners)
		assert.Contains(t, snapshot.DeleteIntents, delTestContainerID)
	})
}

func TestUnifiedReleaseFailureAtomicityAndMapping(t *testing.T) {
	tests := []struct {
		name     string
		inject   func(*durableStateAdapter)
		context  func() context.Context
		wantCode types.ResponseCode
	}{
		{
			name: "commit",
			inject: func(adapter *durableStateAdapter) {
				adapter.store.releaseEndpoint = func(
					context.Context,
					uint64,
					state.PodIdentity,
					time.Time,
					func(state.Snapshot) error,
				) (bool, error) {
					return false, errInjectedReleaseFailure
				}
			},
			wantCode: types.UnexpectedError,
		},
		{
			name: "stale generation",
			inject: func(adapter *durableStateAdapter) {
				adapter.store.releaseEndpoint = func(
					context.Context,
					uint64,
					state.PodIdentity,
					time.Time,
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
					return durableCacheProjection{}, errInjectedReleaseFailure
				}
			},
			wantCode: types.UnexpectedError,
		},
		{
			name:     delTestCanceled,
			inject:   func(*durableStateAdapter) {},
			context:  canceledUnifiedReleaseContext,
			wantCode: types.UnexpectedError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
				delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
			}, nil)
			request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
			request.DesiredIPAddresses = []string{delTestIPv4Address}
			_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
			require.NoError(t, err)
			beforeDB := requireUnifiedSnapshot(t, db)
			beforeCache := durableCacheFingerprint(service, adapter)
			tt.inject(adapter)
			ctx := context.Background()
			if tt.context != nil {
				ctx = tt.context()
			}

			response, err := service.ReleaseIPConfigHandlerHelper(ctx, request)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, response.Response.ReturnCode)
			assert.Equal(t, beforeDB, requireUnifiedSnapshot(t, db))
			assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		})
	}
}

func TestUnifiedReleasePostcommitProjectionRestoresCache(t *testing.T) {
	service, adapter, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
	}, nil)
	request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	request.DesiredIPAddresses = []string{delTestIPv4Address}
	_, err := service.requestIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	adapter.applyDeleteProjection = func(durableCacheProjection) error { return errInjectedProjectionFailure }

	response, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
	require.ErrorIs(t, err, errInjectedProjectionFailure)
	assert.Equal(t, types.UnexpectedError, response.Response.ReturnCode)
	snapshot := requireUnifiedSnapshot(t, db)
	assert.Empty(t, snapshot.Assignments)
	assert.Empty(t, snapshot.IPOwners)
	assert.Contains(t, snapshot.DeleteIntents, delTestContainerID)
	status := service.PodIPConfigState[delTestIPID1]
	assert.Equal(t, types.Available, status.GetState())
	assert.Empty(t, service.PodIPIDByPodInterfaceKey)
	generation, projected := adapter.cacheGeneration()
	assert.True(t, projected)
	assert.Equal(t, snapshot.Metadata.Generation, generation)
}

func TestUnifiedReleaseImportedAssignment(t *testing.T) {
	service, _, db, _ := newUnifiedAddFixture(t, map[string][]state.IPRecord{
		delTestNCIPv4: {{ID: delTestIPID1, IPAddress: delTestIPv4Address, NCID: delTestNCIPv4, NCVersion: 1}},
	}, func(db *state.DB) {
		assignment := state.AssignmentRecord{
			Pod: state.PodIdentity{
				PodKey:           delTestInterfaceID,
				InfraContainerID: delTestContainerID,
				InterfaceID:      delTestInterfaceID,
				PodName:          delTestPodName,
				PodNamespace:     delTestNamespace,
			},
			IPIDs: []string{delTestIPID1},
		}
		_, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			state.EndpointRecord{
				PodName:      delTestPodName,
				PodNamespace: delTestNamespace,
				IfnameToIPMap: map[string]*state.IPInfoRecord{
					InfraInterfaceName: {
						IPv4:               []net.IPNet{testIPNet(delTestIPv4Address + "/24")},
						NetworkContainerID: delTestNCIPv4,
						NICType:            cns.InfraNIC,
					},
				},
			},
			time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC),
			unifiedDeleteIntentTTL,
		)
		require.NoError(t, err)
	})
	request := unifiedAddRequest(delTestContainerID, delTestInterfaceID, InfraInterfaceName, delTestPodName, delTestNamespace)
	request.DesiredIPAddresses = []string{delTestIPv4Address}

	response, err := service.ReleaseIPConfigHandlerHelper(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, types.Success, response.Response.ReturnCode)
	snapshot := requireUnifiedSnapshot(t, db)
	assert.Empty(t, snapshot.Assignments)
	assert.Empty(t, snapshot.IPOwners)
	assert.Contains(t, snapshot.DeleteIntents, delTestContainerID)
}

func canceledUnifiedReleaseContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
