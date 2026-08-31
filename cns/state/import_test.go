// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	importNC1  = "11111111-1111-1111-1111-111111111111"
	importNC2  = "22222222-2222-2222-2222-222222222222"
	importIPv4 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	importIPv6 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	importNet1 = "cccccccc-cccc-cccc-cccc-cccccccccccc"

	importIPv4Address            = "10.0.0.4"
	importIPv6Address            = "fd00::4"
	importNet1Address            = "10.1.0.4"
	importSubnetCIDR             = "10.0.0.0/24"
	importGatewayIP              = "10.0.0.1"
	importNetworkType            = "azure"
	importOrchestratorType       = "KubernetesCRD"
	importLocation               = "westus2"
	importBootID                 = "boot-1"
	importCNSFileName            = "azure-cns.json"
	importNetworkName            = "network-1"
	importPodName                = "pod-1"
	importPodNamespace           = "ns-1"
	importIfname                 = "eth0"
	importMAC                    = "00:11:22:33:44:55"
	importMAC2                   = "00:11:22:33:44:66"
	importPnPMAC                 = "00-11-22-33-44-55"
	importContainerID            = "container-1"
	importOrchestratorContextKey = "pod"
	importIPv4Key                = "IPv4"
	importIPv6Key                = "IPv6"
)

func TestImportLegacySuccess(t *testing.T) {
	tests := []struct {
		name            string
		manageEndpoints bool
	}{
		{name: "CNS only"},
		{name: "CNS and endpoint state", manageEndpoints: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			cnsData, endpointData := completeLegacyImportData(t)
			opts := writeLegacyImportFiles(t, cnsData, endpointData, tt.manageEndpoints)

			changed, err := db.ImportLegacy(context.Background(), opts)
			require.NoError(t, err)
			assert.True(t, changed)

			snapshot := requireValidSnapshot(t, db)
			assert.Equal(t, SchemaVersion, snapshot.Metadata.SchemaVersion)
			assert.Equal(t, AuthorityBolt, snapshot.Metadata.Authority)
			assert.Equal(t, uint64(1), snapshot.Metadata.Generation)
			assert.Equal(t, importBootID, snapshot.Metadata.BootID)
			assert.True(t, snapshot.Metadata.LegacyImportComplete)
			assert.Equal(t, importOrchestratorType, snapshot.Metadata.OrchestratorType)
			assert.Equal(t, "node-1", snapshot.Metadata.NodeID)
			assert.Equal(t, "eastus", snapshot.Metadata.Location)
			assert.Equal(t, importNetworkType, snapshot.Metadata.NetworkType)
			assert.True(t, snapshot.Metadata.Initialized)
			assert.Equal(t, testNow, snapshot.Metadata.TimeStamp)

			require.Len(t, snapshot.NetworkContainers, 2)
			require.Len(t, snapshot.IPs, 3)
			assert.Equal(t, importIPv4Address, snapshot.IPs[importIPv4].IPAddress)
			assert.Equal(t, importNC1, snapshot.IPs[importIPv4].NCID)
			assert.Equal(t, 7, snapshot.IPs[importIPv4].NCVersion)
			assert.Equal(t, importIPv6Address, snapshot.IPs[importIPv6].IPAddress)
			assert.Equal(t, importNet1Address, snapshot.IPs[importNet1].IPAddress)
			for _, ncID := range []string{importNC1, importNC2} {
				record := snapshot.NetworkContainers[ncID]
				assert.Empty(t, record.Request.AuthorizationToken)
				assert.Nil(t, record.Request.SecondaryIPConfigs)
				assert.NotEmpty(t, record.Request.EndpointPolicies)
				assert.NotEmpty(t, record.Request.NetworkInterfaceInfo.MACAddress)
			}
			assert.Equal(t, []string{importNC1, importNC2}, snapshot.OrchestratorContexts["pod-1ns-1"])
			assert.Equal(t, "PCI\\VEN_1234", snapshot.PnPIDByMAC[importMAC])
			assert.Equal(t, "network-1", snapshot.Networks["network-1"].NetworkName)
			assert.Equal(t, importSubnetCIDR, snapshot.Networks[importNetworkName].NicInfo.Subnet)
			assert.Equal(t, "value", snapshot.Networks["network-1"].Options["custom"])

			if !tt.manageEndpoints {
				assert.Empty(t, snapshot.Endpoints)
				assert.Empty(t, snapshot.Assignments)
				assert.Empty(t, snapshot.IPOwners)
				assert.Empty(t, snapshot.DeleteIntents)
				return
			}
			require.Len(t, snapshot.Endpoints, 1)
			endpoint := snapshot.Endpoints[importContainerID]
			assert.Equal(t, importPodName, endpoint.PodName)
			assert.Equal(t, importPodNamespace, endpoint.PodNamespace)
			assert.Equal(t, "hns-endpoint-1", endpoint.IfnameToIPMap[importIfname].HNSEndpointID)
			assert.Equal(t, "hns-network-1", endpoint.IfnameToIPMap[importIfname].HNSNetworkID)
			assert.Equal(t, "veth-1", endpoint.IfnameToIPMap[importIfname].HostVethName)
			assert.Equal(t, cns.InfraNIC, endpoint.IfnameToIPMap[importIfname].NICType)
			assert.Equal(t, cns.DelegatedVMNIC, endpoint.IfnameToIPMap["net1"].NICType)
			require.Len(t, endpoint.IfnameToIPMap[importIfname].IPv4, 1)
			require.Len(t, endpoint.IfnameToIPMap[importIfname].IPv6, 1)
			assert.Equal(t, []string{importIPv4, importIPv6, importNet1}, snapshot.Assignments[importContainerID].IPIDs)
			assert.Equal(t, importContainerID, snapshot.IPOwners[importIPv4])
			assert.Equal(t, DeleteIntent{CreatedAt: testNow.Add(-time.Minute)}, snapshot.DeleteIntents["deleted-container"])
		})
	}
}

