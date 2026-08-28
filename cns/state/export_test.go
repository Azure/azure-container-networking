// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

var errRollbackFault = errors.New("rollback fault")

const (
	exportNC1  = "11111111-1111-1111-1111-111111111111"
	exportNC2  = "22222222-2222-2222-2222-222222222222"
	exportIPv4 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	exportIPv6 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	exportNet1 = "cccccccc-cccc-cccc-cccc-cccccccccccc"

	exportIPv4Address   = "10.0.0.4"
	exportIPv6Address   = "fd00::4"
	exportIfnameEth0    = "eth0"
	exportNodeID        = "node-1"
	exportNetworkName   = "network-1"
	exportGatewayIPv4   = "10.0.0.1"
	exportSecondaryIPv4 = "10.0.0.5"
	exportContainerID   = "container-1"
	exportPodName       = "pod-1"
	exportPodNamespace  = "ns-1"
)

type rollbackFailureStage string

const (
	rollbackFailureMkdir      rollbackFailureStage = "mkdir"
	rollbackFailureCreate     rollbackFailureStage = "create"
	rollbackFailureWrite      rollbackFailureStage = "write"
	rollbackFailureSync       rollbackFailureStage = "sync"
	rollbackFailureClose      rollbackFailureStage = "close"
	rollbackFailureReplace    rollbackFailureStage = "replace"
	rollbackFailureParentSync rollbackFailureStage = "parent sync"
)

type faultRollbackFile struct {
	rollbackTemporaryFile
	stage rollbackFailureStage
}

func (f *faultRollbackFile) Write(data []byte) (int, error) {
	if f.stage == rollbackFailureWrite {
		return 0, errRollbackFault
	}
	return f.rollbackTemporaryFile.Write(data) //nolint:wrapcheck // test double forwards the underlying temp file's error unchanged for fault-injection identity checks
}

func (f *faultRollbackFile) Sync() error {
	if f.stage == rollbackFailureSync {
		return errRollbackFault
	}
	return f.rollbackTemporaryFile.Sync() //nolint:wrapcheck // test double forwards the underlying temp file's error unchanged for fault-injection identity checks
}

func (f *faultRollbackFile) Close() error {
	err := f.rollbackTemporaryFile.Close()
	if f.stage == rollbackFailureClose {
		return errors.Join(err, errRollbackFault)
	}
	return err //nolint:wrapcheck // test double forwards the underlying temp file's error unchanged for fault-injection identity checks
}

