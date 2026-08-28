// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	stateMachineNCCount = 3
	stateMachineSteps   = 72
)

type stateMachineContainer struct {
	active       bool
	endpoint     bool
	retained     bool
	patchVersion int
	intent       time.Time
}

type stateMachineModel struct {
	containers [stateMachineNCCount]stateMachineContainer
	inventory  map[string]IPRecord
	bootID     string
}

func TestRandomizedMultiNCStateMachine(t *testing.T) {
	for _, seed := range []int64{12001, 12017, 12031} {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			runRandomizedMultiNCStateMachine(t, seed)
		})
	}
}

func runRandomizedMultiNCStateMachine(t *testing.T, seed int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	model := seedStateMachineInventory(t, db)
	now := time.Date(2026, time.July, 24, 1, 0, 0, 0, time.UTC)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{
			BootID:           "initial-boot",
			OrchestratorType: "KubernetesCRD",
			NodeID:           "node-state-machine",
			Location:         "eastus",
			NetworkType:      "azure",
			Initialized:      true,
			TimeStamp:        now,
		})
	}))
	model.bootID = "initial-boot"
	assertStateMachineModel(t, db, model)

	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // A fixed seed makes the state-machine sequence reproducible.
	for step := range stateMachineSteps {
		index := rng.Intn(stateMachineNCCount)
		switch rng.Intn(8) {
		case 0:
			assignStateMachineContainer(t, db, &model, index, now.Add(time.Duration(step)*time.Second))
		case 1:
			releaseStateMachineContainer(t, db, &model, index, now.Add(time.Duration(step)*time.Second))
		case 2:
			patchStateMachineContainer(t, db, &model, index, now.Add(time.Duration(step)*time.Second))
		case 3:
			deleteStateMachineEndpoint(t, db, &model, index)
		case 4:
			pruneStateMachineIntents(t, db, &model, now.Add(2*testDeleteIntentTTL+time.Duration(step)*time.Second))
		case 5:
			updateStateMachineInventory(t, db, &model, index, step)
		case 6:
			applyStateMachineBoot(t, db, &model, step, rng.Intn(2) == 0)
		case 7:
			require.NoError(t, db.Close())
			db, err = Open(path, Options{})
			require.NoError(t, err)
		}
		assertStateMachineModel(t, db, model)
	}

	before := requireValidSnapshot(t, db)
	opts := ExportOptions{
		CNSJSONPath:      filepath.Join(t.TempDir(), "rollback", "azure-cns.json"),
		EndpointJSONPath: filepath.Join(t.TempDir(), "rollback", "azure-endpoints.json"),
	}
	changed, err := db.ExportLegacy(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, changed)

	replayed, _ := openTestDB(t)
	changed, err = replayed.ImportLegacy(context.Background(), ImportOptions{
		CNSPath:             opts.CNSJSONPath,
		EndpointPath:        opts.EndpointJSONPath,
		ManageEndpointState: true,
		BootID:              "replayed-boot",
	})
	require.NoError(t, err)
	require.True(t, changed)
	after := requireValidSnapshot(t, replayed)
	assert.Equal(t, before.NetworkContainers, after.NetworkContainers)
	assert.Equal(t, before.IPs, after.IPs)
	assert.Equal(t, before.Networks, after.Networks)
	assert.Equal(t, before.OrchestratorContexts, after.OrchestratorContexts)
	assert.Equal(t, before.PnPIDByMAC, after.PnPIDByMAC)
	assert.Equal(t, sortedKeys(before.Endpoints), sortedKeys(after.Endpoints))
	for key, endpoint := range before.Endpoints {
		equal, err := endpointsEqual(endpoint, after.Endpoints[key])
		require.NoError(t, err)
		assert.True(t, equal, key)
	}
	assert.Equal(t, before.DeleteIntents, after.DeleteIntents)
}

func seedStateMachineInventory(t *testing.T, db *DB) stateMachineModel {
	t.Helper()
	model := stateMachineModel{inventory: map[string]IPRecord{}}
	for index := range stateMachineNCCount {
		records := stateMachineIPs(index, 1)
		_, err := db.ApplyNetworkContainer(
			context.Background(),
			testNetworkContainer(stateMachineNCID(index)),
			records,
		)
		require.NoError(t, err)
		for _, record := range records {
			model.inventory[record.ID] = record
		}
	}
	return model
}

func assignStateMachineContainer(
	t *testing.T,
	db *DB,
	model *stateMachineModel,
	index int,
	now time.Time,
) {
	t.Helper()
	state := &model.containers[index]
	if state.active || !state.intent.IsZero() {
		return
	}
	endpoint := stateMachineEndpoint(index, state.patchVersion)
	for _, assignment := range stateMachineAssignments(index) {
		changed, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			now,
			testDeleteIntentTTL,
		)
		require.NoError(t, err)
		require.True(t, changed)
	}
	state.active = true
	state.endpoint = true
	state.retained = false
}