func TestImportLegacySourceFailures(t *testing.T) {
	validCNS, validEndpoint := completeLegacyImportData(t)
	tests := []struct {
		name       string
		cnsData    []byte
		endpoint   []byte
		manage     bool
		mutateOpts func(*ImportOptions)
	}{
		{name: "empty CNS", cnsData: []byte{}},
		{name: "whitespace CNS", cnsData: []byte(" \n\t")},
		{name: "truncated CNS", cnsData: []byte(`{"ContainerNetworkService":`)},
		{name: "malformed CNS", cnsData: []byte(`{"ContainerNetworkService":!}`)},
		{name: "null CNS", cnsData: []byte(`null`)},
		{name: "array CNS", cnsData: []byte(`[]`)},
		{name: "multiple CNS values", cnsData: append(append([]byte{}, validCNS...), []byte(` {}`)...)},
		{name: "missing CNS envelope", cnsData: []byte(`{"Other":{}}`)},
		{name: "duplicate CNS key", cnsData: []byte(`{"ContainerNetworkService":{},"ContainerNetworkService":{}}`)},
		{name: "wrong CNS field type", cnsData: []byte(`{"ContainerNetworkService":{"Location":7}}`)},
		{name: "unknown relevant CNS field", cnsData: []byte(`{"ContainerNetworkService":{"Unexpected":true}}`)},
		{
			name: "null NC list",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				state["ContainerIDByOrchestratorContext"] = map[string]any{importOrchestratorContextKey: nil}
			}),
		},
		{
			name: "empty NC list",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				state["ContainerIDByOrchestratorContext"] = map[string]any{importOrchestratorContextKey: ""}
			}),
		},
		{
			name: "wrong NC list shape",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				state["ContainerIDByOrchestratorContext"] = map[string]any{importOrchestratorContextKey: []string{importNC1}}
			}),
		},
		{
			name: "malformed comma NC list",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				state["ContainerIDByOrchestratorContext"] = map[string]any{importOrchestratorContextKey: importNC1 + ", "}
			}),
		},
		{name: "empty endpoint", cnsData: validCNS, endpoint: []byte{}, manage: true},
		{name: "truncated endpoint", cnsData: validCNS, endpoint: []byte(`{"Endpoints":`), manage: true},
		{name: "malformed endpoint", cnsData: validCNS, endpoint: []byte(`{"Endpoints":!}`), manage: true},
		{name: "null endpoint", cnsData: validCNS, endpoint: []byte(`null`), manage: true},
		{name: "array endpoint", cnsData: validCNS, endpoint: []byte(`[]`), manage: true},
		{name: "multiple endpoint values", cnsData: validCNS, endpoint: append(append([]byte{}, validEndpoint...), []byte(` {}`)...), manage: true},
		{name: "missing endpoint envelope", cnsData: validCNS, endpoint: []byte(`{"Other":{}}`), manage: true},
		{name: "duplicate endpoint key", cnsData: validCNS, endpoint: []byte(`{"Endpoints":{},"endpoints":{}}`), manage: true},
		{name: "wrong endpoint type", cnsData: validCNS, endpoint: []byte(`{"Endpoints":[]}`), manage: true},
		{name: "wrong endpoint field type", cnsData: validCNS, endpoint: []byte(`{"Endpoints":{"container":{"podName":7}}}`), manage: true},
		{
			name:       "missing CNS path option",
			cnsData:    validCNS,
			mutateOpts: func(opts *ImportOptions) { opts.CNSPath = "" },
		},
		{
			name:       "missing boot ID",
			cnsData:    validCNS,
			mutateOpts: func(opts *ImportOptions) { opts.BootID = "" },
		},
		{
			name:       "missing endpoint path option",
			cnsData:    validCNS,
			endpoint:   validEndpoint,
			manage:     true,
			mutateOpts: func(opts *ImportOptions) { opts.EndpointPath = "" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			cnsData := tt.cnsData
			if cnsData == nil {
				cnsData = validCNS
			}
			endpointData := tt.endpoint
			if endpointData == nil {
				endpointData = validEndpoint
			}
			opts := writeLegacyImportFiles(t, cnsData, endpointData, tt.manage)
			if tt.mutateOpts != nil {
				tt.mutateOpts(&opts)
			}
			before := requireValidSnapshot(t, db)

			changed, err := db.ImportLegacy(context.Background(), opts)
			require.Error(t, err)
			assert.False(t, changed)
			assert.Equal(t, before, requireValidSnapshot(t, db))
		})
	}
}