func TestExportLegacySuccess(t *testing.T) {
	ctx := context.Background()
	db, _ := openPopulatedExportDB(t)
	before := requireValidSnapshot(t, db)
	opts := exportPaths(t)

	changed, err := db.ExportLegacy(ctx, opts)
	require.NoError(t, err)
	require.True(t, changed)

	cnsData := readRollbackFile(t, opts.CNSJSONPath)
	endpointData := readRollbackFile(t, opts.EndpointJSONPath)
	assert.NotContains(t, string(cnsData), "secret-token")
	assert.NotContains(t, string(cnsData), "another-secret")
	for _, key := range []string{
		"Location",
		"ContainerIDByOrchestratorContext",
		"ContainerStatus",
		"CreateNetworkContainerRequest",
		"SecondaryIPConfigs",
		"Networks",
		"PnpIDByMacAddress",
	} {
		assert.Contains(t, string(cnsData), `"`+key+`"`)
	}
	for _, key := range []string{
		"Endpoints",
		"DeleteIntents",
		"PodName",
		"IfnameToIPMap",
		"HnsEndpointID",
		"NetworkContainerID",
		"NICType",
	} {
		assert.Contains(t, string(endpointData), `"`+key+`"`)
	}
	assert.NotContains(t, string(endpointData), `"podName"`)

	var cnsEnvelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(cnsData, &cnsEnvelope))
	var exported rollbackCNSState
	//nolint:musttag // embeds legacy cns.CreateNetworkContainerRequest, untagged by design for field-name JSON compatibility
	require.NoError(t, json.Unmarshal(cnsEnvelope["ContainerNetworkService"], &exported))
	assert.Equal(t, before.Metadata.Location, exported.Location)
	assert.Equal(t, before.Metadata.NetworkType, exported.NetworkType)
	assert.Equal(t, before.Metadata.OrchestratorType, exported.OrchestratorType)
	assert.Equal(t, before.Metadata.NodeID, exported.NodeID)
	assert.Equal(t, before.Metadata.Initialized, exported.Initialized)
	assert.Equal(t, before.Metadata.TimeStamp, exported.TimeStamp)
	require.NotNil(t, exported.ContainerIDByOrchestratorContext["pod-1ns-1"])
	assert.Equal(
		t,
		exportNC1+","+exportNC2,
		*exported.ContainerIDByOrchestratorContext["pod-1ns-1"],
	)
	assert.ElementsMatch(t, []string{exportNC1, exportNC2}, sortedKeys(exported.ContainerStatus))
	request := exported.ContainerStatus[exportNC1].CreateNetworkContainerRequest
	assert.Empty(t, request.AuthorizationToken)
	assert.Equal(t, map[string]cns.SecondaryIPConfig{
		exportIPv4: {IPAddress: exportIPv4Address, NCVersion: 7},
		exportIPv6: {IPAddress: exportIPv6Address, NCVersion: 7},
	}, request.SecondaryIPConfigs)
	require.Len(t, request.EndpointPolicies, 1)
	assert.Equal(t, before.NetworkContainers[exportNC1].Request.EndpointPolicies[0].Type, request.EndpointPolicies[0].Type)
	assert.Equal(t, before.NetworkContainers[exportNC1].Request.EndpointPolicies[0].EndpointType, request.EndpointPolicies[0].EndpointType)
	assert.JSONEq(
		t,
		string(before.NetworkContainers[exportNC1].Request.EndpointPolicies[0].Settings),
		string(request.EndpointPolicies[0].Settings),
	)
	assert.Equal(t, before.NetworkContainers[exportNC1].Request.NetworkInterfaceInfo, request.NetworkInterfaceInfo)
	require.NotNil(t, exported.Networks[exportNetworkName].NicInfo)
	assert.Equal(t, before.Networks[exportNetworkName].NicInfo, exported.Networks[exportNetworkName].NicInfo)
	assert.Equal(t, "value", exported.Networks[exportNetworkName].Options["custom"])
	assert.Equal(t, "PCI\\VEN_1234", exported.PnpIDByMacAddress["00:11:22:33:44:55"])

	var endpointEnvelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(endpointData, &endpointEnvelope))
	var endpoints map[string]*rollbackEndpointInfo
	require.NoError(t, json.Unmarshal(endpointEnvelope["Endpoints"], &endpoints))
	assert.Equal(t, exportPodName, endpoints[exportContainerID].PodName)
	assert.Equal(t, exportPodNamespace, endpoints[exportContainerID].PodNamespace)
	assert.ElementsMatch(t, []string{exportIfnameEth0, "net1"}, sortedKeys(endpoints[exportContainerID].IfnameToIPMap))
	info := endpoints[exportContainerID].IfnameToIPMap[exportIfnameEth0]
	require.Len(t, info.IPv4, 1)
	assert.Equal(t, net.ParseIP(exportIPv4Address), info.IPv4[0].IP)
	require.Len(t, info.IPv6, 1)
	assert.Equal(t, net.ParseIP(exportIPv6Address), info.IPv6[0].IP)
	assert.Equal(t, "hns-endpoint-1", info.HnsEndpointID)
	assert.Equal(t, "hns-network-1", info.HnsNetworkID)
	assert.Equal(t, "veth-1", info.HostVethName)
	assert.Equal(t, "00:11:22:33:44:55", info.MacAddress)
	assert.Equal(t, exportNC1, info.NetworkContainerID)
	assert.Equal(t, cns.InfraNIC, info.NICType)
	delegated := endpoints[exportContainerID].IfnameToIPMap["net1"]
	require.Len(t, delegated.IPv4, 1)
	assert.Equal(t, net.ParseIP("10.1.0.4"), delegated.IPv4[0].IP)
	assert.Equal(t, exportNC2, delegated.NetworkContainerID)
	assert.Equal(t, cns.DelegatedVMNIC, delegated.NICType)
	var intents map[string]DeleteIntent
	require.NoError(t, json.Unmarshal(endpointEnvelope["DeleteIntents"], &intents))
	assert.Equal(t, before.DeleteIntents, intents)

	for _, path := range []string{opts.CNSJSONPath, opts.EndpointJSONPath} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
		parentInfo, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o755), parentInfo.Mode().Perm())
	}
	assertNoRollbackTemps(t, opts)
	after := requireValidSnapshot(t, db)
	assert.Equal(t, AuthorityJSON, after.Metadata.Authority)
	assert.True(t, after.Metadata.RollbackExportComplete)
	assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
}

