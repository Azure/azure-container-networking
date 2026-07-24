// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolterrors "go.etcd.io/bbolt/errors"
)

var (
	errAdapterCommitFailure   = errors.New("commit failed")
	errAdapterMetadataFailure = errors.New("metadata commit failed")
	errAdapterCloseFailure    = errors.New("close failed")
)

const (
	adapterTestNCID        = "nc-1"
	adapterTestIPv4        = "10.0.0.4"
	adapterTestSubnet      = "10.0.0.0/24"
	adapterTestLocation    = "azure"
	adapterTestNetworkType = "underlay"
	adapterTestNetwork     = "10.0.0.0"
	adapterTestDNS         = "10.0.0.10"
)

type adapterTestStore struct {
	mu             sync.Mutex
	snapshot       state.Snapshot
	replaceErr     error
	metadataErr    error
	statusErr      error
	closeErr       error
	closeCalls     int
	beforeReplace  func()
	beforeMetadata func()
}

func newAdapterTestStore(snapshot state.Snapshot) *adapterTestStore {
	cloned, err := cloneJSON(snapshot)
	if err != nil {
		panic(err)
	}
	return &adapterTestStore{snapshot: cloned}
}

func (s *adapterTestStore) operations() durableStateOperations {
	return durableStateOperations{
		snapshot: func(ctx context.Context) (state.Snapshot, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return state.Snapshot{}, fmt.Errorf("reading adapter test snapshot: %w", err)
			}
			return s.snapshot, nil
		},
		replace: func(ctx context.Context, expected uint64, durable state.DurableState) (bool, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.beforeReplace != nil {
				s.beforeReplace()
			}
			if err := ctx.Err(); err != nil {
				return false, fmt.Errorf("replacing adapter test state: %w", err)
			}
			if s.replaceErr != nil {
				return false, s.replaceErr
			}
			if s.snapshot.Metadata.Generation != expected {
				return false, state.ErrStaleGeneration
			}
			s.snapshot.NetworkContainers = durable.NetworkContainers
			s.snapshot.IPs = durable.IPs
			s.snapshot.Networks = durable.Networks
			s.snapshot.OrchestratorContexts = durable.OrchestratorContexts
			s.snapshot.PnPIDByMAC = durable.PnPIDByMAC
			s.snapshot.Metadata.Generation++
			return true, nil
		},
		updateMetadata: func(ctx context.Context, expected uint64, metadata state.Metadata) (bool, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.beforeMetadata != nil {
				s.beforeMetadata()
			}
			if err := ctx.Err(); err != nil {
				return false, fmt.Errorf("updating adapter test metadata: %w", err)
			}
			if s.metadataErr != nil {
				return false, s.metadataErr
			}
			if s.snapshot.Metadata.Generation != expected {
				return false, state.ErrStaleGeneration
			}
			metadata.Generation++
			s.snapshot.Metadata = metadata
			return true, nil
		},
		status: func(ctx context.Context) (state.Status, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return state.Status{}, fmt.Errorf("reading adapter test status: %w", err)
			}
			if s.statusErr != nil {
				return state.Status{}, s.statusErr
			}
			return healthyAdapterStatus(s.snapshot.Metadata.Generation), nil
		},
		close: func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.closeCalls++
			return s.closeErr
		},
	}
}

func TestDurableStateAdapterProjection(t *testing.T) {
	snapshot := completeAdapterSnapshot(7)
	store := newAdapterTestStore(snapshot)
	service := newAdapterTestService()
	originalEndpoint := service.EndpointState["existing-endpoint"]
	originalAssignments := append([]string{}, service.PodIPIDByPodInterfaceKey["existing-pod"]...)
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
	require.NoError(t, err)

	require.NoError(t, adapter.restore(context.Background()))
	assertAdapterProjection(t, service, snapshot)
	generation, projected := adapter.cacheGeneration()
	assert.True(t, projected)
	assert.Equal(t, uint64(7), generation)
	assert.NotSame(t, originalEndpoint, service.EndpointState["container-a"])
	assert.NotEqual(t, originalAssignments, service.PodIPIDByPodInterfaceKey["if-a"])
	assert.NotContains(t, service.EndpointState, "existing-endpoint")

	snapshot.NetworkContainers[adapterTestNCID] = state.NetworkContainerRecord{}
	snapshot.Networks["network-1"].Options["nested"].(map[string]any)["key"] = "mutated"
	snapshot.Assignments["if-a"].IPIDs[0] = "mutated"
	snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"].IPv4[0].IP[0] = 192
	assert.Equal(t, adapterTestNCID, service.state.ContainerStatus[adapterTestNCID].ID)
	assert.Equal(t, "value", service.state.Networks["network-1"].Options["nested"].(map[string]any)["key"])
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", service.PodIPIDByPodInterfaceKey["if-a"][0])
	assert.Equal(t, "10.0.0.4", service.EndpointState["container-a"].IfnameToIPMap["eth0"].IPv4[0].IP.String())

	service.state.Location = "same-generation-no-op"
	require.NoError(t, adapter.restore(context.Background()))
	assert.Equal(t, "same-generation-no-op", service.state.Location)

	before := durableCacheFingerprint(service, adapter)
	store.mu.Lock()
	store.snapshot.Metadata.Generation = 6
	store.mu.Unlock()
	require.ErrorIs(t, adapter.restore(context.Background()), state.ErrStaleGeneration)
	assert.Equal(t, before, durableCacheFingerprint(service, adapter))

	newer := completeAdapterSnapshot(8)
	newer.Metadata.Location = "new-location"
	store.mu.Lock()
	store.snapshot = newer
	store.mu.Unlock()
	require.NoError(t, adapter.restore(context.Background()))
	assert.Equal(t, "new-location", service.state.Location)
	assert.Equal(t, uint64(8), adapter.generation)
}