func TestImportLegacyReadFailures(t *testing.T) {
	t.Run("real missing CNS file", func(t *testing.T) {
		db, _ := openTestDB(t)
		before := requireValidSnapshot(t, db)
		changed, err := db.ImportLegacy(context.Background(), ImportOptions{
			CNSPath: filepath.Join(t.TempDir(), "missing.json"),
			BootID:  importBootID,
		})
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("injected permission failure does not expose source", func(t *testing.T) {
		db, _ := openTestDB(t)
		secret := "do-not-expose"
		changed, err := db.importLegacy(
			context.Background(),
			ImportOptions{CNSPath: importCNSFileName, BootID: importBootID},
			func(string) ([]byte, error) { return nil, os.ErrPermission },
		)
		require.ErrorIs(t, err, os.ErrPermission)
		assert.NotContains(t, err.Error(), secret)
		assert.False(t, changed)
		assert.Zero(t, readMetadata(t, db).Generation)
	})

	t.Run("missing endpoint file", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, _ := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, nil, true)
		require.NoError(t, os.Remove(opts.EndpointPath))
		changed, err := db.ImportLegacy(context.Background(), opts)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.False(t, changed)
		assert.Zero(t, readMetadata(t, db).Generation)
	})

	t.Run("endpoint is ignored when ownership is disabled", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, _ := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, []byte(`not JSON`), false)
		opts.EndpointPath = filepath.Join(t.TempDir(), "missing-endpoint.json")
		changed, err := db.ImportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Empty(t, requireValidSnapshot(t, db).Endpoints)
	})
}