func TestExportLegacyValidationAndDatabaseFailures(t *testing.T) {
	tests := []struct {
		name string
		opts ExportOptions
	}{
		{name: "missing CNS path", opts: ExportOptions{EndpointJSONPath: "endpoint.json"}},
		{name: "empty CNS path", opts: ExportOptions{CNSJSONPath: " \t", EndpointJSONPath: "endpoint.json"}},
		{name: "missing endpoint path", opts: ExportOptions{CNSJSONPath: "cns.json"}},
		{name: "same path", opts: ExportOptions{CNSJSONPath: "state.json", EndpointJSONPath: "./state.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			changed, err := db.ExportLegacy(context.Background(), tt.opts)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.False(t, changed)
			assert.Zero(t, readMetadata(t, db).Generation)
		})
	}

	t.Run("canceled", func(t *testing.T) {
		db, _ := openTestDB(t)
		opts := exportPaths(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		changed, err := db.ExportLegacy(ctx, opts)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, changed)
		assertRollbackDestinationsMissing(t, opts)
	})

	t.Run("closed", func(t *testing.T) {
		db, _ := openTestDB(t)
		require.NoError(t, db.Close())
		opts := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), opts)
		require.ErrorIs(t, err, bolterrors.ErrDatabaseNotOpen)
		assert.False(t, changed)
		assertRollbackDestinationsMissing(t, opts)
	})

	t.Run("read only", func(t *testing.T) {
		db, path := openPopulatedExportDB(t)
		require.NoError(t, db.Close())
		readOnly, err := Open(path, Options{ReadOnly: true})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
		opts := exportPaths(t)
		changed, err := readOnly.ExportLegacy(context.Background(), opts)
		require.ErrorIs(t, err, bolterrors.ErrDatabaseReadOnly)
		assert.False(t, changed)
		assertRollbackDestinationsMissing(t, opts)
	})

	t.Run("invalid snapshot", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(bucketIPs).Put([]byte(exportIPv4), []byte(`{"id":"broken"}`))
		}))
		opts := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), opts)
		require.Error(t, err)
		assert.False(t, changed)
		assertRollbackDestinationsMissing(t, opts)
	})
}