func TestDurableStateAdapterProjectionFailureIsAtomic(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*state.Snapshot)
	}{
		{
			name: "invalid host version",
			mutate: func(snapshot *state.Snapshot) {
				record := snapshot.NetworkContainers[adapterTestNCID]
				record.HostVersion = "not-a-version"
				snapshot.NetworkContainers[adapterTestNCID] = record
			},
		},
		{
			name: "late network clone failure",
			mutate: func(snapshot *state.Snapshot) {
				record := snapshot.Networks["network-1"]
				record.Options["unsupported"] = make(chan struct{})
				snapshot.Networks["network-1"] = record
			},
		},
		{
			name: "malformed endpoint prefix",
			mutate: func(snapshot *state.Snapshot) {
				snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"].IPv4[0].Mask = net.IPMask{0xff, 0}
			},
		},
		{
			name: "malformed endpoint MAC",
			mutate: func(snapshot *state.Snapshot) {
				snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"].MACAddress = "not-a-mac"
			},
		},
		{
			name: "nil endpoint interface",
			mutate: func(snapshot *state.Snapshot) {
				snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"] = nil
			},
		},
		{
			name: "missing endpoint",
			mutate: func(snapshot *state.Snapshot) {
				delete(snapshot.Endpoints, "container-a")
			},
		},
		{
			name: "missing assignment IP",
			mutate: func(snapshot *state.Snapshot) {
				delete(snapshot.IPs, "11111111-1111-1111-1111-111111111111")
			},
		},
		{
			name: "missing owner",
			mutate: func(snapshot *state.Snapshot) {
				delete(snapshot.IPOwners, "11111111-1111-1111-1111-111111111111")
			},
		},
		{
			name: "duplicate assignment ownership",
			mutate: func(snapshot *state.Snapshot) {
				assignment := snapshot.Assignments["if-a"]
				assignment.IPIDs = append(assignment.IPIDs, "44444444-4444-4444-4444-444444444444")
				snapshot.Assignments["if-a"] = assignment
			},
		},
		{
			name: "missing projection metadata",
			mutate: func(snapshot *state.Snapshot) {
				snapshot.Metadata.Authority = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newAdapterTestService()
			initial := completeAdapterSnapshot(1)
			store := newAdapterTestStore(initial)
			adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
			require.NoError(t, err)
			require.NoError(t, adapter.restore(context.Background()))
			before := durableCacheFingerprint(service, adapter)

			failed := completeAdapterSnapshot(2)
			tt.mutate(&failed)
			store.mu.Lock()
			store.snapshot = failed
			store.mu.Unlock()
			require.Error(t, adapter.restore(context.Background()))
			assert.Equal(t, before, durableCacheFingerprint(service, adapter))
		})
	}
}

func TestDurableStateAdapterEndpointProjectionDisabled(t *testing.T) {
	service := newAdapterTestService()
	originalEndpoint := service.EndpointState["existing-endpoint"]
	originalAssignments := append([]string(nil), service.PodIPIDByPodInterfaceKey["existing-pod"]...)
	store := newAdapterTestStore(completeAdapterSnapshot(1))
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), false)
	require.NoError(t, err)

	require.NoError(t, adapter.restore(context.Background()))
	assert.Len(t, service.state.ContainerStatus, 2)
	assert.Len(t, service.PodIPConfigState, 4)
	pending := service.PodIPConfigState["11111111-1111-1111-1111-111111111111"]
	assert.Equal(
		t,
		types.PendingProgramming,
		pending.GetState(),
	)
	assert.Nil(t, pending.PodInfo)
	available := service.PodIPConfigState["33333333-3333-3333-3333-333333333333"]
	assert.Equal(
		t,
		types.Available,
		available.GetState(),
	)
	assert.Same(t, originalEndpoint, service.EndpointState["existing-endpoint"])
	assert.Equal(t, originalAssignments, service.PodIPIDByPodInterfaceKey["existing-pod"])
	assert.Len(t, service.EndpointState, 1)
	assert.Len(t, service.PodIPIDByPodInterfaceKey, 1)
}

