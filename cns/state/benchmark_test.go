// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var nodeInventorySizes = []int{250, 500, 1000}

func BenchmarkSnapshot(b *testing.B) {
	for _, size := range nodeInventorySizes {
		b.Run(fmt.Sprintf("inventory=%d", size), func(b *testing.B) {
			db := openBenchmarkDB(b)
			durable := benchmarkDurableState(size)
			changed, err := db.ReplaceDurableState(context.Background(), 0, durable)
			require.NoError(b, err)
			require.True(b, changed)

			b.ReportAllocs()
			for b.Loop() {
				snapshot, err := db.Snapshot(context.Background())
				if err != nil {
					b.Fatal(err)
				}
				if len(snapshot.IPs) != size {
					b.Fatalf("snapshot has %d IPs, want %d", len(snapshot.IPs), size)
				}
			}
		})
	}
}

func BenchmarkApplyNetworkContainerReplacement(b *testing.B) {
	for _, size := range nodeInventorySizes {
		b.Run(fmt.Sprintf("inventory=%d", size), func(b *testing.B) {
			db := openBenchmarkDB(b)
			durable := benchmarkDurableState(size)
			changed, err := db.ReplaceDurableState(context.Background(), 0, durable)
			require.NoError(b, err)
			require.True(b, changed)
			replacement := benchmarkNCIPs(durable, "benchmark-nc-0")

			b.ReportAllocs()
			for b.Loop() {
				changed, err := db.ApplyNetworkContainer(
					context.Background(),
					durable.NetworkContainers["benchmark-nc-0"],
					replacement,
				)
				if err != nil {
					b.Fatal(err)
				}
				if !changed {
					b.Fatal("replacement unexpectedly reported no change")
				}
			}
		})
	}
}

func BenchmarkAssignReleaseTransaction(b *testing.B) {
	for _, size := range nodeInventorySizes {
		b.Run(fmt.Sprintf("inventory=%d", size), func(b *testing.B) {
			db := openBenchmarkDB(b)
			durable := benchmarkDurableState(size)
			changed, err := db.ReplaceDurableState(context.Background(), 0, durable)
			require.NoError(b, err)
			require.True(b, changed)

			ip := durable.IPs[benchmarkIPID(0)]
			assignment := testAssignment("benchmark-container", "benchmark-pod", "benchmark-ns", ip.ID)
			endpoint := EndpointRecord{
				PodName:      assignment.Pod.PodName,
				PodNamespace: assignment.Pod.PodNamespace,
				IfnameToIPMap: map[string]*IPInfoRecord{
					exportIfnameEth0: {
						IPv4:               []net.IPNet{mustIPNetValue(ip.IPAddress, 24)},
						NetworkContainerID: ip.NCID,
					},
				},
			}
			now := time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)

			b.ReportAllocs()
			for b.Loop() {
				changed, err := db.AssignEndpoint(
					context.Background(),
					assignment,
					endpoint,
					now,
					testDeleteIntentTTL,
				)
				if err != nil || !changed {
					b.Fatalf("assigning endpoint: changed=%t err=%v", changed, err)
				}
				changed, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, now)
				if err != nil || !changed {
					b.Fatalf("releasing endpoint: changed=%t err=%v", changed, err)
				}
				changed, err = db.DeleteEndpointRecord(context.Background(), assignment.Pod.InfraContainerID)
				if err != nil || !changed {
					b.Fatalf("deleting endpoint: changed=%t err=%v", changed, err)
				}
				pruned, err := db.PruneDeleteIntents(
					context.Background(),
					now.Add(testDeleteIntentTTL),
					testDeleteIntentTTL,
				)
				if err != nil || pruned != 1 {
					b.Fatalf("pruning delete intent: count=%d err=%v", pruned, err)
				}
			}
		})
	}
}

func openBenchmarkDB(b *testing.B) *DB {
	b.Helper()
	db, err := Open(filepath.Join(b.TempDir(), "state.db"), Options{NoSync: true})
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, db.Close()) })
	return db
}

func benchmarkDurableState(size int) DurableState {
	const ipsPerNC = 50
	durable := NewDurableState()
	for index := range size {
		ncIndex := index / ipsPerNC
		ncID := fmt.Sprintf("benchmark-nc-%d", ncIndex)
		if _, ok := durable.NetworkContainers[ncID]; !ok {
			durable.NetworkContainers[ncID] = testNetworkContainer(ncID)
			durable.OrchestratorContexts[fmt.Sprintf("benchmark-context-%d", ncIndex)] = []string{ncID}
		}
		durable.IPs[benchmarkIPID(index)] = IPRecord{
			ID:        benchmarkIPID(index),
			IPAddress: fmt.Sprintf("10.%d.%d.%d", 100+index/62500, (index/250)%250, index%250+1),
			NCID:      ncID,
			NCVersion: 1,
		}
	}
	return durable
}

func benchmarkNCIPs(durable DurableState, ncID string) []IPRecord {
	records := make([]IPRecord, 0)
	for _, id := range sortedKeys(durable.IPs) {
		if durable.IPs[id].NCID == ncID {
			records = append(records, durable.IPs[id])
		}
	}
	return records
}

func benchmarkIPID(index int) string {
	return fmt.Sprintf("20000000-0000-4000-8000-%012d", index+1)
}
