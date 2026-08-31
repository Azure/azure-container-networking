// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjectedCNIImport = errors.New("injected commit failure")

const cniImportTestSecondaryInterface = "net1"

func TestCNIImportCountsRejectOverflow(t *testing.T) {
	_, err := cniImportCounts(NewSnapshot(), math.MaxInt)
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCNIEndpointImportPreflightAndAtomicImport(t *testing.T) {
	db, _ := openTestDB(t)
	seedCNIImportInventory(t, db)
	records := cniImportRecords(t)

	plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), plan.ExpectedGeneration)
	assert.Equal(t, CNIImportCounts{
		Containers:  1,
		Interfaces:  2,
		Assignments: 2,
		IPs:         3,
	}, plan.Counts)
	require.Len(t, plan.IdentityDigest, 64)
	assert.True(t, sort.StringsAreSorted(plan.identities))
	publicPlan, err := json.Marshal(plan) //nolint:musttag // The public preflight intentionally exposes only its exported summary fields.
	require.NoError(t, err)
	assert.NotContains(t, string(publicPlan), "10.0.0.4")
	assert.NotContains(t, string(publicPlan), "pod-1")
	assert.NotContains(t, string(publicPlan), "path")

	reordered := []cns.CNIEndpointState{records[1], records[0]}
	reorderedPlan, err := db.PreflightCNIEndpointImport(context.Background(), reordered, false)
	require.NoError(t, err)
	assert.Equal(t, plan.IdentityDigest, reorderedPlan.IdentityDigest)

	projected, err := plan.SnapshotForProjection(requireValidSnapshot(t, db))
	require.NoError(t, err)
	assert.Equal(t, []string{"ip-v4", "ip-v6"}, projected.Assignments["iface-primary"].IPIDs)
	assert.Equal(t, "iface-secondary", projected.IPOwners["ip-secondary"])
	assert.Equal(t, "nc-1", projected.Endpoints["container-1"].IfnameToIPMap["eth0"].NetworkContainerID)
	projected.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0].IP[0] = 192
	fresh, err := plan.SnapshotForProjection(requireValidSnapshot(t, db))
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.4", fresh.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0].IP.String())

	before := requireValidSnapshot(t, db)
	changed, err := db.ImportCNIEndpointState(context.Background(), records, plan)
	require.NoError(t, err)
	require.True(t, changed)
	after := requireValidSnapshot(t, db)
	assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
	assert.Equal(t, before.NetworkContainers, after.NetworkContainers)
	assert.Equal(t, before.IPs, after.IPs)
	assert.Equal(t, before.Networks, after.Networks)
	assert.Equal(t, before.OrchestratorContexts, after.OrchestratorContexts)
	assert.Equal(t, before.PnPIDByMAC, after.PnPIDByMAC)
	assert.Equal(t, fresh.Endpoints["container-1"].PodName, after.Endpoints["container-1"].PodName)
	assert.Equal(t, "10.0.0.4", after.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0].IP.String())
	assert.Equal(t, "fd00::4", after.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv6[0].IP.String())
	assert.Equal(t, "10.1.0.4", after.Endpoints["container-1"].IfnameToIPMap["eth1"].IPv4[0].IP.String())
	assert.Equal(t, fresh.Assignments, after.Assignments)
	assert.Equal(t, fresh.IPOwners, after.IPOwners)
	assert.Empty(t, after.DeleteIntents)

	replay, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
	require.NoError(t, err)
	changed, err = db.ImportCNIEndpointState(context.Background(), records, replay)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, after.Metadata.Generation, readMetadata(t, db).Generation)
}