func TestDurableStateAdapterPreflightsEndpointProjectionBeforeDurableMutation(t *testing.T) {
	service := newAdapterTestService()
	store := newAdapterTestStore(completeAdapterSnapshot(1))
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
	require.NoError(t, err)
	require.NoError(t, adapter.restore(context.Background()))
	before := durableCacheFingerprint(service, adapter)

	store.mu.Lock()
	store.snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"].MACAddress = "invalid"
	generation := store.snapshot.Metadata.Generation
	store.mu.Unlock()

	err = adapter.putNetwork(context.Background(), state.NetworkRecord{NetworkName: "must-not-commit"})
	require.Error(t, err)
	assert.Equal(t, before, durableCacheFingerprint(service, adapter))
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, generation, store.snapshot.Metadata.Generation)
	assert.NotContains(t, store.snapshot.Networks, "must-not-commit")
}

func TestDurableStateAdapterCommitOrderingAndFailures(t *testing.T) {
	snapshot := completeAdapterSnapshot(3)
	service := newAdapterTestService()
	store := newAdapterTestStore(snapshot)
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
	require.NoError(t, err)
	require.NoError(t, adapter.restore(context.Background()))

	record := snapshot.NetworkContainers[adapterTestNCID]
	record.VMVersion = "9"
	record.HostVersion = "9"
	record.Request.Version = "9"
	ips := make([]state.IPRecord, 0, 3)
	for _, ip := range snapshot.IPs {
		if ip.NCID == "nc-1" {
			ips = append(ips, ip)
		}
	}
	ips = append(ips, state.IPRecord{
		ID:        "ip-new",
		IPAddress: "10.0.0.99",
		NCID:      adapterTestNCID,
		NCVersion: 9,
	})
	store.beforeReplace = func() {
		assert.Equal(t, "2", service.state.ContainerStatus[adapterTestNCID].VMVersion)
		assert.NotContains(t, service.PodIPConfigState, "ip-new")
	}
	require.NoError(t, adapter.applyNetworkContainer(context.Background(), record, ips))
	store.beforeReplace = nil
	assert.Equal(t, "9", service.state.ContainerStatus[adapterTestNCID].VMVersion)
	assert.Contains(t, service.PodIPConfigState, "ip-new")
	assert.Equal(t, uint64(4), adapter.generation)
	store.mu.Lock()
	assert.Equal(t, "9", store.snapshot.NetworkContainers[adapterTestNCID].VMVersion)
	assert.Contains(t, store.snapshot.IPs, "ip-new")
	store.mu.Unlock()

	failures := []struct {
		name string
		err  error
	}{
		{name: "commit", err: errAdapterCommitFailure},
		{name: "stale", err: state.ErrStaleGeneration},
		{name: "read only", err: bolterrors.ErrDatabaseReadOnly},
		{name: "canceled", err: context.Canceled},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			before := durableCacheFingerprint(service, adapter)
			store.mu.Lock()
			store.replaceErr = tt.err
			store.mu.Unlock()
			operationErr := adapter.putNetwork(context.Background(), state.NetworkRecord{NetworkName: "not-committed"})
			require.ErrorIs(t, operationErr, tt.err)
			assert.Equal(t, before, durableCacheFingerprint(service, adapter))
			store.mu.Lock()
			assert.NotContains(t, store.snapshot.Networks, "not-committed")
			store.replaceErr = nil
			store.mu.Unlock()
		})
	}

	before := durableCacheFingerprint(service, adapter)
	store.mu.Lock()
	store.metadataErr = errAdapterMetadataFailure
	store.mu.Unlock()
	err = adapter.putServiceMetadata(context.Background(), durableServiceMetadata{NodeID: "not-committed"})
	require.Error(t, err)
	assert.Equal(t, before, durableCacheFingerprint(service, adapter))
	store.mu.Lock()
	assert.NotEqual(t, "not-committed", store.snapshot.Metadata.NodeID)
	store.metadataErr = nil
	store.mu.Unlock()

	before = durableCacheFingerprint(service, adapter)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, adapter.putNetwork(ctx, state.NetworkRecord{NetworkName: "canceled"}), context.Canceled)
	assert.Equal(t, before, durableCacheFingerprint(service, adapter))
}