func TestExportLegacyFileFailuresAndReplay(t *testing.T) {
	stages := []rollbackFailureStage{
		rollbackFailureMkdir,
		rollbackFailureCreate,
		rollbackFailureWrite,
		rollbackFailureSync,
		rollbackFailureClose,
		rollbackFailureReplace,
		rollbackFailureParentSync,
	}
	outputs := []struct {
		name  string
		first bool
	}{
		{name: "first CNS file", first: true},
		{name: "second endpoint file"},
	}
	for _, output := range outputs {
		for _, stage := range stages {
			t.Run(output.name+"/"+string(stage), func(t *testing.T) {
				db, _ := openPopulatedExportDB(t)
				before := requireValidSnapshot(t, db)
				opts := exportPaths(t)
				oldCNS := []byte(`{"old":"cns"}`)
				oldEndpoint := []byte(`{"old":"endpoint"}`)
				writeRollbackDestination(t, opts.CNSJSONPath, oldCNS)
				writeRollbackDestination(t, opts.EndpointJSONPath, oldEndpoint)
				target := opts.EndpointJSONPath
				if output.first {
					target = opts.CNSJSONPath
				}

				changed, err := db.exportLegacy(
					context.Background(),
					opts,
					faultRollbackOperations(target, stage),
				)
				require.ErrorIs(t, err, errRollbackFault)
				assert.False(t, changed)
				failed := requireValidSnapshot(t, db)
				assert.Equal(t, AuthorityBolt, failed.Metadata.Authority)
				assert.False(t, failed.Metadata.RollbackExportComplete)
				assert.Equal(t, before.Metadata.Generation, failed.Metadata.Generation)
				assertNoRollbackTemps(t, opts)

				if output.first && stage != rollbackFailureParentSync {
					assert.Equal(t, oldCNS, readRollbackFile(t, opts.CNSJSONPath))
					assert.Equal(t, oldEndpoint, readRollbackFile(t, opts.EndpointJSONPath))
				}
				if output.first && stage == rollbackFailureParentSync {
					assertValidRollbackJSON(t, opts.CNSJSONPath)
					assert.Equal(t, oldEndpoint, readRollbackFile(t, opts.EndpointJSONPath))
				}
				if !output.first {
					assertValidRollbackJSON(t, opts.CNSJSONPath)
					if stage != rollbackFailureParentSync {
						assert.Equal(t, oldEndpoint, readRollbackFile(t, opts.EndpointJSONPath))
					}
				}

				changed, err = db.ExportLegacy(context.Background(), opts)
				require.NoError(t, err)
				assert.True(t, changed)
				assertValidRollbackJSON(t, opts.CNSJSONPath)
				assertValidRollbackJSON(t, opts.EndpointJSONPath)
				assertNoRollbackTemps(t, opts)
				after := requireValidSnapshot(t, db)
				assert.Equal(t, AuthorityJSON, after.Metadata.Authority)
				assert.True(t, after.Metadata.RollbackExportComplete)
				assert.Equal(t, before.Metadata.Generation+1, after.Metadata.Generation)
			})
		}
	}
}

func TestCompletedExportPreservesJSONMutation(t *testing.T) {
	db, _ := openPopulatedExportDB(t)
	opts := exportPaths(t)
	changed, err := db.ExportLegacy(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, changed)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(readRollbackFile(t, opts.CNSJSONPath), &envelope))
	cnsState := envelope["ContainerNetworkService"].(map[string]any)
	cnsState["NodeID"] = "newer-json-node"
	mutated, err := json.MarshalIndent(envelope, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(opts.CNSJSONPath, mutated, 0o600))

	changed, err = db.ExportLegacy(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, mutated, readRollbackFile(t, opts.CNSJSONPath))
}

func TestExportLegacyMarkerFailureReplayAndNoop(t *testing.T) {
	t.Run("marker transaction failure retries both files", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		before := requireValidSnapshot(t, db)
		opts := exportPaths(t)
		changed, err := db.exportLegacyWithCommitHook(
			context.Background(),
			opts,
			osRollbackFileOperations(),
			func() error { return errAbort },
		)
		require.ErrorIs(t, err, errAbort)
		assert.False(t, changed)
		assertValidRollbackJSON(t, opts.CNSJSONPath)
		assertValidRollbackJSON(t, opts.EndpointJSONPath)
		failed := requireValidSnapshot(t, db)
		assert.Equal(t, AuthorityBolt, failed.Metadata.Authority)
		assert.False(t, failed.Metadata.RollbackExportComplete)
		assert.Equal(t, before.Metadata.Generation, failed.Metadata.Generation)

		firstCNS := readRollbackFile(t, opts.CNSJSONPath)
		firstEndpoint := readRollbackFile(t, opts.EndpointJSONPath)
		require.NoError(t, os.WriteFile(opts.CNSJSONPath, []byte(`{"torn":"cns"}`), 0o600))
		require.NoError(t, os.WriteFile(opts.EndpointJSONPath, []byte(`{"torn":"endpoint"}`), 0o600))
		changed, err = db.ExportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, firstCNS, readRollbackFile(t, opts.CNSJSONPath))
		assert.Equal(t, firstEndpoint, readRollbackFile(t, opts.EndpointJSONPath))
		assertNoRollbackTemps(t, opts)
	})

	t.Run("completed export preserves external mutation after reopen", func(t *testing.T) {
		db, path := openPopulatedExportDB(t)
		opts := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), opts)
		require.NoError(t, err)
		require.True(t, changed)
		generation := readMetadata(t, db).Generation
		cnsMutation := []byte(`{"newer":"cns"}`)
		endpointMutation := []byte(`{"newer":"endpoint"}`)
		require.NoError(t, os.WriteFile(opts.CNSJSONPath, cnsMutation, 0o600))
		require.NoError(t, os.WriteFile(opts.EndpointJSONPath, endpointMutation, 0o600))

		changed, err = db.ExportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, cnsMutation, readRollbackFile(t, opts.CNSJSONPath))
		assert.Equal(t, endpointMutation, readRollbackFile(t, opts.EndpointJSONPath))
		assert.Equal(t, generation, readMetadata(t, db).Generation)
		require.NoError(t, db.Close())

		reopened, err := Open(path, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, reopened.Close()) })
		changed, err = reopened.ExportLegacy(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, cnsMutation, readRollbackFile(t, opts.CNSJSONPath))
		assert.Equal(t, endpointMutation, readRollbackFile(t, opts.EndpointJSONPath))
		assert.Equal(t, generation, readMetadata(t, reopened).Generation)
	})
}

