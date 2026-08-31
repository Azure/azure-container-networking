// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const maxFuzzStateBytes = 128 << 10

func FuzzStrictLegacyCNSImport(f *testing.F) {
	valid, _ := completeLegacyImportData(f)
	for _, seed := range [][]byte{
		valid,
		{},
		[]byte("null"),
		[]byte(`{"ContainerNetworkService":{}}`),
		[]byte(`{"ContainerNetworkService":{},"containernetworkservice":{}}`),
		[]byte(`{"ContainerNetworkService":{"unknown":true}}`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzStateBytes {
			t.Skip()
		}
		snapshot, err := parseLegacyCNS(data, "fuzz-boot")
		if err != nil {
			return
		}
		require.NoError(t, snapshot.Validate())
		for id := range snapshot.NetworkContainers {
			record := snapshot.NetworkContainers[id]
			require.Empty(t, record.Request.AuthorizationToken, id)
			require.Nil(t, record.Request.SecondaryIPConfigs, id)
		}
	})
}

func FuzzStrictLegacyEndpointImport(f *testing.F) {
	cnsData, valid := completeLegacyImportData(f)
	for _, seed := range [][]byte{
		valid,
		{},
		[]byte("null"),
		[]byte(`{"Endpoints":{}}`),
		[]byte(`{"Endpoints":null}`),
		[]byte(`{"Endpoints":{},"endpoints":{}}`),
		[]byte(`{"Endpoints":{},"DeleteIntents":null}`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzStateBytes {
			t.Skip()
		}
		snapshot, err := parseLegacyCNS(cnsData, "fuzz-boot")
		require.NoError(t, err)
		if err := addLegacyEndpoints(&snapshot, data); err != nil {
			return
		}
		require.NoError(t, snapshot.Validate())
	})
}

func FuzzSnapshotValidationAndStrictRecordDecoding(f *testing.F) {
	validSnapshot, err := json.Marshal(completeSnapshot()) //nolint:musttag // snapshot includes legacy wire structs with default JSON field names
	require.NoError(f, err)
	validIP, err := json.Marshal(completeSnapshot().IPs["ip-v4"])
	require.NoError(f, err)
	for _, seed := range []struct {
		kind byte
		data []byte
	}{
		{kind: 0, data: validSnapshot},
		{kind: 1, data: validIP},
		{kind: 2, data: []byte(`{"id":"nc-1","request":{"NetworkContainerid":"nc-1"}}`)},
		{kind: 3, data: []byte(`{"podName":"pod","ifnameToIPMap":{}}`)},
		{kind: 0, data: []byte("null")},
		{kind: 1, data: []byte(`{"id":"a","ID":"b"}`)},
		{kind: 2, data: []byte(`{} {}`)},
	} {
		f.Add(seed.kind, seed.data)
	}

	f.Fuzz(func(t *testing.T, kind byte, data []byte) {
		if len(data) > maxFuzzStateBytes {
			t.Skip()
		}
		switch kind % 4 {
		case 0:
			var snapshot Snapshot
			if err := decodeStrictJSON(data, &snapshot); err != nil {
				return
			}
			_ = snapshot.Validate()
		case 1:
			var record IPRecord
			_ = decodeJSONValue(data, &record)
		case 2:
			var record NetworkContainerRecord
			_ = decodeJSONValue(data, &record)
		case 3:
			var record EndpointRecord
			_ = decodeJSONValue(data, &record)
		}
	})
}