func TestDurableStateAdapterDurableOperations(t *testing.T) {
	service := newAdapterTestService()
	store := newAdapterTestStore(emptyAdapterSnapshot(0))
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
	require.NoError(t, err)
	require.NoError(t, adapter.restore(context.Background()))

	record := adapterNetworkContainer("nc")
	require.NoError(t, adapter.applyNetworkContainer(context.Background(), record, []state.IPRecord{{
		ID: "ip", IPAddress: adapterTestIPv4, NCID: "nc", NCVersion: 2,
	}}))
	require.NoError(t, adapter.putNetwork(context.Background(), state.NetworkRecord{
		NetworkName: "network",
		NicInfo:     &wireserver.InterfaceInfo{Subnet: adapterTestSubnet, SecondaryIPs: []string{adapterTestIPv4}},
		Options:     map[string]any{"mode": "bridge"},
	}))
	require.NoError(t, adapter.putOrchestratorContext(context.Background(), "pod", []string{"nc"}))
	require.NoError(t, adapter.putPnPID(context.Background(), "00-11-22-33-44-55", "pnp"))
	require.NoError(t, adapter.putServiceMetadata(context.Background(), durableServiceMetadata{
		OrchestratorType: cns.KubernetesCRD,
		NodeID:           "node",
		Location:         adapterTestLocation,
		NetworkType:      adapterTestNetworkType,
		Initialized:      true,
		TimeStamp:        time.Unix(100, 0).UTC(),
	}))

	assert.Contains(t, service.state.ContainerStatus, "nc")
	assert.Contains(t, service.PodIPConfigState, "ip")
	assert.Contains(t, service.state.Networks, "network")
	assert.Equal(t, ncList("nc"), *service.state.ContainerIDByOrchestratorContext["pod"])
	assert.Equal(t, "pnp", service.state.PnpIDByMacAddress["00:11:22:33:44:55"])
	assert.Equal(t, "node", service.state.NodeID)
	assert.Equal(t, uint64(5), adapter.generation)

	require.NoError(t, adapter.deleteOrchestratorContext(context.Background(), "pod"))
	require.NoError(t, adapter.deletePnPID(context.Background(), "00:11:22:33:44:55"))
	require.NoError(t, adapter.deleteNetwork(context.Background(), "network"))
	require.NoError(t, adapter.deleteNetworkContainer(context.Background(), "nc"))
	assert.Empty(t, service.state.ContainerStatus)
	assert.Empty(t, service.PodIPConfigState)
	assert.Empty(t, service.state.Networks)
	assert.Empty(t, service.state.ContainerIDByOrchestratorContext)
	assert.Empty(t, service.state.PnpIDByMacAddress)
}

func TestDurableStateAdapterStaleWritersDoNotLoseState(t *testing.T) {
	baseDir, err := os.MkdirTemp(".", ".durable-adapter-concurrency-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(baseDir)) })

	db, err := state.Open(filepath.Join(baseDir, "state.db"), state.Options{})
	require.NoError(t, err)
	first, err := newDurableStateAdapter(newAdapterTestService(), db, true)
	require.NoError(t, err)
	second, err := newDurableStateAdapter(newAdapterTestService(), db, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	require.NoError(t, first.restore(context.Background()))
	require.NoError(t, second.restore(context.Background()))
	firstBefore := durableCacheFingerprint(first.service, first)
	secondBefore := durableCacheFingerprint(second.service, second)
	start := make(chan struct{})
	results := make(chan struct {
		name    string
		adapter *durableStateAdapter
		err     error
	}, 2)
	var writers sync.WaitGroup
	writers.Add(2)
	go func() {
		defer writers.Done()
		<-start
		results <- struct {
			name    string
			adapter *durableStateAdapter
			err     error
		}{"first", first, first.putNetwork(context.Background(), state.NetworkRecord{NetworkName: "first"})}
	}()
	go func() {
		defer writers.Done()
		<-start
		results <- struct {
			name    string
			adapter *durableStateAdapter
			err     error
		}{"second", second, second.putNetwork(context.Background(), state.NetworkRecord{NetworkName: "second"})}
	}()
	close(start)
	writers.Wait()
	close(results)

	var staleWriter struct {
		name    string
		adapter *durableStateAdapter
	}

	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, result.err, state.ErrStaleGeneration)
		staleWriter.name = result.name
		staleWriter.adapter = result.adapter
	}
	require.Equal(t, 1, successes)
	require.NotNil(t, staleWriter.adapter)
	if staleWriter.adapter == first {
		assert.Equal(t, firstBefore, durableCacheFingerprint(first.service, first))
	} else {
		assert.Equal(t, secondBefore, durableCacheFingerprint(second.service, second))
	}

	require.NoError(t, staleWriter.adapter.restore(context.Background()))
	require.NoError(
		t,
		staleWriter.adapter.putNetwork(context.Background(), state.NetworkRecord{NetworkName: staleWriter.name}),
	)
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Contains(t, snapshot.Networks, "first")
	assert.Contains(t, snapshot.Networks, "second")
}

func TestDurableStateAdapterConcurrentRestore(t *testing.T) {
	store := newAdapterTestStore(completeAdapterSnapshot(1))
	service := newAdapterTestService()
	adapter, err := newDurableStateAdapterWithOperations(service, store.operations(), true)
	require.NoError(t, err)
	require.NoError(t, adapter.restore(context.Background()))

	newer := completeAdapterSnapshot(2)
	newer.Metadata.Location = "new-location"
	newer.Endpoints["container-a"].IfnameToIPMap["eth0"].HostVethName = "new-veth"
	store.mu.Lock()
	store.snapshot = newer
	store.mu.Unlock()

	const restoreCount = 8
	start := make(chan struct{})
	errs := make(chan error, restoreCount)
	var restores sync.WaitGroup
	restores.Add(restoreCount)
	for range restoreCount {
		go func() {
			defer restores.Done()
			<-start
			errs <- adapter.restore(context.Background())
		}()
	}
	close(start)
	restores.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, uint64(2), adapter.generation)
	assert.Equal(t, "new-location", service.state.Location)
	assert.Equal(t, "new-veth", service.EndpointState["container-a"].IfnameToIPMap["eth0"].HostVethName)
	assertAdapterProjection(t, service, newer)
}

