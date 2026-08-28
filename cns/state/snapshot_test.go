// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

func TestNewSnapshotInitializesMaps(t *testing.T) {
	snapshot := NewSnapshot()

	assert.NotNil(t, snapshot.NetworkContainers)
	assert.NotNil(t, snapshot.IPs)
	assert.NotNil(t, snapshot.Networks)
	assert.NotNil(t, snapshot.OrchestratorContexts)
	assert.NotNil(t, snapshot.PnPIDByMAC)
	assert.NotNil(t, snapshot.Assignments)
	assert.NotNil(t, snapshot.IPOwners)
	assert.NotNil(t, snapshot.Endpoints)
	assert.NotNil(t, snapshot.DeleteIntents)
}

func TestSnapshotEmptyDatabase(t *testing.T) {
	db, _ := openTestDB(t)

	snapshot, err := db.Snapshot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, snapshot.Metadata.SchemaVersion)
	assert.Equal(t, AuthorityBolt, snapshot.Metadata.Authority)
	assert.NotNil(t, snapshot.NetworkContainers)
	assert.NotNil(t, snapshot.IPs)
	assert.NotNil(t, snapshot.Networks)
	assert.NotNil(t, snapshot.OrchestratorContexts)
	assert.NotNil(t, snapshot.PnPIDByMAC)
	assert.NotNil(t, snapshot.Assignments)
	assert.NotNil(t, snapshot.IPOwners)
	assert.NotNil(t, snapshot.Endpoints)
	assert.NotNil(t, snapshot.DeleteIntents)
}

func TestSnapshotCompleteDualStackMultiNICState(t *testing.T) {
	db, _ := openTestDB(t)
	want := completeSnapshot()
	writeSnapshot(t, db, want)

	got, err := db.Snapshot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, want.NetworkContainers, got.NetworkContainers)
	assert.Equal(t, want.IPs, got.IPs)
	assert.Equal(t, want.Networks, got.Networks)
	assert.Equal(t, want.OrchestratorContexts, got.OrchestratorContexts)
	assert.Equal(t, want.PnPIDByMAC, got.PnPIDByMAC)
	assert.Equal(t, want.Assignments, got.Assignments)
	assert.Equal(t, want.IPOwners, got.IPOwners)
	assert.Equal(t, want.Endpoints, got.Endpoints)
	assert.Equal(t, want.DeleteIntents, got.DeleteIntents)
	assert.Equal(
		t,
		"10.0.0.4",
		got.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0].IP.String(),
	)
}

func TestSnapshotInfraContainerIdentity(t *testing.T) {
	db, _ := openTestDB(t)
	want := completeSnapshot()
	want.Assignments = map[string]AssignmentRecord{
		"container-1": {
			Pod: PodIdentity{
				PodKey:           "container-1",
				InfraContainerID: "container-1",
				PodName:          "pod-1",
				PodNamespace:     "ns-1",
			},
			IPIDs: []string{"ip-v4", "ip-v6", "ip-secondary"},
		},
	}
	want.IPOwners = map[string]string{
		"ip-v4":        "container-1",
		"ip-v6":        "container-1",
		"ip-secondary": "container-1",
	}
	writeSnapshot(t, db, want)

	got, err := db.Snapshot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, want.Assignments, got.Assignments)
	assert.Equal(t, want.IPOwners, got.IPOwners)
}