func TestCNIEndpointImportValidation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *DB, []cns.CNIEndpointState)
		mutate  func([]cns.CNIEndpointState) []cns.CNIEndpointState
		want    error
	}{
		{
			name: "missing inventory",
			mutate: func(records []cns.CNIEndpointState) []cns.CNIEndpointState {
				records[0].IPAddresses[0] = testCNIIPNet(t, "10.0.0.99/24")
				return records
			},
			want: ErrInvalidInput,
		},
		{
			name: "duplicate inventory",
			prepare: func(t *testing.T, db *DB, _ []cns.CNIEndpointState) {
				putRaw(t, db, bucketIPs, []byte("duplicate"), []byte(
					`{"id":"duplicate","ipAddress":"10.0.0.4","ncID":"nc-1","ncVersion":7}`,
				))
			},
			want: ErrInconsistentState,
		},
		{
			name: "active delete intent",
			prepare: func(t *testing.T, db *DB, _ []cns.CNIEndpointState) {
				require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
					return tx.PutDeleteIntent("old", DeleteIntent{CreatedAt: time.Now()})
				}))
			},
			want: ErrDeleteIntent,
		},
		{
			name: "conflicting container pod",
			mutate: func(records []cns.CNIEndpointState) []cns.CNIEndpointState {
				records[1].PodName = "other"
				return records
			},
			want: ErrInvalidInput,
		},
		{
			name: "duplicate interface",
			mutate: func(records []cns.CNIEndpointState) []cns.CNIEndpointState {
				records[1].InterfaceName = "eth0"
				return records
			},
			want: ErrInvalidInput,
		},
		{
			name: "duplicate endpoint",
			mutate: func(records []cns.CNIEndpointState) []cns.CNIEndpointState {
				records[1].PodEndpointID = records[0].PodEndpointID
				return records
			},
			want: ErrInvalidInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			seedCNIImportInventory(t, db)
			records := cniImportRecords(t)
			if tt.prepare != nil {
				tt.prepare(t, db, records)
			}
			if tt.mutate != nil {
				records = tt.mutate(records)
			}
			_, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestCNIEndpointImportInputErrors(t *testing.T) {
	db, _ := openTestDB(t)
	seedCNIImportInventory(t, db)
	records := cniImportRecords(t)
	_, err := db.PreflightCNIEndpointImport(nil, records, false) //nolint:staticcheck // Verifies the fail-closed nil-context guard.
	require.Error(t, err)
	_, err = db.ImportCNIEndpointState(nil, records, CNIImportPreflight{}) //nolint:staticcheck // Verifies the fail-closed nil-context guard.
	require.Error(t, err)
	_, err = (CNIImportPreflight{}).SnapshotForProjection(NewSnapshot())
	require.ErrorIs(t, err, ErrInvalidInput)

	plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
	require.NoError(t, err)
	base := requireValidSnapshot(t, db)
	base.Metadata.Generation++
	_, err = plan.SnapshotForProjection(base)
	require.ErrorIs(t, err, ErrStaleGeneration)

	tests := []struct {
		name   string
		mutate func([]cns.CNIEndpointState)
	}{
		{name: "empty container", mutate: func(records []cns.CNIEndpointState) {
			records[0].InfraContainerID = ""
		}},
		{name: "empty endpoint", mutate: func(records []cns.CNIEndpointState) {
			records[0].PodEndpointID = ""
		}},
		{name: "empty pod name", mutate: func(records []cns.CNIEndpointState) {
			records[0].PodName = ""
		}},
		{name: "empty pod namespace", mutate: func(records []cns.CNIEndpointState) {
			records[0].PodNamespace = ""
		}},
		{name: "empty interface key", mutate: func(records []cns.CNIEndpointState) {
			records[0].InterfaceKey = ""
		}},
		{name: "empty interface name", mutate: func(records []cns.CNIEndpointState) {
			records[0].InterfaceName = ""
		}},
		{name: "empty IPs", mutate: func(records []cns.CNIEndpointState) {
			records[0].IPAddresses = []net.IPNet{}
		}},
		{name: "duplicate interface key", mutate: func(records []cns.CNIEndpointState) {
			records[1].InterfaceKey = records[0].InterfaceKey
		}},
		{name: "duplicate address", mutate: func(records []cns.CNIEndpointState) {
			records[1].IPAddresses = append(records[1].IPAddresses, records[0].IPAddresses[0])
		}},
		{name: "invalid address", mutate: func(records []cns.CNIEndpointState) {
			records[0].IPAddresses[0].IP = net.IP{1, 2}
		}},
		{name: "invalid mask", mutate: func(records []cns.CNIEndpointState) {
			records[0].IPAddresses[0].Mask = net.CIDRMask(24, 32)
		}},
		{name: "pod spans containers", mutate: func(records []cns.CNIEndpointState) {
			records[1].InfraContainerID = "container-2"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := cniImportRecords(t)
			tt.mutate(input)
			_, err := db.PreflightCNIEndpointImport(context.Background(), input, false)
			require.Error(t, err)
		})
	}
}

func TestCNIEndpointImportReplacementStaleTamperedAndCommitFailure(t *testing.T) {
	t.Run("replacement requires explicit mode", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		_, err = db.ImportCNIEndpointState(context.Background(), records, plan)
		require.NoError(t, err)

		changedRecords := cniImportRecords(t)
		changedRecords[1].InterfaceName = cniImportTestSecondaryInterface
		_, err = db.PreflightCNIEndpointImport(context.Background(), changedRecords, false)
		require.ErrorIs(t, err, ErrCNIImportConflict)
		fresh, err := db.PreflightCNIEndpointImport(context.Background(), changedRecords, true)
		require.NoError(t, err)
		changed, err := db.ImportCNIEndpointState(context.Background(), changedRecords, fresh)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Contains(
			t,
			requireValidSnapshot(t, db).Endpoints["container-1"].IfnameToIPMap,
			cniImportTestSecondaryInterface,
		)
	})

	t.Run("stale and tampered", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		tampered := plan
		tampered.IdentityDigest = "modified"
		_, err = db.ImportCNIEndpointState(context.Background(), records, tampered)
		require.ErrorIs(t, err, ErrInvalidInput)

		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			meta, metadataErr := tx.Metadata()
			if metadataErr != nil {
				return metadataErr
			}
			meta.NodeID = "changed"
			return tx.PutMetadata(meta)
		}))
		_, err = db.ImportCNIEndpointState(context.Background(), records, plan)
		require.ErrorIs(t, err, ErrStaleGeneration)
	})

	t.Run("commit failure", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		changed, err := db.importCNIEndpointState(
			context.Background(),
			records,
			plan,
			func() error { return errInjectedCNIImport },
		)
		require.ErrorIs(t, err, errInjectedCNIImport)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})
}