func TestDurableStateAdapterRestartRestoreAndClose(t *testing.T) {
	baseDir, err := os.MkdirTemp(".", ".durable-adapter-restart-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(baseDir)) })
	path := filepath.Join(baseDir, "state.db")

	db, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	firstService := newAdapterTestService()
	first, err := newDurableStateAdapter(firstService, db, true)
	require.NoError(t, err)
	require.NoError(t, first.restore(context.Background()))
	session := completeAdapterSnapshot(0)
	for _, ncID := range []string{adapterTestNCID, "nc-2"} {
		record := session.NetworkContainers[ncID]
		record.Request.AuthorizationToken = "must-not-persist"
		record.Request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{
			"embedded": {IPAddress: "10.0.0.8"},
		}
		var ips []state.IPRecord
		for _, ip := range session.IPs {
			if ip.NCID == ncID {
				ips = append(ips, ip)
			}
		}
		require.NoError(t, first.applyNetworkContainer(context.Background(), record, ips))
	}
	for _, podKey := range []string{"if-a", "container-b"} {
		assignment := session.Assignments[podKey]
		changed, assignErr := db.AssignEndpoint(
			context.Background(),
			assignment,
			session.Endpoints[assignment.Pod.InfraContainerID],
			time.Unix(200, 0).UTC(),
			time.Hour,
		)
		require.NoError(t, assignErr)
		require.True(t, changed)
	}
	require.NoError(t, first.restore(context.Background()))
	require.NoError(t, first.putNetwork(context.Background(), state.NetworkRecord{
		NetworkName: "network",
		NicInfo:     &wireserver.InterfaceInfo{Subnet: adapterTestSubnet},
		Options:     map[string]any{"nested": map[string]any{"enabled": true}},
	}))
	require.NoError(t, first.putOrchestratorContext(context.Background(), "pod", []string{"nc-1", "nc-2"}))
	require.NoError(t, first.putPnPID(context.Background(), "00-11-22-33-44-55", "pnp"))
	require.NoError(t, first.putServiceMetadata(context.Background(), durableServiceMetadata{
		OrchestratorType: cns.KubernetesCRD,
		NodeID:           "node",
		Location:         adapterTestLocation,
		NetworkType:      adapterTestNetworkType,
		Initialized:      true,
		TimeStamp:        time.Unix(100, 0).UTC(),
	}))
	require.NoError(t, db.Update(context.Background(), func(tx *state.WriteTx) error {
		return tx.PutDeleteIntent("delete-only", state.DeleteIntent{CreatedAt: time.Unix(300, 0).UTC()})
	}))
	require.NoError(t, first.restore(context.Background()))
	require.NoError(t, first.putNetwork(context.Background(), state.NetworkRecord{NetworkName: "intent-preserving"}))
	snapshotBeforeRestart, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Contains(t, snapshotBeforeRestart.DeleteIntents, "delete-only")
	want := durableCacheFingerprint(firstService, first)
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())

	reopened, err := state.Open(path, state.Options{})
	require.NoError(t, err)
	secondService := newAdapterTestService()
	second, err := newDurableStateAdapter(secondService, reopened, true)
	require.NoError(t, err)
	require.NoError(t, second.restore(context.Background()))
	assert.Equal(t, want, durableCacheFingerprint(secondService, second))
	assert.Empty(t, secondService.state.ContainerStatus["nc-1"].CreateNetworkContainerRequest.AuthorizationToken)
	assert.NotContains(
		t,
		secondService.state.ContainerStatus["nc-1"].CreateNetworkContainerRequest.SecondaryIPConfigs,
		"embedded",
	)
	restartedSnapshot, err := reopened.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Contains(t, restartedSnapshot.DeleteIntents, "delete-only")
	require.NoError(t, second.Close())
}

func TestDurableStateAdapterCloseErrorIsStable(t *testing.T) {
	store := newAdapterTestStore(emptyAdapterSnapshot(0))
	store.closeErr = errAdapterCloseFailure
	adapter, err := newDurableStateAdapterWithOperations(newAdapterTestService(), store.operations(), true)
	require.NoError(t, err)
	require.ErrorIs(t, adapter.Close(), errAdapterCloseFailure)
	require.ErrorIs(t, adapter.Close(), errAdapterCloseFailure)
	assert.Equal(t, 1, store.closeCalls)
}

func TestDurableStateAdapterConstructorValidation(t *testing.T) {
	service := newAdapterTestService()
	store := newAdapterTestStore(emptyAdapterSnapshot(0))
	operations := store.operations()

	adapter, err := newDurableStateAdapterWithOperations(nil, operations, true)
	require.Error(t, err)
	assert.Nil(t, adapter)
	adapter, err = newDurableStateAdapter(service, nil, true)
	require.Error(t, err)
	assert.Nil(t, adapter)

	tests := []struct {
		name   string
		mutate func(*durableStateOperations)
	}{
		{name: "snapshot", mutate: func(ops *durableStateOperations) { ops.snapshot = nil }},
		{name: "replace", mutate: func(ops *durableStateOperations) { ops.replace = nil }},
		{name: "metadata", mutate: func(ops *durableStateOperations) { ops.updateMetadata = nil }},
		{name: "status", mutate: func(ops *durableStateOperations) { ops.status = nil }},
		{name: "close", mutate: func(ops *durableStateOperations) { ops.close = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := operations
			tt.mutate(&invalid)
			adapter, err := newDurableStateAdapterWithOperations(service, invalid, true)
			require.Error(t, err)
			assert.Nil(t, adapter)
		})
	}
}