func TestSnapshotRejectsMalformedValuesInEveryBucket(t *testing.T) {
	tests := []struct {
		name   string
		bucket []byte
		key    []byte
	}{
		{name: "metadata", bucket: bucketMetadata, key: metaKeyService},
		{name: "network containers", bucket: bucketNetworkContainers, key: []byte("nc-1")},
		{name: "IPs", bucket: bucketIPs, key: []byte("ip-1")},
		{name: "networks", bucket: bucketNetworks, key: []byte("network-1")},
		{name: "orchestrator contexts", bucket: bucketOrchestratorContexts, key: []byte("context-1")},
		{name: "PnP IDs", bucket: bucketPnPIDByMAC, key: []byte("00:11:22:33:44:55")},
		{name: "assignments", bucket: bucketAssignments, key: []byte("interface-1")},
		{name: "IP owners", bucket: bucketIPOwners, key: []byte("ip-1")},
		{name: "endpoints", bucket: bucketEndpoints, key: []byte("container-1")},
		{name: "delete intents", bucket: bucketDeleteIntents, key: []byte("nc-1")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			putRaw(t, db, tt.bucket, tt.key, []byte("{"))

			_, err := db.Snapshot(context.Background())

			require.ErrorIs(t, err, ErrCorrupt)
			assert.NotErrorIs(t, err, ErrInconsistentState)
			assert.Contains(t, err.Error(), string(tt.bucket))
			assert.Contains(t, err.Error(), string(tt.key))
		})
	}
}