func TestExportLegacyConcurrencyAndWriterGate(t *testing.T) {
	t.Run("concurrent exports transition once", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		before := requireValidSnapshot(t, db)
		opts := exportPaths(t)
		const callers = 12
		start := make(chan struct{})
		results := make(chan bool, callers)
		errs := make(chan error, callers)
		var wg sync.WaitGroup
		wg.Add(callers)
		for range callers {
			go func() {
				defer wg.Done()
				<-start
				changed, err := db.ExportLegacy(context.Background(), opts)
				results <- changed
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)

		var transitions int
		for result := range results {
			if result {
				transitions++
			}
		}
		for err := range errs {
			require.NoError(t, err)
		}
		assert.Equal(t, 1, transitions)
		assert.Equal(t, before.Metadata.Generation+1, readMetadata(t, db).Generation)
		assertValidRollbackJSON(t, opts.CNSJSONPath)
		assertValidRollbackJSON(t, opts.EndpointJSONPath)
	})

	t.Run("DB update waits for files and sees transition", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		before := requireValidSnapshot(t, db)
		opts := exportPaths(t)
		enteredWrite := make(chan struct{})
		releaseWrite := make(chan struct{})
		files := osRollbackFileOperations()
		createTemp := files.createTemp
		var blockedOnce atomic.Bool
		files.createTemp = func(dir, pattern string) (rollbackTemporaryFile, error) {
			file, err := createTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			if blockedOnce.CompareAndSwap(false, true) {
				return &blockingRollbackFile{
					rollbackTemporaryFile: file,
					entered:               enteredWrite,
					release:               releaseWrite,
				}, nil
			}
			return file, nil
		}

		exportDone := make(chan error, 1)
		go func() {
			_, err := db.exportLegacy(context.Background(), opts, files)
			exportDone <- err
		}()
		<-enteredWrite
		require.Len(t, db.writeGate, 1)

		callbackEntered := make(chan Metadata, 1)
		updateDone := make(chan error, 1)
		go func() {
			updateDone <- db.Update(context.Background(), func(tx *WriteTx) error {
				meta, err := tx.Metadata()
				if err == nil {
					callbackEntered <- meta
				}
				return errors.Join(err, errAbort)
			})
		}()
		select {
		case <-callbackEntered:
			t.Fatal("DB update entered while rollback files were incomplete")
		default:
		}

		close(releaseWrite)
		require.NoError(t, <-exportDone)
		require.ErrorIs(t, <-updateDone, errAbort)
		seen := <-callbackEntered
		assert.Equal(t, AuthorityJSON, seen.Authority)
		assert.True(t, seen.RollbackExportComplete)
		assert.Equal(t, before.Metadata.Generation+1, seen.Generation)
	})
}