func newAdapterTestService() *HTTPRestService {
	return &HTTPRestService{
		state: &httpRestServiceState{
			ContainerStatus:                  map[string]containerstatus{"old": {ID: "old"}},
			ContainerIDByOrchestratorContext: map[string]*ncList{},
			Networks:                         map[string]*networkInfo{},
			joinedNetworks:                   map[string]struct{}{"joined": {}},
			PnpIDByMacAddress:                map[string]string{},
		},
		PodIPConfigState: map[string]cns.IPConfigurationStatus{},
		PodIPIDByPodInterfaceKey: map[string][]string{
			"existing-pod": {"existing-ip"},
		},
		EndpointState: map[string]*EndpointInfo{
			"existing-endpoint": {PodName: "existing"},
		},
	}
}

func completeAdapterSnapshot(generation uint64) state.Snapshot {
	snapshot := emptyAdapterSnapshot(generation)
	snapshot.Metadata.OrchestratorType = cns.KubernetesCRD
	snapshot.Metadata.NodeID = "node-1"
	snapshot.Metadata.Location = adapterTestLocation
	snapshot.Metadata.NetworkType = adapterTestNetworkType
	snapshot.Metadata.Initialized = true
	snapshot.Metadata.TimeStamp = time.Unix(50, 0).UTC()
	snapshot.NetworkContainers[adapterTestNCID] = adapterNetworkContainer(adapterTestNCID)
	second := adapterNetworkContainer("nc-2")
	second.HostVersion = "3"
	snapshot.NetworkContainers["nc-2"] = second
	snapshot.IPs["11111111-1111-1111-1111-111111111111"] = state.IPRecord{
		ID:        "11111111-1111-1111-1111-111111111111",
		IPAddress: adapterTestIPv4,
		NCID:      adapterTestNCID,
		NCVersion: 2,
	}
	snapshot.IPs["22222222-2222-2222-2222-222222222222"] = state.IPRecord{
		ID:        "22222222-2222-2222-2222-222222222222",
		IPAddress: "2001:db8::4",
		NCID:      "nc-1",
		NCVersion: 2,
	}
	snapshot.IPs["33333333-3333-3333-3333-333333333333"] = state.IPRecord{
		ID:        "33333333-3333-3333-3333-333333333333",
		IPAddress: "10.0.1.4",
		NCID:      "nc-2",
		NCVersion: 2,
	}
	snapshot.IPs["44444444-4444-4444-4444-444444444444"] = state.IPRecord{
		ID:        "44444444-4444-4444-4444-444444444444",
		IPAddress: "10.0.1.5",
		NCID:      "nc-2",
		NCVersion: 2,
	}
	snapshot.Networks["network-1"] = state.NetworkRecord{
		NetworkName: "network-1",
		NicInfo: &wireserver.InterfaceInfo{
			Subnet:       adapterTestSubnet,
			Gateway:      gatewayIP,
			PrimaryIP:    "10.0.0.2",
			SecondaryIPs: []string{adapterTestIPv4},
		},
		Options: map[string]any{"nested": map[string]any{"key": "value"}},
	}
	snapshot.OrchestratorContexts["pod-a"] = []string{"nc-1", "nc-2"}
	snapshot.PnPIDByMAC["00:11:22:33:44:55"] = "pnp-1"
	snapshot.Endpoints["container-a"] = state.EndpointRecord{
		PodName:      "pod-a",
		PodNamespace: "namespace-a",
		IfnameToIPMap: map[string]*state.IPInfoRecord{
			"eth0": {
				IPv4:               []net.IPNet{testIPNet("10.0.0.4/24")},
				IPv6:               []net.IPNet{testIPNet("2001:db8::4/64")},
				HNSEndpointID:      "hns-endpoint-a",
				HNSNetworkID:       "hns-network-a",
				HostVethName:       "veth-a",
				MACAddress:         "00:11:22:33:44:55",
				NetworkContainerID: "nc-1",
				NICType:            cns.InfraNIC,
			},
			"net1": {
				IPv4:               []net.IPNet{testIPNet("10.0.1.4/32")},
				HNSEndpointID:      "hns-endpoint-b",
				HNSNetworkID:       "hns-network-b",
				HostVethName:       "veth-b",
				MACAddress:         "00:11:22:33:44:66",
				NetworkContainerID: "nc-2",
				NICType:            cns.DelegatedVMNIC,
			},
		},
	}
	snapshot.Assignments["if-a"] = state.AssignmentRecord{
		Pod: state.PodIdentity{
			PodKey:           "if-a",
			InfraContainerID: "container-a",
			InterfaceID:      "if-a",
			PodName:          "pod-a",
			PodNamespace:     "namespace-a",
		},
		IPIDs: []string{
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222",
			"33333333-3333-3333-3333-333333333333",
		},
	}
	snapshot.Endpoints["container-b"] = state.EndpointRecord{
		PodName:      "pod-b",
		PodNamespace: "namespace-b",
		IfnameToIPMap: map[string]*state.IPInfoRecord{
			"eth0": {
				IPv4:               []net.IPNet{testIPNet("10.0.1.5/24")},
				NetworkContainerID: "nc-2",
				NICType:            cns.InfraNIC,
			},
		},
	}
	snapshot.Assignments["container-b"] = state.AssignmentRecord{
		Pod: state.PodIdentity{
			PodKey:           "container-b",
			InfraContainerID: "container-b",
			PodName:          "pod-b",
			PodNamespace:     "namespace-b",
		},
		IPIDs: []string{"44444444-4444-4444-4444-444444444444"},
	}
	for podKey, assignment := range snapshot.Assignments {
		for _, ipID := range assignment.IPIDs {
			snapshot.IPOwners[ipID] = podKey
		}
	}
	return snapshot
}