func TestSnapshotRejectsWrongWireShapes(t *testing.T) {
	tests := []struct {
		name   string
		bucket []byte
		key    []byte
		value  []byte
	}{
		{name: "metadata array", bucket: bucketMetadata, key: metaKeyService, value: []byte("[]")},
		{name: "network container array", bucket: bucketNetworkContainers, key: []byte("nc-1"), value: []byte("[]")},
		{name: "IP array", bucket: bucketIPs, key: []byte("ip-1"), value: []byte("[]")},
		{name: "network array", bucket: bucketNetworks, key: []byte("network-1"), value: []byte("[]")},
		{name: "orchestrator object", bucket: bucketOrchestratorContexts, key: []byte("context-1"), value: []byte("{}")},
		{name: "PnP array", bucket: bucketPnPIDByMAC, key: []byte("00:11:22:33:44:55"), value: []byte("[]")},
		{name: "assignment array", bucket: bucketAssignments, key: []byte("interface-1"), value: []byte("[]")},
		{name: "owner array", bucket: bucketIPOwners, key: []byte("ip-1"), value: []byte("[]")},
		{name: "endpoint array", bucket: bucketEndpoints, key: []byte("container-1"), value: []byte("[]")},
		{name: "delete intent array", bucket: bucketDeleteIntents, key: []byte("nc-1"), value: []byte("[]")},
		{name: "null value", bucket: bucketEndpoints, key: []byte("container-1"), value: []byte("null")},
		{
			name:   "unknown field",
			bucket: bucketNetworks,
			key:    []byte("network-1"),
			value:  []byte(`{"networkName":"network-1","unknown":true}`),
		},
		{
			name:   "multiple values",
			bucket: bucketDeleteIntents,
			key:    []byte("nc-1"),
			value:  []byte(`{"createdAt":"2026-07-23T22:00:00Z"} {}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			putRaw(t, db, tt.bucket, tt.key, tt.value)

			_, err := db.Snapshot(context.Background())

			require.ErrorIs(t, err, ErrCorrupt)
			assert.NotErrorIs(t, err, ErrInconsistentState)
		})
	}
}

func TestSnapshotRejectsLogicalInconsistencies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{
			name: "network container record ID mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.NetworkContainers["nc-1"]
				record.ID = "other"
				snapshot.NetworkContainers["nc-1"] = record
			},
		},
		{
			name: "network container request ID mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.NetworkContainers["nc-1"]
				record.Request.NetworkContainerid = "other"
				snapshot.NetworkContainers["nc-1"] = record
			},
		},
		{
			name: "network container secret",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.NetworkContainers["nc-1"]
				record.Request.AuthorizationToken = "secret"
				snapshot.NetworkContainers["nc-1"] = record
			},
		},
		{
			name: "network container embedded secondary IPs",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.NetworkContainers["nc-1"]
				record.Request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{"ip-1": {}}
				snapshot.NetworkContainers["nc-1"] = record
			},
		},
		{
			name: "IP record ID mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.IPs["ip-v4"]
				record.ID = "other"
				snapshot.IPs["ip-v4"] = record
			},
		},
		{
			name: "duplicate canonical IP address",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPs["ip-alias"] = IPRecord{
					ID:        "ip-alias",
					IPAddress: "::ffff:10.0.0.4",
					NCID:      "nc-1",
				}
			},
		},
		{
			name: "IP missing network container",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.IPs["ip-v4"]
				record.NCID = "missing"
				snapshot.IPs["ip-v4"] = record
			},
		},
		{
			name: "network record name mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Networks["network-1"]
				record.NetworkName = "other"
				snapshot.Networks["network-1"] = record
			},
		},
		{
			name: "orchestrator context duplicate NC",
			mutate: func(snapshot *Snapshot) {
				snapshot.OrchestratorContexts["context-1"] = []string{"nc-1", "nc-1"}
			},
		},
		{
			name: "orchestrator context missing NC",
			mutate: func(snapshot *Snapshot) {
				snapshot.OrchestratorContexts["context-1"] = []string{"missing"}
			},
		},
		{
			name: "canonical PnP MAC alias",
			mutate: func(snapshot *Snapshot) {
				snapshot.PnPIDByMAC["00-11-22-33-44-55"] = "pnp-2"
			},
		},
		{
			name: "empty PnP ID",
			mutate: func(snapshot *Snapshot) {
				snapshot.PnPIDByMAC["00:11:22:33:44:55"] = ""
			},
		},
		{
			name: "assignment key mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.Pod.PodKey = "other"
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "assignment empty infra container",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.Pod.InfraContainerID = ""
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "infra-key assignment has mismatched key",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.Pod.InterfaceID = ""
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "interface assignment has mismatched key",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.Pod.InterfaceID = "other"
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "assignment has no IPs",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.IPIDs = []string{}
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "assignment duplicate IP ID",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.IPIDs = []string{"ip-v4", "ip-v4"}
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "assignment missing IP",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.IPIDs = []string{"missing"}
				snapshot.Assignments["iface-primary"] = record
			},
		},
		{
			name: "IP in multiple assignments",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-secondary"]
				record.IPIDs = append(record.IPIDs, "ip-v4")
				snapshot.Assignments["iface-secondary"] = record
			},
		},
		{
			name: "assignment IP missing owner",
			mutate: func(snapshot *Snapshot) {
				delete(snapshot.IPOwners, "ip-v4")
			},
		},
		{
			name: "assignment owner mismatch",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPOwners["ip-v4"] = "iface-secondary"
			},
		},
		{
			name: "owner missing IP",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPOwners["missing"] = "iface-primary"
			},
		},
		{
			name: "owner missing assignment",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPs["ip-unassigned"] = IPRecord{
					ID:        "ip-unassigned",
					IPAddress: "10.2.0.4",
					NCID:      "nc-1",
				}
				snapshot.IPOwners["ip-unassigned"] = "missing"
			},
		},
		{
			name: "owner assignment missing IP",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPs["ip-unassigned"] = IPRecord{
					ID:        "ip-unassigned",
					IPAddress: "10.2.0.4",
					NCID:      "nc-1",
				}
				snapshot.IPOwners["ip-unassigned"] = "iface-primary"
			},
		},
		{
			name: "missing endpoint",
			mutate: func(snapshot *Snapshot) {
				delete(snapshot.Endpoints, "container-1")
			},
		},
		{
			name: "endpoint pod identity mismatch",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Endpoints["container-1"]
				record.PodName = "other"
				snapshot.Endpoints["container-1"] = record
			},
		},
		{
			name: "assigned IP missing from endpoint",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4 = nil
			},
		},
		{
			name: "endpoint NC mismatch",
			mutate: func(snapshot *Snapshot) {
				snapshot.NetworkContainers["nc-2"] = NewNetworkContainerRecord(
					"nc-2",
					"2",
					"1",
					true,
					completeNetworkContainerRequest(),
				)
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].NetworkContainerID = "nc-2"
			},
		},
		{
			name: "duplicate endpoint IP ownership",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-2"] = EndpointRecord{
					IfnameToIPMap: map[string]*IPInfoRecord{
						"eth0": {
							IPv4: []net.IPNet{{
								IP:   net.ParseIP("10.0.0.4"),
								Mask: net.CIDRMask(24, 32),
							}},
							NetworkContainerID: "nc-1",
						},
					},
				}
			},
		},
		{
			name: "empty endpoint interface name",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap[""] = &IPInfoRecord{}
			},
		},
		{
			name: "endpoint missing NC",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].NetworkContainerID = "missing"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			snapshot := completeSnapshot()
			tt.mutate(&snapshot)
			writeSnapshot(t, db, snapshot)

			_, err := db.Snapshot(context.Background())

			require.ErrorIs(t, err, ErrInconsistentState)
			assert.NotErrorIs(t, err, ErrCorrupt)
		})
	}
}

func TestSnapshotValidationRejectsEmptyKeys(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{
			name: "network container",
			mutate: func(snapshot *Snapshot) {
				snapshot.NetworkContainers[""] = snapshot.NetworkContainers["nc-1"]
			},
		},
		{
			name: "IP",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPs[""] = IPRecord{}
			},
		},
		{
			name: "network",
			mutate: func(snapshot *Snapshot) {
				snapshot.Networks[""] = NetworkRecord{}
			},
		},
		{
			name: "orchestrator context",
			mutate: func(snapshot *Snapshot) {
				snapshot.OrchestratorContexts[""] = []string{}
			},
		},
		{
			name: "PnP MAC",
			mutate: func(snapshot *Snapshot) {
				snapshot.PnPIDByMAC[""] = "pnp"
			},
		},
		{
			name: "assignment",
			mutate: func(snapshot *Snapshot) {
				snapshot.Assignments[""] = AssignmentRecord{}
			},
		},
		{
			name: "IP owner",
			mutate: func(snapshot *Snapshot) {
				snapshot.IPOwners[""] = "iface-primary"
			},
		},
		{
			name: "endpoint",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints[""] = EndpointRecord{}
			},
		},
		{
			name: "delete intent",
			mutate: func(snapshot *Snapshot) {
				snapshot.DeleteIntents[""] = DeleteIntent{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := completeSnapshot()
			tt.mutate(&snapshot)

			err := snapshot.validate()

			require.ErrorIs(t, err, ErrInconsistentState)
			assert.NotErrorIs(t, err, ErrCorrupt)
		})
	}
}

func TestSnapshotRejectsStructuralNetworkData(t *testing.T) {
	tests := []struct {
		name   string
		bucket []byte
		key    string
		mutate func(*Snapshot)
	}{
		{
			name:   "malformed IP record address",
			bucket: bucketIPs,
			key:    "ip-v4",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.IPs["ip-v4"]
				record.IPAddress = "not-an-ip"
				snapshot.IPs["ip-v4"] = record
			},
		},
		{
			name:   "malformed network prefix",
			bucket: bucketNetworks,
			key:    "network-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Networks["network-1"].NicInfo.Subnet = "10.0.0.0/not-a-prefix"
			},
		},
		{
			name:   "malformed network address",
			bucket: bucketNetworks,
			key:    "network-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Networks["network-1"].NicInfo.Gateway = "not-an-ip"
			},
		},
		{
			name:   "malformed PnP MAC",
			bucket: bucketPnPIDByMAC,
			key:    "not-a-mac",
			mutate: func(snapshot *Snapshot) {
				delete(snapshot.PnPIDByMAC, "00:11:22:33:44:55")
				snapshot.PnPIDByMAC["not-a-mac"] = "pnp-1"
			},
		},
		{
			name:   "null endpoint IP info",
			bucket: bucketEndpoints,
			key:    "container-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"] = nil
			},
		},
		{
			name:   "malformed endpoint MAC",
			bucket: bucketEndpoints,
			key:    "container-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].MACAddress = "not-a-mac"
			},
		},
		{
			name:   "non-contiguous endpoint mask",
			bucket: bucketEndpoints,
			key:    "container-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0].Mask = net.IPMask{255, 0, 255, 0}
			},
		},
		{
			name:   "IPv6 address in IPv4 prefixes",
			bucket: bucketEndpoints,
			key:    "container-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv4[0] = net.IPNet{
					IP:   net.ParseIP("fd00::99"),
					Mask: net.CIDRMask(64, 128),
				}
			},
		},
		{
			name:   "IPv4 address in IPv6 prefixes",
			bucket: bucketEndpoints,
			key:    "container-1",
			mutate: func(snapshot *Snapshot) {
				snapshot.Endpoints["container-1"].IfnameToIPMap["eth0"].IPv6[0] = net.IPNet{
					IP:   net.ParseIP("10.0.0.99"),
					Mask: net.CIDRMask(24, 32),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			snapshot := completeSnapshot()
			tt.mutate(&snapshot)
			writeSnapshot(t, db, snapshot)

			_, err := db.Snapshot(context.Background())

			require.ErrorIs(t, err, ErrCorrupt)
			assert.NotErrorIs(t, err, ErrInconsistentState)
			assert.Contains(t, err.Error(), string(tt.bucket))
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}

func TestSnapshotRejectsUnknownSchema(t *testing.T) {
	db, _ := openTestDB(t)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(metaKeySchemaVersion, encodeUint32(SchemaVersion+1))
	}))

	_, err := db.Snapshot(context.Background())

	require.ErrorIs(t, err, ErrSchemaMismatch)
	assert.NotErrorIs(t, err, ErrCorrupt)
	assert.NotErrorIs(t, err, ErrInconsistentState)
}

func TestSnapshotContextAndClosedDatabase(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		db, _ := openTestDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := db.Snapshot(ctx)

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("closed database", func(t *testing.T) {
		db, _ := openTestDB(t)
		require.NoError(t, db.Close())

		_, err := db.Snapshot(context.Background())

		require.ErrorIs(t, err, bolterrors.ErrDatabaseNotOpen)
	})
}

func TestSnapshotRejectsEveryMissingBucket(t *testing.T) {
	for _, bucketName := range allBuckets {
		t.Run(string(bucketName), func(t *testing.T) {
			db, _ := openTestDB(t)
			require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
				return tx.DeleteBucket(bucketName)
			}))

			_, err := db.Snapshot(context.Background())

			require.ErrorIs(t, err, ErrCorrupt)
			assert.Contains(t, err.Error(), string(bucketName))
		})
	}
}

func TestSnapshotDoesNotRepairPersistedData(t *testing.T) {
	db, _ := openTestDB(t)
	record := NewNetworkContainerRecord("nc-1", "2", "1", true, completeNetworkContainerRequest())
	record.Request.AuthorizationToken = "secret"
	data, err := json.Marshal(record)
	require.NoError(t, err)
	putRaw(t, db, bucketNetworkContainers, []byte("nc-1"), data)
	before := rawValue(t, db, bucketNetworkContainers, []byte("nc-1"))

	_, err = db.Snapshot(context.Background())

	require.ErrorIs(t, err, ErrInconsistentState)
	after := rawValue(t, db, bucketNetworkContainers, []byte("nc-1"))
	assert.Equal(t, before, after)
	assert.Contains(t, string(after), "secret")
}

func TestSnapshotValidationErrorsAreDeterministic(t *testing.T) {
	snapshot := completeSnapshot()
	snapshot.Networks["z"] = NetworkRecord{NetworkName: "wrong-z"}
	snapshot.Networks["a"] = NetworkRecord{NetworkName: "wrong-a"}

	var first string
	for range 20 {
		err := snapshot.validate()
		require.ErrorIs(t, err, ErrInconsistentState)
		if first == "" {
			first = err.Error()
			continue
		}
		assert.Equal(t, first, err.Error())
	}
	assert.True(t, strings.Contains(first, `"a"`))
}

func completeSnapshot() Snapshot {
	snapshot := NewSnapshot()
	snapshot.NetworkContainers["nc-1"] = NewNetworkContainerRecord(
		"nc-1",
		"2",
		"1",
		true,
		completeNetworkContainerRequest(),
	)
	snapshot.IPs = map[string]IPRecord{
		"ip-v4": {
			ID:        "ip-v4",
			IPAddress: "10.0.0.4",
			NCID:      "nc-1",
			NCVersion: 7,
		},
		"ip-v6": {
			ID:        "ip-v6",
			IPAddress: "fd00::4",
			NCID:      "nc-1",
			NCVersion: 7,
		},
		"ip-secondary": {
			ID:        "ip-secondary",
			IPAddress: "10.1.0.4",
			NCID:      "nc-1",
			NCVersion: 7,
		},
	}
	snapshot.Networks["network-1"] = NetworkRecord{
		NetworkName: "network-1",
		NicInfo: &wireserver.InterfaceInfo{
			Subnet:       "10.0.0.0/24",
			Gateway:      "10.0.0.1",
			IsPrimary:    true,
			PrimaryIP:    "10.0.0.4",
			SecondaryIPs: []string{"10.0.0.5"},
		},
		Options: map[string]any{
			"mode":    "transparent",
			"enabled": true,
		},
	}
	snapshot.OrchestratorContexts["context-1"] = []string{"nc-1"}
	snapshot.PnPIDByMAC["00:11:22:33:44:55"] = "pnp-1"
	snapshot.Assignments = map[string]AssignmentRecord{
		"iface-primary": {
			Pod: PodIdentity{
				PodKey:           "iface-primary",
				InfraContainerID: "container-1",
				InterfaceID:      "iface-primary",
				PodName:          "pod-1",
				PodNamespace:     "ns-1",
			},
			IPIDs: []string{"ip-v4", "ip-v6"},
		},
		"iface-secondary": {
			Pod: PodIdentity{
				PodKey:           "iface-secondary",
				InfraContainerID: "container-1",
				InterfaceID:      "iface-secondary",
				PodName:          "pod-1",
				PodNamespace:     "ns-1",
			},
			IPIDs: []string{"ip-secondary"},
		},
	}
	snapshot.IPOwners = map[string]string{
		"ip-v4":        "iface-primary",
		"ip-v6":        "iface-primary",
		"ip-secondary": "iface-secondary",
	}
	snapshot.Endpoints["container-1"] = completeEndpointRecord()
	snapshot.DeleteIntents["nc-old"] = DeleteIntent{
		CreatedAt: time.Date(2026, time.July, 23, 22, 0, 0, 0, time.UTC),
	}
	return snapshot
}

func writeSnapshot(t *testing.T, db *DB, snapshot Snapshot) {
	t.Helper()
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		writeJSONMap(t, tx, bucketNetworkContainers, snapshot.NetworkContainers)
		writeJSONMap(t, tx, bucketIPs, snapshot.IPs)
		writeJSONMap(t, tx, bucketNetworks, snapshot.Networks)
		writeJSONMap(t, tx, bucketOrchestratorContexts, snapshot.OrchestratorContexts)
		writeJSONMap(t, tx, bucketPnPIDByMAC, snapshot.PnPIDByMAC)
		writeJSONMap(t, tx, bucketAssignments, snapshot.Assignments)
		writeJSONMap(t, tx, bucketIPOwners, snapshot.IPOwners)
		writeJSONMap(t, tx, bucketEndpoints, snapshot.Endpoints)
		writeJSONMap(t, tx, bucketDeleteIntents, snapshot.DeleteIntents)
		return nil
	}))
}

func writeJSONMap[T any](t *testing.T, tx *bolt.Tx, bucketName []byte, values map[string]T) {
	t.Helper()
	bucket := tx.Bucket(bucketName)
	for key, value := range values {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, bucket.Put([]byte(key), data))
	}
}

func putRaw(t *testing.T, db *DB, bucketName, key, value []byte) {
	t.Helper()
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Put(key, value)
	}))
}

func rawValue(t *testing.T, db *DB, bucketName, key []byte) []byte {
	t.Helper()
	var value []byte
	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		value = append(value, tx.Bucket(bucketName).Get(key)...)
		return nil
	}))
	return value
}