func TestImportLegacySemanticFailures(t *testing.T) {
	validCNS, validEndpoint := completeLegacyImportData(t)
	tests := []struct {
		name              string
		cnsData           []byte
		endpoint          []byte
		wantErrorContains string
	}{
		{
			name: "unknown request field",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				request["Unexpected"] = true
			}),
			wantErrorContains: "unknown field",
		},
		{
			name: "invalid IP UUID",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				request["SecondaryIPConfigs"] = map[string]any{
					"not-a-uuid": map[string]any{"IPAddress": importIPv4Address, "NCVersion": float64(7)},
				}
			}),
		},
		{
			name: "mismatched NC ID",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				status := state["ContainerStatus"].(map[string]any)[importNC1].(map[string]any)
				status["ID"] = importNC2
			}),
		},
		{
			name: "duplicate UUID across NCs",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC2].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				request["SecondaryIPConfigs"] = map[string]any{
					importIPv4: map[string]any{"IPAddress": importNet1Address, "NCVersion": float64(8)},
				}
			}),
		},
		{
			name: "duplicate canonical address",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC2].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				config := request["SecondaryIPConfigs"].(map[string]any)[importNet1].(map[string]any)
				config["IPAddress"] = importIPv4Address
			}),
		},
		{
			name: "malformed IP",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				config := request["SecondaryIPConfigs"].(map[string]any)[importIPv4].(map[string]any)
				config["IPAddress"] = "bad-ip"
			}),
		},
		{
			name: "malformed prefix",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				ipConfig := request["IPConfiguration"].(map[string]any)
				ipConfig["IPSubnet"].(map[string]any)["PrefixLength"] = float64(40)
			}),
		},
		{
			name: "malformed request MAC",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				statuses := state["ContainerStatus"].(map[string]any)
				request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
				request["NetworkInterfaceInfo"].(map[string]any)["MACAddress"] = "bad-mac"
			}),
		},
		{
			name: "dangling orchestrator mapping",
			cnsData: mutateLegacyCNS(t, validCNS, func(state map[string]any) {
				state["ContainerIDByOrchestratorContext"] = map[string]any{importOrchestratorContextKey: "missing-nc"}
			}),
		},
		{
			name:    "noncanonical endpoint container identity",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpoints(t, validEndpoint, func(endpoints map[string]any) {
				endpoints[" "+importContainerID] = endpoints[importContainerID]
				delete(endpoints, importContainerID)
			}),
		},
		{
			name:    "endpoint IP absent from inventory",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpoints(t, validEndpoint, func(endpoints map[string]any) {
				info := endpointIPInfo(endpoints, importContainerID, importIfname)
				info["ipv4"] = encodeIPNets(t, "10.9.0.4/24")
			}),
			wantErrorContains: "missing from inventory",
		},
		{
			name:    "duplicate endpoint ownership",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpoints(t, validEndpoint, func(endpoints map[string]any) {
				endpoints["container-2"] = endpoints[importContainerID]
			}),
		},
		{
			name:    "endpoint NC mismatch",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpoints(t, validEndpoint, func(endpoints map[string]any) {
				endpointIPInfo(endpoints, importContainerID, importIfname)["networkContainerID"] = importNC2
			}),
			wantErrorContains: "does not match",
		},
		{
			name:    "malformed endpoint MAC",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpoints(t, validEndpoint, func(endpoints map[string]any) {
				endpointIPInfo(endpoints, importContainerID, importIfname)["macAddress"] = "bad-mac"
			}),
			wantErrorContains: "invalid MAC",
		},
		{
			name:    "invalid delete intent",
			cnsData: validCNS,
			endpoint: mutateLegacyEndpointEnvelope(t, validEndpoint, func(envelope map[string]any) {
				envelope["DeleteIntents"] = map[string]any{
					"deleted-container": map[string]any{"createdAt": "0001-01-01T00:00:00Z"},
				}
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			endpointData := tt.endpoint
			if endpointData == nil {
				endpointData = validEndpoint
			}
			opts := writeLegacyImportFiles(t, tt.cnsData, endpointData, tt.endpoint != nil)
			before := requireValidSnapshot(t, db)
			changed, err := db.ImportLegacy(context.Background(), opts)
			require.Error(t, err)
			if tt.wantErrorContains != "" {
				require.ErrorContains(t, err, tt.wantErrorContains)
				assert.NotContains(t, err.Error(), "duplicate json key")
			}
			assert.False(t, changed)
			assert.Equal(t, before, requireValidSnapshot(t, db))
		})
	}
}