func testIPNet(prefix string) net.IPNet {
	ip, network, err := net.ParseCIDR(prefix)
	if err != nil {
		panic(err)
	}
	network.IP = ip
	return *network
}

func emptyAdapterSnapshot(generation uint64) state.Snapshot {
	snapshot := state.NewSnapshot()
	snapshot.Metadata = state.Metadata{
		SchemaVersion: state.SchemaVersion,
		Authority:     state.AuthorityBolt,
		Generation:    generation,
	}
	return snapshot
}

func adapterNetworkContainer(id string) state.NetworkContainerRecord {
	return state.NewNetworkContainerRecord(id, "2", "1", true, cns.CreateNetworkContainerRequest{
		NetworkContainerid: id,
		Version:            "2",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         cns.IPSubnet{IPAddress: adapterTestNetwork, PrefixLength: 24},
			GatewayIPAddress: gatewayIP,
			DNSServers:       []string{adapterTestDNS},
		},
		Routes: []cns.Route{{IPAddress: "0.0.0.0/0", GatewayIPAddress: gatewayIP}},
	})
}

func healthyAdapterStatus(generation uint64) state.Status {
	return state.Status{
		Backend:         state.BackendBolt,
		Authority:       state.AuthorityBolt,
		SchemaVersion:   state.SchemaVersion,
		Generation:      generation,
		InvariantStatus: state.InvariantHealthy,
	}
}

func assertAdapterProjection(t *testing.T, service *HTTPRestService, snapshot state.Snapshot) {
	t.Helper()
	require.Len(t, service.state.ContainerStatus, 2)
	assert.Equal(t, snapshot.Metadata.NodeID, service.state.NodeID)
	assert.Equal(t, snapshot.Metadata.OrchestratorType, service.state.OrchestratorType)
	assert.Equal(t, snapshot.Metadata.TimeStamp, service.state.TimeStamp)
	assert.Equal(
		t,
		snapshot.NetworkContainers[adapterTestNCID].Request.Routes,
		service.state.ContainerStatus[adapterTestNCID].CreateNetworkContainerRequest.Routes,
	)
	assert.Empty(t, service.state.ContainerStatus[adapterTestNCID].CreateNetworkContainerRequest.AuthorizationToken)
	require.Len(t, service.PodIPConfigState, 4)
	for id := range service.PodIPConfigState {
		status := service.PodIPConfigState[id]
		assert.Equal(t, types.Assigned, status.GetState(), id)
		require.NotNil(t, status.PodInfo, id)
	}
	assert.Equal(t, "if-a", service.PodIPConfigState["11111111-1111-1111-1111-111111111111"].PodInfo.Key())
	assert.Equal(t, "container-a", service.PodIPConfigState["11111111-1111-1111-1111-111111111111"].PodInfo.InfraContainerID())
	assert.Equal(t, "if-a", service.PodIPConfigState["11111111-1111-1111-1111-111111111111"].PodInfo.InterfaceID())
	assert.True(t, service.PodIPConfigState["11111111-1111-1111-1111-111111111111"].PodInfo.SecondaryInterfacesExist())
	assert.Equal(t, "container-b", service.PodIPConfigState["44444444-4444-4444-4444-444444444444"].PodInfo.Key())
	assert.False(t, service.PodIPConfigState["44444444-4444-4444-4444-444444444444"].PodInfo.SecondaryInterfacesExist())
	assert.Equal(
		t,
		adapterTestIPv4,
		service.state.ContainerStatus[adapterTestNCID].CreateNetworkContainerRequest.
			SecondaryIPConfigs["11111111-1111-1111-1111-111111111111"].IPAddress,
	)
	assert.Equal(t, snapshot.Networks["network-1"].NicInfo, service.state.Networks["network-1"].NicInfo)
	assert.Equal(t, snapshot.Networks["network-1"].Options, service.state.Networks["network-1"].Options)
	assert.Equal(t, ncList("nc-1,nc-2"), *service.state.ContainerIDByOrchestratorContext["pod-a"])
	assert.Equal(t, "pnp-1", service.state.PnpIDByMacAddress["00:11:22:33:44:55"])
	assert.Equal(
		t,
		[]string{
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222",
			"33333333-3333-3333-3333-333333333333",
		},
		service.PodIPIDByPodInterfaceKey["if-a"],
	)
	require.Len(t, service.EndpointState, 2)
	eth0 := service.EndpointState["container-a"].IfnameToIPMap["eth0"]
	wantEth0 := snapshot.Endpoints["container-a"].IfnameToIPMap["eth0"]
	assert.Equal(t, wantEth0.IPv4, eth0.IPv4)
	assert.Equal(t, wantEth0.IPv6, eth0.IPv6)
	assert.Equal(t, wantEth0.HNSEndpointID, eth0.HnsEndpointID)
	assert.Equal(t, wantEth0.HNSNetworkID, eth0.HnsNetworkID)
	assert.Equal(t, wantEth0.HostVethName, eth0.HostVethName)
	assert.Equal(t, wantEth0.MACAddress, eth0.MacAddress)
	assert.Equal(t, wantEth0.NetworkContainerID, eth0.NetworkContainerID)
	assert.Equal(t, wantEth0.NICType, eth0.NICType)
	assert.Equal(t, cns.DelegatedVMNIC, service.EndpointState["container-a"].IfnameToIPMap["net1"].NICType)
}