func TestExportLegacyRejectsInconsistentAuthorityMarker(t *testing.T) {
	tests := []struct {
		name      string
		authority Authority
		marker    bool
	}{
		{name: "Bolt with marker", authority: AuthorityBolt, marker: true},
		{name: "JSON without marker", authority: AuthorityJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openPopulatedExportDB(t)
			require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
				metadata := tx.Bucket(bucketMetadata)
				if err := metadata.Put(metaKeyAuthority, []byte(tt.authority)); err != nil {
					return fmt.Errorf("setting authority marker: %w", err)
				}
				if tt.marker {
					return metadata.Put(metaKeyRollbackExport, []byte(rollbackExportMarker))
				}
				return metadata.Delete(metaKeyRollbackExport)
			}))
			opts := exportPaths(t)
			writeRollbackDestination(t, opts.CNSJSONPath, []byte(`{"newer":"cns"}`))
			writeRollbackDestination(t, opts.EndpointJSONPath, []byte(`{"newer":"endpoint"}`))
			changed, err := db.ExportLegacy(context.Background(), opts)
			require.ErrorIs(t, err, ErrCorrupt)
			assert.False(t, changed)
			assert.JSONEq(t, `{"newer":"cns"}`, string(readRollbackFile(t, opts.CNSJSONPath)))
			assert.JSONEq(t, `{"newer":"endpoint"}`, string(readRollbackFile(t, opts.EndpointJSONPath)))
		})
	}
}

type blockingRollbackFile struct {
	rollbackTemporaryFile
	entered chan<- struct{}
	release <-chan struct{}
}

func (f *blockingRollbackFile) Write(data []byte) (int, error) {
	close(f.entered)
	<-f.release
	return f.rollbackTemporaryFile.Write(data) //nolint:wrapcheck // test double forwards the underlying temp file's error unchanged for fault-injection identity checks
}

func faultRollbackOperations(target string, stage rollbackFailureStage) rollbackFileOperations {
	files := osRollbackFileOperations()
	mkdirAll := files.mkdirAll
	files.mkdirAll = func(path string, perm fs.FileMode) error {
		if stage == rollbackFailureMkdir && filepath.Clean(path) == filepath.Clean(filepath.Dir(target)) {
			return errRollbackFault
		}
		return mkdirAll(path, perm)
	}
	createTemp := files.createTemp
	files.createTemp = func(dir, pattern string) (rollbackTemporaryFile, error) {
		destination := filepath.Join(dir, strings.TrimSuffix(pattern, ".rollback-*"))
		if filepath.Clean(destination) != filepath.Clean(target) {
			return createTemp(dir, pattern)
		}
		if stage == rollbackFailureCreate {
			return nil, errRollbackFault
		}
		file, err := createTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &faultRollbackFile{rollbackTemporaryFile: file, stage: stage}, nil
	}
	replace := files.durableReplace
	files.durableReplace = func(source, destination string) error {
		if filepath.Clean(destination) != filepath.Clean(target) {
			return replace(source, destination)
		}
		switch stage {
		case rollbackFailureReplace:
			return errRollbackFault
		case rollbackFailureParentSync:
			if err := platform.ReplaceFile(source, destination); err != nil {
				return err //nolint:wrapcheck // test double emulates durableReplace and must return platform.ReplaceFile's error unchanged before injecting the fault
			}
			return errRollbackFault
		case rollbackFailureMkdir, rollbackFailureCreate, rollbackFailureWrite, rollbackFailureSync, rollbackFailureClose:
			return replace(source, destination)
		default:
			return replace(source, destination)
		}
	}
	return files
}