func TestImportLegacyAtomicityReplayAndConcurrency(t *testing.T) {
	t.Run("staged write failure rolls back buckets and marker", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, endpointData := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, endpointData, true)
		before := requireValidSnapshot(t, db)
		changed, err := db.importLegacyWithCommitHook(
			context.Background(),
			opts,
			os.ReadFile,
			func() error { return errAbort },
		)
		require.ErrorIs(t, err, errAbort)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("replay ignores changed corrupt and missing sources", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, endpointData := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, endpointData, true)
		changed, err := db.ImportLegacy(context.Background(), opts)
		require.NoError(t, err)
		require.True(t, changed)
		updated, err := db.UpdateReadinessObservation(context.Background(), importNC1, ReadinessObservation{
			VMVersion:         "7",
			HostVersion:       "current",
			VFPUpdateComplete: true,
		})
		require.NoError(t, err)
		require.True(t, updated)
		current := requireValidSnapshot(t, db)

		require.NoError(t, os.WriteFile(opts.CNSPath, []byte(`not JSON`), 0o600))
		require.NoError(t, os.Remove(opts.EndpointPath))
		changed, err = db.ImportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, current, requireValidSnapshot(t, db))
	})

	t.Run("reopen replay", func(t *testing.T) {
		db, path := openTestDB(t)
		cnsData, endpointData := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, endpointData, true)
		changed, err := db.ImportLegacy(context.Background(), opts)
		require.NoError(t, err)
		require.True(t, changed)
		imported := requireValidSnapshot(t, db)
		require.NoError(t, db.Close())

		reopened, err := Open(path, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, reopened.Close()) })
		require.NoError(t, os.Remove(opts.CNSPath))
		require.NoError(t, os.Remove(opts.EndpointPath))
		changed, err = reopened.ImportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, imported, requireValidSnapshot(t, reopened))
	})

	t.Run("concurrent calls commit once", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, endpointData := completeLegacyImportData(t)
		opts := writeLegacyImportFiles(t, cnsData, endpointData, true)
		const callers = 16
		start := make(chan struct{})
		results := make(chan bool, callers)
		errs := make(chan error, callers)
		var wg sync.WaitGroup
		wg.Add(callers)
		for range callers {
			go func() {
				defer wg.Done()
				<-start
				changed, err := db.ImportLegacy(context.Background(), opts)
				results <- changed
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)

		var commits int
		for changed := range results {
			if changed {
				commits++
			}
		}
		for err := range errs {
			require.NoError(t, err)
		}
		assert.Equal(t, 1, commits)
		assert.Equal(t, uint64(1), readMetadata(t, db).Generation)
	})
}

func TestImportLegacyRejectsNonemptyUnmarkedDatabase(t *testing.T) {
	db, _ := openTestDB(t)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{NodeID: "current-node"})
	}))
	before := requireValidSnapshot(t, db)
	cnsData, endpointData := completeLegacyImportData(t)
	opts := writeLegacyImportFiles(t, cnsData, endpointData, true)

	changed, err := db.ImportLegacy(context.Background(), opts)
	require.ErrorIs(t, err, ErrLegacyImportTargetNotEmpty)
	assert.False(t, changed)
	assert.Equal(t, before, requireValidSnapshot(t, db))
}