func releaseStateMachineContainer(
	t *testing.T,
	db *DB,
	model *stateMachineModel,
	index int,
	now time.Time,
) {
	t.Helper()
	state := &model.containers[index]
	if !state.active {
		return
	}
	pod := stateMachineAssignments(index)[0].Pod
	if state.retained {
		pod.PodKey = stateMachineContainerID(index)
		pod.InterfaceID = ""
	}
	changed, err := db.ReleaseEndpoint(context.Background(), pod, now)
	require.NoError(t, err)
	require.True(t, changed)
	state.active = false
	state.retained = false
	state.intent = now.UTC()
}

func patchStateMachineContainer(
	t *testing.T,
	db *DB,
	model *stateMachineModel,
	index int,
	now time.Time,
) {
	t.Helper()
	state := &model.containers[index]
	if !state.active {
		return
	}
	pod := stateMachineAssignments(index)[0].Pod
	if state.retained {
		pod.PodKey = stateMachineContainerID(index)
		pod.InterfaceID = ""
	}
	state.patchVersion++
	changed, err := db.PatchEndpoint(
		context.Background(),
		pod,
		stateMachineEndpoint(index, state.patchVersion),
		now,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	require.True(t, changed)
}

func deleteStateMachineEndpoint(t *testing.T, db *DB, model *stateMachineModel, index int) {
	t.Helper()
	state := &model.containers[index]
	if state.active || !state.endpoint {
		return
	}
	changed, err := db.DeleteEndpointRecord(context.Background(), stateMachineContainerID(index))
	require.NoError(t, err)
	require.True(t, changed)
	state.endpoint = false
}

func pruneStateMachineIntents(t *testing.T, db *DB, model *stateMachineModel, now time.Time) {
	t.Helper()
	want := 0
	for index := range stateMachineNCCount {
		state := &model.containers[index]
		if !state.intent.IsZero() && !deleteIntentLive(DeleteIntent{CreatedAt: state.intent}, now, testDeleteIntentTTL) {
			state.intent = time.Time{}
			want++
		}
	}
	got, err := db.PruneDeleteIntents(context.Background(), now, testDeleteIntentTTL)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func updateStateMachineInventory(
	t *testing.T,
	db *DB,
	model *stateMachineModel,
	index, version int,
) {
	t.Helper()
	if model.containers[index].endpoint {
		return
	}
	records := stateMachineIPs(index, version+2)
	changed, err := db.ApplyNetworkContainer(
		context.Background(),
		testNetworkContainer(stateMachineNCID(index)),
		records,
	)
	require.NoError(t, err)
	require.True(t, changed)
	for _, record := range records {
		model.inventory[record.ID] = record
	}
}

func applyStateMachineBoot(
	t *testing.T,
	db *DB,
	model *stateMachineModel,
	step int,
	clearEndpoints bool,
) {
	t.Helper()
	bootID := fmt.Sprintf("boot-%d", step)
	changed, err := db.ApplyBoot(context.Background(), bootID, BootPolicy{
		ClearEndpoints: clearEndpoints,
		ResetReadiness: step%3 == 0,
	})
	require.NoError(t, err)
	require.True(t, changed)
	model.bootID = bootID
	for index := range stateMachineNCCount {
		state := &model.containers[index]
		wasActive := state.active
		state.intent = time.Time{}
		if clearEndpoints {
			state.active = false
			state.endpoint = false
			state.retained = false
			continue
		}
		state.active = state.endpoint
		state.retained = state.endpoint && (state.retained || !wasActive)
	}
}

func assertStateMachineModel(t *testing.T, db *DB, model stateMachineModel) {
	t.Helper()
	snapshot := requireValidSnapshot(t, db)
	assert.Equal(t, model.bootID, snapshot.Metadata.BootID)
	assert.Equal(t, model.inventory, snapshot.IPs)

	wantAssignments := map[string]AssignmentRecord{}
	wantOwners := map[string]string{}
	wantEndpoints := map[string]EndpointRecord{}
	wantIntents := map[string]DeleteIntent{}
	for index, state := range model.containers {
		containerID := stateMachineContainerID(index)
		if state.endpoint {
			wantEndpoints[containerID] = stateMachineEndpoint(index, state.patchVersion)
		}
		if !state.intent.IsZero() {
			wantIntents[containerID] = DeleteIntent{CreatedAt: state.intent}
		}
		if !state.active {
			continue
		}
		assignments := stateMachineAssignments(index)
		if state.retained {
			assignments = []AssignmentRecord{{
				Pod: PodIdentity{
					PodKey:           containerID,
					InfraContainerID: containerID,
					PodName:          stateMachinePodName(index),
					PodNamespace:     "state-machine",
				},
				IPIDs: stateMachineIPIDs(index),
			}}
		}
		for _, assignment := range assignments {
			wantAssignments[assignment.Pod.PodKey] = assignment
			for _, ipID := range assignment.IPIDs {
				wantOwners[ipID] = assignment.Pod.PodKey
			}
		}
	}
	assert.Equal(t, wantAssignments, snapshot.Assignments)
	assert.Equal(t, wantOwners, snapshot.IPOwners)
	assert.Equal(t, sortedKeys(wantEndpoints), sortedKeys(snapshot.Endpoints))
	for key, want := range wantEndpoints {
		equal, err := endpointsEqual(want, snapshot.Endpoints[key])
		require.NoError(t, err)
		assert.True(t, equal, key)
	}
	assert.Equal(t, wantIntents, snapshot.DeleteIntents)
}

func stateMachineAssignments(index int) []AssignmentRecord {
	ids := stateMachineIPIDs(index)
	containerID := stateMachineContainerID(index)
	return []AssignmentRecord{
		{
			Pod: PodIdentity{
				PodKey:           fmt.Sprintf("interface-%d-primary", index),
				InfraContainerID: containerID,
				InterfaceID:      fmt.Sprintf("interface-%d-primary", index),
				PodName:          stateMachinePodName(index),
				PodNamespace:     "state-machine",
			},
			IPIDs: []string{ids[0], ids[1]},
		},
		{
			Pod: PodIdentity{
				PodKey:           fmt.Sprintf("interface-%d-secondary", index),
				InfraContainerID: containerID,
				InterfaceID:      fmt.Sprintf("interface-%d-secondary", index),
				PodName:          stateMachinePodName(index),
				PodNamespace:     "state-machine",
			},
			IPIDs: []string{ids[2]},
		},
	}
}

func stateMachineEndpoint(index, patchVersion int) EndpointRecord {
	records := stateMachineIPs(index, 1)
	return EndpointRecord{
		PodName:      stateMachinePodName(index),
		PodNamespace: "state-machine",
		IfnameToIPMap: map[string]*IPInfoRecord{
			"eth0": {
				IPv4:               []net.IPNet{mustIPNetValue(records[0].IPAddress, 24)},
				IPv6:               []net.IPNet{mustIPNetValue(records[1].IPAddress, 64)},
				HostVethName:       fmt.Sprintf("veth-%d-%d", index, patchVersion),
				MACAddress:         fmt.Sprintf("02:00:00:10:%02x:%02x", index, index+1),
				NetworkContainerID: stateMachineNCID(index),
			},
			"net1": {
				IPv4:               []net.IPNet{mustIPNetValue(records[2].IPAddress, 24)},
				HostVethName:       fmt.Sprintf("net1-%d-%d", index, patchVersion),
				MACAddress:         fmt.Sprintf("02:00:00:20:%02x:%02x", index, index+1),
				NetworkContainerID: stateMachineNCID(index),
			},
		},
	}
}

func stateMachineIPs(index, version int) []IPRecord {
	ids := stateMachineIPIDs(index)
	return []IPRecord{
		{
			ID:        ids[0],
			IPAddress: fmt.Sprintf("10.%d.0.10", 80+index),
			NCID:      stateMachineNCID(index),
			NCVersion: version,
		},
		{
			ID:        ids[1],
			IPAddress: fmt.Sprintf("fd%02x::10", 80+index),
			NCID:      stateMachineNCID(index),
			NCVersion: version,
		},
		{
			ID:        ids[2],
			IPAddress: fmt.Sprintf("10.%d.1.10", 80+index),
			NCID:      stateMachineNCID(index),
			NCVersion: version,
		},
	}
}

func stateMachineIPIDs(index int) []string {
	return []string{
		fmt.Sprintf("10000000-0000-4000-8000-%012d", index*16+1),
		fmt.Sprintf("10000000-0000-4000-8000-%012d", index*16+2),
		fmt.Sprintf("10000000-0000-4000-8000-%012d", index*16+3),
	}
}

func stateMachineNCID(index int) string {
	return fmt.Sprintf("state-machine-nc-%d", index)
}

func stateMachineContainerID(index int) string {
	return fmt.Sprintf("state-machine-container-%d", index)
}

func stateMachinePodName(index int) string {
	return fmt.Sprintf("state-machine-pod-%d", index)
}

func mustIPNetValue(address string, bits int) net.IPNet {
	ip := net.ParseIP(address)
	totalBits := 128
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
		totalBits = 32
	}
	return net.IPNet{IP: ip, Mask: net.CIDRMask(bits, totalBits)}
}

func TestLegacyImportRollbackRoundTrip(t *testing.T) {
	cnsData, endpointData := completeLegacyImportData(t)
	source, _ := openTestDB(t)
	importOpts := writeLegacyImportFiles(t, cnsData, endpointData, true)
	changed, err := source.ImportLegacy(context.Background(), importOpts)
	require.NoError(t, err)
	require.True(t, changed)
	imported := requireValidSnapshot(t, source)

	exportOpts := exportPaths(t)
	changed, err = source.ExportLegacy(context.Background(), exportOpts)
	require.NoError(t, err)
	require.True(t, changed)

	reimportedDB, _ := openTestDB(t)
	changed, err = reimportedDB.ImportLegacy(context.Background(), ImportOptions{
		CNSPath:             exportOpts.CNSJSONPath,
		EndpointPath:        exportOpts.EndpointJSONPath,
		ManageEndpointState: true,
		BootID:              "reimported-boot",
	})
	require.NoError(t, err)
	require.True(t, changed)
	reimported := requireValidSnapshot(t, reimportedDB)

	assert.Equal(t, logicalRoundTripSnapshot(imported), logicalRoundTripSnapshot(reimported))
}

func logicalRoundTripSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Metadata.Authority = ""
	snapshot.Metadata.Generation = 0
	snapshot.Metadata.BootID = ""
	snapshot.Metadata.LegacyImportComplete = false
	snapshot.Metadata.RollbackExportComplete = false
	return snapshot
}