func FuzzExportImportRoundTrip(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4})
	f.Add([]byte{4, 1, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > 64 {
			t.Skip()
		}
		ncCount := 1 + int(data[0]%4)
		ipsPerNC := 1 + int(data[len(data)-1]%4)
		durable := fuzzDurableState(ncCount, ipsPerNC)

		dir := t.TempDir()
		source, err := Open(filepath.Join(dir, "source.db"), Options{NoSync: true})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, source.Close()) })
		changed, err := source.ReplaceDurableState(context.Background(), 0, durable)
		require.NoError(t, err)
		require.True(t, changed)
		require.NoError(t, source.Update(context.Background(), func(tx *WriteTx) error {
			return tx.PutMetadata(Metadata{
				BootID:           "source-boot",
				OrchestratorType: importOrchestratorType,
				NodeID:           "node-fuzz",
				Location:         hardeningLocation,
				NetworkType:      exportNetworkType,
				Initialized:      true,
				TimeStamp:        time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC),
			})
		}))
		before, err := source.Snapshot(context.Background())
		require.NoError(t, err)

		opts := ExportOptions{
			CNSJSONPath:      filepath.Join(dir, "rollback", "azure-cns.json"),
			EndpointJSONPath: filepath.Join(dir, "rollback", "azure-endpoints.json"),
		}
		changed, err = source.ExportLegacy(context.Background(), opts)
		require.NoError(t, err)
		require.True(t, changed)
		exported, err := os.ReadFile(opts.CNSJSONPath)
		require.NoError(t, err)
		require.NotContains(t, string(exported), "fuzz-secret")

		target, err := Open(filepath.Join(dir, "target.db"), Options{NoSync: true})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, target.Close()) })
		changed, err = target.ImportLegacy(context.Background(), ImportOptions{
			CNSPath:             opts.CNSJSONPath,
			EndpointPath:        opts.EndpointJSONPath,
			ManageEndpointState: true,
			BootID:              "target-boot",
		})
		require.NoError(t, err)
		require.True(t, changed)
		after, err := target.Snapshot(context.Background())
		require.NoError(t, err)
		require.Equal(t, before.NetworkContainers, after.NetworkContainers)
		require.Equal(t, before.IPs, after.IPs)
		require.Equal(t, before.Networks, after.Networks)
		require.Equal(t, before.OrchestratorContexts, after.OrchestratorContexts)
		require.Equal(t, before.PnPIDByMAC, after.PnPIDByMAC)
	})
}

func fuzzDurableState(ncCount, ipsPerNC int) DurableState {
	durable := NewDurableState()
	for ncIndex := range ncCount {
		ncID := fmt.Sprintf("nc-%d", ncIndex)
		request := completeNetworkContainerRequest()
		request.AuthorizationToken = "fuzz-secret"
		record := NewNetworkContainerRecord(ncID, "2", "1", true, request)
		durable.NetworkContainers[ncID] = record
		for ipIndex := range ipsPerNC {
			id := fmt.Sprintf("00000000-0000-4000-8000-%012d", ncIndex*16+ipIndex+1)
			address := netip.AddrFrom4([4]byte{10, 64 + byte(ncIndex), 0, byte(ipIndex + 4)})
			if ipIndex%2 == 1 {
				address = netip.AddrFrom16([16]byte{0xfd, byte(ncIndex + 1), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(ipIndex + 4)})
			}
			durable.IPs[id] = IPRecord{
				ID:        id,
				IPAddress: address.String(),
				NCID:      ncID,
				NCVersion: 2,
			}
		}
		durable.OrchestratorContexts[fmt.Sprintf("context-%d", ncIndex)] = []string{ncID}
		durable.PnPIDByMAC[fmt.Sprintf("02:00:00:00:%02x:%02x", ncIndex, ncIndex+1)] = "pnp-" + ncID
		durable.Networks["network-"+ncID] = NetworkRecord{NetworkName: "network-" + ncID}
	}
	return durable
}

func TestStrictJSONRejectsTrailingAndDuplicateValues(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "null", data: []byte("null")},
		{name: hardeningMultipleValues, data: []byte(`{} {}`)},
		{name: "case-insensitive duplicate", data: []byte(`{"id":"a","ID":"b"}`)},
		{name: "nested duplicate", data: []byte(`{"outer":{"a":1,"A":2}}`)},
		{name: "invalid closing delimiter", data: []byte(`{"outer":[1,2}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var destination map[string]any
			require.Error(t, decodeJSONDocument(tt.data, &destination))
		})
	}
}

func TestStrictJSONAcceptsBoundedNestedDocument(t *testing.T) {
	data := []byte(`{"array":[1,{"nested":true},null],"object":{"value":"ok"}}`)
	var destination map[string]any
	require.NoError(t, decodeJSONDocument(data, &destination))
	require.Equal(t, "ok", destination["object"].(map[string]any)["value"])
}