func TestImportLegacyContextCancellation(t *testing.T) {
	t.Run("before read", func(t *testing.T) {
		db, _ := openTestDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var reads atomic.Int32
		changed, err := db.importLegacy(
			ctx,
			ImportOptions{CNSPath: importCNSFileName, BootID: importBootID},
			func(string) ([]byte, error) {
				reads.Add(1)
				return nil, nil
			},
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, changed)
		assert.Zero(t, reads.Load())
		assert.Zero(t, readMetadata(t, db).Generation)
	})

	t.Run("waiting for writer gate", func(t *testing.T) {
		db, _ := openTestDB(t)
		cnsData, _ := completeLegacyImportData(t)
		db.writeGate <- struct{}{}
		t.Cleanup(func() {
			select {
			case <-db.writeGate:
			default:
			}
		})
		readDone := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := db.importLegacy(
				ctx,
				ImportOptions{CNSPath: importCNSFileName, BootID: importBootID},
				func(string) ([]byte, error) {
					close(readDone)
					return cnsData, nil
				},
			)
			result <- err
		}()
		<-readDone
		cancel()
		require.ErrorIs(t, <-result, context.Canceled)
		<-db.writeGate
		assert.Zero(t, readMetadata(t, db).Generation)
		assert.False(t, readMetadata(t, db).LegacyImportComplete)
	})
}

func TestImportLegacyDeterministicErrors(t *testing.T) {
	cnsData, _ := completeLegacyImportData(t)
	cnsData = mutateLegacyCNS(t, cnsData, func(state map[string]any) {
		statuses := state["ContainerStatus"].(map[string]any)
		request := statuses[importNC1].(map[string]any)["CreateNetworkContainerRequest"].(map[string]any)
		request["HostPrimaryIP"] = "bad-host"
		request["PrimaryInterfaceIdentifier"] = "bad-interface"
	})
	var want string
	for range 20 {
		_, err := parseLegacyCNS(cnsData, importBootID)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret-token")
		if want == "" {
			want = err.Error()
			continue
		}
		assert.Equal(t, want, err.Error())
	}
}

func completeLegacyImportData(t testing.TB) (cnsData, endpointData []byte) {
	t.Helper()
	request1 := completeNetworkContainerRequest()
	request1.NetworkContainerid = importNC1
	request1.Version = "7"
	request1.AuthorizationToken = "secret-token"
	request1.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{
		importIPv4: {IPAddress: importIPv4Address, NCVersion: 7},
		importIPv6: {IPAddress: importIPv6Address, NCVersion: 7},
	}
	request2 := completeNetworkContainerRequest()
	request2.NetworkContainerid = importNC2
	request2.Version = "8"
	request2.AuthorizationToken = "another-secret"
	request2.IPConfiguration.IPSubnet.IPAddress = "10.1.0.0"
	request2.IPConfiguration.GatewayIPAddress = "10.1.0.1"
	request2.NetworkInterfaceInfo.MACAddress = importMAC2
	request2.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{
		importNet1: {IPAddress: importNet1Address, NCVersion: 8},
	}
	ncList := importNC1 + "," + importNC2
	cnsState := legacyCNSState{
		Location:         "eastus",
		NetworkType:      importNetworkType,
		OrchestratorType: importOrchestratorType,
		NodeID:           "node-1",
		Initialized:      true,
		ContainerIDByOrchestratorContext: map[string]*string{
			"pod-1ns-1": &ncList,
		},
		ContainerStatus: map[string]legacyContainerStatus{
			importNC1: {
				ID:                            importNC1,
				VMVersion:                     "7",
				HostVersion:                   "6",
				CreateNetworkContainerRequest: request1,
				VfpUpdateComplete:             true,
			},
			importNC2: {
				ID:                            importNC2,
				VMVersion:                     "8",
				HostVersion:                   "7",
				CreateNetworkContainerRequest: request2,
				VfpUpdateComplete:             false,
			},
		},
		Networks: map[string]*legacyNetworkInfo{
			importNetworkName: {
				NetworkName: importNetworkName,
				NicInfo: &wireserver.InterfaceInfo{
					Subnet:       importSubnetCIDR,
					Gateway:      importGatewayIP,
					PrimaryIP:    "10.0.0.2",
					SecondaryIPs: []string{"10.0.0.3"},
					IsPrimary:    true,
				},
				Options: map[string]any{"custom": "value", "enabled": true},
			},
		},
		TimeStamp: testNow,
		PnpIDByMacAddress: map[string]string{
			importPnPMAC: "PCI\\VEN_1234",
		},
	}
	cnsData, err := json.Marshal(map[string]any{
		"ContainerNetworkService": cnsState,
		"Unrelated":               map[string]any{"compatible": true},
	})
	require.NoError(t, err)

	endpoint := EndpointRecord{
		PodName:      importPodName,
		PodNamespace: importPodNamespace,
		IfnameToIPMap: map[string]*IPInfoRecord{
			importIfname: {
				IPv4:               []net.IPNet{mustIPNet(t, importIPv4Address+"/24")},
				IPv6:               []net.IPNet{mustIPNet(t, importIPv6Address+"/64")},
				HNSEndpointID:      "hns-endpoint-1",
				HNSNetworkID:       "hns-network-1",
				HostVethName:       "veth-1",
				MACAddress:         importMAC,
				NetworkContainerID: importNC1,
				NICType:            cns.InfraNIC,
			},
			cniImportTestSecondaryInterface: {
				IPv4:               []net.IPNet{mustIPNet(t, importNet1Address+"/24")},
				HNSEndpointID:      "hns-endpoint-2",
				HNSNetworkID:       "hns-network-2",
				HostVethName:       "veth-2",
				MACAddress:         importMAC2,
				NetworkContainerID: importNC2,
				NICType:            cns.DelegatedVMNIC,
			},
		},
	}
	endpointData, err = json.Marshal(map[string]any{
		testEndpointsKey: map[string]*EndpointRecord{importContainerID: &endpoint},
		"DeleteIntents": map[string]DeleteIntent{
			"deleted-container": {CreatedAt: testNow.Add(-time.Minute)},
		},
		"Unrelated": map[string]any{"compatible": true},
	})
	require.NoError(t, err)
	return cnsData, endpointData
}