type adapterCacheFingerprint struct {
	Generation      uint64
	Projected       bool
	Metadata        durableServiceMetadata
	Containers      map[string]containerstatus
	IPs             map[string]adapterIPFingerprint
	Networks        map[string]state.NetworkRecord
	OrchestratorNCs map[string]string
	PnPIDs          map[string]string
	Assignments     map[string][]string
	Endpoints       map[string]*EndpointInfo
}

type adapterIPFingerprint struct {
	ID        string
	IPAddress string
	NCID      string
	State     types.IPState
	PodKey    string
	InfraID   string
	Interface string
	Secondary bool
}

func durableCacheFingerprint(service *HTTPRestService, adapter *durableStateAdapter) adapterCacheFingerprint {
	generation, projected := adapter.cacheGeneration()
	containers, err := cloneJSON(service.state.ContainerStatus)
	if err != nil {
		panic(err)
	}
	fingerprint := adapterCacheFingerprint{
		Generation: generation,
		Projected:  projected,
		Metadata: durableServiceMetadata{
			OrchestratorType: service.state.OrchestratorType,
			NodeID:           service.state.NodeID,
			Location:         service.state.Location,
			NetworkType:      service.state.NetworkType,
			Initialized:      service.state.Initialized,
			TimeStamp:        service.state.TimeStamp,
		},
		Containers:      containers,
		IPs:             make(map[string]adapterIPFingerprint, len(service.PodIPConfigState)),
		Networks:        make(map[string]state.NetworkRecord, len(service.state.Networks)),
		OrchestratorNCs: make(map[string]string, len(service.state.ContainerIDByOrchestratorContext)),
		PnPIDs:          make(map[string]string, len(service.state.PnpIDByMacAddress)),
		Assignments:     make(map[string][]string, len(service.PodIPIDByPodInterfaceKey)),
	}
	fingerprint.Endpoints, err = cloneJSON(service.EndpointState)
	if err != nil {
		panic(err)
	}
	for id := range service.PodIPConfigState {
		status := service.PodIPConfigState[id]
		value := adapterIPFingerprint{
			ID: status.ID, IPAddress: status.IPAddress, NCID: status.NCID, State: status.GetState(),
		}
		if status.PodInfo != nil {
			value.PodKey = status.PodInfo.Key()
			value.InfraID = status.PodInfo.InfraContainerID()
			value.Interface = status.PodInfo.InterfaceID()
			value.Secondary = status.PodInfo.SecondaryInterfacesExist()
		}
		fingerprint.IPs[id] = value
	}
	for name, network := range service.state.Networks {
		record, err := cloneJSON(state.NetworkRecord{
			NetworkName: network.NetworkName,
			NicInfo:     network.NicInfo,
			Options:     network.Options,
		})
		if err != nil {
			panic(err)
		}
		fingerprint.Networks[name] = record
	}
	for id, ncs := range service.state.ContainerIDByOrchestratorContext {
		fingerprint.OrchestratorNCs[id] = string(*ncs)
	}
	for macAddress, pnpID := range service.state.PnpIDByMacAddress {
		fingerprint.PnPIDs[macAddress] = pnpID
	}
	for podKey, ipIDs := range service.PodIPIDByPodInterfaceKey {
		fingerprint.Assignments[podKey] = append([]string(nil), ipIDs...)
	}
	return fingerprint
}

func TestBuildDurableCacheProjectionRejectsInvalidIPFamily(t *testing.T) {
	snapshot := completeAdapterSnapshot(1)
	snapshot.IPs["11111111-1111-1111-1111-111111111111"] = state.IPRecord{
		ID: "11111111-1111-1111-1111-111111111111", IPAddress: net.IP{}.String(), NCID: adapterTestNCID,
	}
	_, err := buildDurableCacheProjection(snapshot)
	require.Error(t, err)
}