func TestConcurrentImportExportBootAndGateCancellation(t *testing.T) {
	cnsData, endpointData := completeLegacyImportData(t)
	importDB, _ := openTestDB(t)
	importOpts := writeLegacyImportFiles(t, cnsData, endpointData, true)
	exportDB, _ := openPopulatedExportDB(t)
	exportOpts := exportPaths(t)
	bootDB, _ := openTestDB(t)
	writeSnapshot(t, bootDB, completeSnapshot())
	gateDB, _ := openTestDB(t)
	gateDB.writeGate <- struct{}{}
	t.Cleanup(func() {
		select {
		case <-gateDB.writeGate:
		default:
		}
	})
	gateCtx, cancelGate := context.WithCancel(context.Background())
	t.Cleanup(cancelGate)

	type result struct {
		name string
		err  error
	}
	start := make(chan struct{})
	gateStarted := make(chan struct{})
	results := make(chan result, 4)
	var ready sync.WaitGroup
	ready.Add(4)
	go func() {
		ready.Done()
		<-start
		_, err := importDB.ImportLegacy(context.Background(), importOpts)
		results <- result{name: "import", err: err}
	}()
	go func() {
		ready.Done()
		<-start
		_, err := exportDB.ExportLegacy(context.Background(), exportOpts)
		results <- result{name: "export", err: err}
	}()
	go func() {
		ready.Done()
		<-start
		_, err := bootDB.ApplyBoot(
			context.Background(),
			"concurrent-boot",
			BootPolicy{ClearEndpoints: true, ResetReadiness: true},
		)
		results <- result{name: "boot", err: err}
	}()
	go func() {
		ready.Done()
		<-start
		close(gateStarted)
		_, err := gateDB.ApplyBoot(gateCtx, "canceled-boot", BootPolicy{})
		results <- result{name: "gate cancellation", err: err}
	}()
	ready.Wait()
	close(start)
	<-gateStarted
	cancelGate()

	for range 4 {
		got := <-results
		if got.name == "gate cancellation" {
			require.ErrorIs(t, got.err, context.Canceled)
			continue
		}
		require.NoError(t, got.err, got.name)
	}
	<-gateDB.writeGate

	requireValidSnapshot(t, importDB)
	requireValidSnapshot(t, exportDB)
	requireValidSnapshot(t, bootDB)
	requireValidSnapshot(t, gateDB)
	assert.Equal(t, "concurrent-boot", readMetadata(t, bootDB).BootID)
	assert.Empty(t, readMetadata(t, gateDB).BootID)
}

func TestConcurrentConflictingOwnersHaveSingleWinner(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	other := assignment
	other.Pod = PodIdentity{
		PodKey:           "other-container",
		InfraContainerID: "other-container",
		PodName:          "other-pod",
		PodNamespace:     "ns-1",
	}
	otherEndpoint := endpoint
	otherEndpoint.PodName = other.Pod.PodName

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, candidate := range []struct {
		assignment AssignmentRecord
		endpoint   EndpointRecord
	}{
		{assignment: assignment, endpoint: endpoint},
		{assignment: other, endpoint: otherEndpoint},
	} {
		go func() {
			ready.Done()
			<-start
			_, err := db.AssignEndpoint(
				context.Background(),
				candidate.assignment,
				candidate.endpoint,
				testNow,
				testDeleteIntentTTL,
			)
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrIPAlreadyAssigned):
			conflicts++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	snapshot := requireValidSnapshot(t, db)
	assert.Len(t, snapshot.Assignments, 1)
	assert.Len(t, snapshot.IPOwners, len(assignment.IPIDs))
}