func openPopulatedExportDB(t *testing.T) (db *DB, path string) {
	t.Helper()
	db, path = openTestDB(t)

	request1 := completeNetworkContainerRequest()
	request1.AuthorizationToken = "secret-token"
	record1 := NewNetworkContainerRecord(exportNC1, "7", "6", true, request1)
	changed, err := db.ApplyNetworkContainer(context.Background(), record1, []IPRecord{
		{ID: exportIPv4, IPAddress: exportIPv4Address, NCID: exportNC1, NCVersion: 7},
		{ID: exportIPv6, IPAddress: exportIPv6Address, NCID: exportNC1, NCVersion: 7},
	})
	require.NoError(t, err)
	require.True(t, changed)

	request2 := completeNetworkContainerRequest()
	request2.AuthorizationToken = "another-secret"
	record2 := NewNetworkContainerRecord(exportNC2, "8", "7", true, request2)
	changed, err = db.ApplyNetworkContainer(context.Background(), record2, []IPRecord{
		{ID: exportNet1, IPAddress: "10.1.0.4", NCID: exportNC2, NCVersion: 8},
	})
	require.NoError(t, err)
	require.True(t, changed)

	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		meta, metaErr := tx.Metadata()
		if metaErr != nil {
			return metaErr
		}
		meta.BootID = "export-boot"
		meta.Location = "eastus"
		meta.NetworkType = "azure"
		meta.OrchestratorType = "kubernetes"
		meta.NodeID = exportNodeID
		meta.Initialized = true
		meta.TimeStamp = testNow.Add(-time.Hour)
		if putErr := tx.PutMetadata(meta); putErr != nil {
			return putErr
		}
		if networkErr := tx.PutNetwork(NetworkRecord{
			NetworkName: exportNetworkName,
			NicInfo: &wireserver.InterfaceInfo{
				Subnet:       "10.0.0.0/24",
				Gateway:      exportGatewayIPv4,
				IsPrimary:    true,
				PrimaryIP:    exportIPv4Address,
				SecondaryIPs: []string{exportSecondaryIPv4},
			},
			Options: map[string]any{"custom": "value"},
		}); networkErr != nil {
			return networkErr
		}
		if contextErr := tx.PutOrchestratorContext("pod-1ns-1", []string{exportNC1, exportNC2}); contextErr != nil {
			return contextErr
		}
		return tx.PutPnPID("00:11:22:33:44:55", "PCI\\VEN_1234")
	}))

	endpoint := completeEndpointRecord()
	endpoint.IfnameToIPMap[exportIfnameEth0].NetworkContainerID = exportNC1
	endpoint.IfnameToIPMap["net1"].NetworkContainerID = exportNC2
	changed, err = db.AssignEndpoint(
		context.Background(),
		AssignmentRecord{
			Pod: PodIdentity{
				PodKey:           exportContainerID,
				InfraContainerID: exportContainerID,
				PodName:          exportPodName,
				PodNamespace:     exportPodNamespace,
			},
			IPIDs: []string{exportIPv4, exportIPv6, exportNet1},
		},
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	require.True(t, changed)

	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutDeleteIntent("deleted-container", DeleteIntent{CreatedAt: testNow.Add(-time.Minute)})
	}))
	return db, path
}

func exportPaths(t *testing.T) ExportOptions {
	t.Helper()
	dir := t.TempDir()
	return ExportOptions{
		CNSJSONPath:      filepath.Join(dir, "cns", "azure-cns.json"),
		EndpointJSONPath: filepath.Join(dir, "endpoint", "azure-endpoints.json"),
	}
}

func writeRollbackDestination(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func readRollbackFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func assertRollbackDestinationsMissing(t *testing.T, opts ExportOptions) {
	t.Helper()
	for _, path := range []string{opts.CNSJSONPath, opts.EndpointJSONPath} {
		_, err := os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func assertNoRollbackTemps(t *testing.T, opts ExportOptions) {
	t.Helper()
	for _, path := range []string{opts.CNSJSONPath, opts.EndpointJSONPath} {
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), filepath.Base(path)+".rollback-*"))
		require.NoError(t, err)
		assert.Empty(t, matches)
	}
}

func assertValidRollbackJSON(t *testing.T, path string) {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(readRollbackFile(t, path), &value))
}

func TestEncodeRollbackFilesDeterministic(t *testing.T) {
	db, _ := openPopulatedExportDB(t)
	snapshot := requireValidSnapshot(t, db)
	firstCNS, firstEndpoint, err := encodeRollbackFiles(snapshot)
	require.NoError(t, err)
	for range 20 {
		cnsData, endpointData, err := encodeRollbackFiles(snapshot)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(firstCNS, cnsData))
		assert.True(t, bytes.Equal(firstEndpoint, endpointData))
	}
}