func TestCNIEndpointImportConcurrentCloseReadOnlyCancelAndReopen(t *testing.T) {
	t.Run("one concurrent import", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := db.ImportCNIEndpointState(context.Background(), records, plan)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		successes := 0
		stale := 0
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrStaleGeneration):
				stale++
			default:
				t.Fatalf("unexpected import error: %v", err)
			}
		}
		assert.Equal(t, 1, successes)
		assert.Equal(t, 7, stale)
	})

	t.Run("canceled", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := db.PreflightCNIEndpointImport(ctx, cniImportRecords(t), false)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("canceled at writer gate", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		db.writeGate <- struct{}{}
		ctx, cancel := context.WithCancel(context.Background())
		errs := make(chan error, 1)
		go func() {
			_, importErr := db.ImportCNIEndpointState(ctx, records, plan)
			errs <- importErr
		}()
		select {
		case earlyErr := <-errs:
			t.Fatalf("import returned while writer gate was held: %v", earlyErr)
		case <-time.After(10 * time.Millisecond):
		}
		cancel()
		err = <-errs
		<-db.writeGate
		require.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, requireValidSnapshot(t, db).Endpoints)
	})

	t.Run("closed", func(t *testing.T) {
		db, _ := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		require.NoError(t, db.Close())
		_, err = db.ImportCNIEndpointState(context.Background(), records, plan)
		require.Error(t, err)
	})

	t.Run("read only", func(t *testing.T) {
		db, path := openTestDB(t)
		seedCNIImportInventory(t, db)
		require.NoError(t, db.Close())
		readOnly, err := Open(path, Options{ReadOnly: true})
		require.NoError(t, err)
		defer readOnly.Close()
		records := cniImportRecords(t)
		plan, err := readOnly.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		_, err = readOnly.ImportCNIEndpointState(context.Background(), records, plan)
		require.Error(t, err)
	})

	t.Run("close reopen", func(t *testing.T) {
		db, path := openTestDB(t)
		seedCNIImportInventory(t, db)
		records := cniImportRecords(t)
		plan, err := db.PreflightCNIEndpointImport(context.Background(), records, false)
		require.NoError(t, err)
		_, err = db.ImportCNIEndpointState(context.Background(), records, plan)
		require.NoError(t, err)
		require.NoError(t, db.Close())
		reopened, err := Open(path, Options{})
		require.NoError(t, err)
		defer reopened.Close()
		snapshot := requireValidSnapshot(t, reopened)
		assert.Equal(t, "iface-primary", snapshot.IPOwners["ip-v4"])
	})
}

func seedCNIImportInventory(t *testing.T, db *DB) {
	t.Helper()
	snapshot := completeSnapshot()
	snapshot.Assignments = map[string]AssignmentRecord{}
	snapshot.IPOwners = map[string]string{}
	snapshot.Endpoints = map[string]EndpointRecord{}
	snapshot.DeleteIntents = map[string]DeleteIntent{}
	ips := make([]IPRecord, 0, len(snapshot.IPs))
	for _, id := range sortedKeys(snapshot.IPs) {
		ips = append(ips, snapshot.IPs[id])
	}
	changed, err := db.ApplyNetworkContainer(
		context.Background(),
		snapshot.NetworkContainers["nc-1"],
		ips,
	)
	require.NoError(t, err)
	require.True(t, changed)
}

func cniImportRecords(t *testing.T) []cns.CNIEndpointState {
	t.Helper()
	return []cns.CNIEndpointState{
		{
			InfraContainerID: "container-1",
			PodEndpointID:    "iface-primary",
			PodName:          "pod-1",
			PodNamespace:     "ns-1",
			InterfaceKey:     "container-1-eth0",
			InterfaceName:    "eth0",
			IPAddresses: []net.IPNet{
				testCNIIPNet(t, "fd00::4/64"),
				testCNIIPNet(t, "10.0.0.4/24"),
			},
		},
		{
			InfraContainerID: "container-1",
			PodEndpointID:    "iface-secondary",
			PodName:          "pod-1",
			PodNamespace:     "ns-1",
			InterfaceKey:     "container-1-eth1",
			InterfaceName:    "eth1",
			IPAddresses:      []net.IPNet{testCNIIPNet(t, "10.1.0.4/24")},
		},
	}
}

func testCNIIPNet(t *testing.T, value string) net.IPNet {
	t.Helper()
	ip, prefix, err := net.ParseCIDR(value)
	require.NoError(t, err)
	prefix.IP = ip
	return *prefix
}