func writeLegacyImportFiles(t *testing.T, cnsData, endpointData []byte, manage bool) ImportOptions {
	t.Helper()
	dir := t.TempDir()
	cnsPath := filepath.Join(dir, importCNSFileName)
	require.NoError(t, os.WriteFile(cnsPath, cnsData, 0o600))
	opts := ImportOptions{CNSPath: cnsPath, BootID: importBootID, ManageEndpointState: manage}
	if manage {
		opts.EndpointPath = filepath.Join(dir, "azure-endpoints.json")
		require.NoError(t, os.WriteFile(opts.EndpointPath, endpointData, 0o600))
	}
	return opts
}

func mutateLegacyCNS(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(data, &envelope))
	mutate(envelope["ContainerNetworkService"].(map[string]any))
	result, err := json.Marshal(envelope)
	require.NoError(t, err)
	return result
}

func mutateLegacyEndpoints(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	return mutateLegacyEndpointEnvelope(t, data, func(envelope map[string]any) {
		mutate(envelope["Endpoints"].(map[string]any))
	})
}

func mutateLegacyEndpointEnvelope(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(data, &envelope))
	mutate(envelope)
	result, err := json.Marshal(envelope)
	require.NoError(t, err)
	return result
}

func endpointIPInfo(endpoints map[string]any, containerID, ifname string) map[string]any {
	endpoint := endpoints[containerID].(map[string]any)
	interfaces := endpoint["ifnameToIPMap"].(map[string]any)
	return interfaces[ifname].(map[string]any)
}

func encodeIPNets(t *testing.T, prefixes ...string) any {
	t.Helper()
	values := make([]net.IPNet, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, mustIPNet(t, prefix))
	}
	data, err := json.Marshal(values)
	require.NoError(t, err)
	var result any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func mustIPNet(t testing.TB, value string) net.IPNet {
	t.Helper()
	ip, network, err := net.ParseCIDR(value)
	require.NoError(t, err)
	network.IP = ip
	return *network
}
