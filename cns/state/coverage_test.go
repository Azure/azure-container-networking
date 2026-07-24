// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestOwnershipNormalizationRejectsMalformedRecords(t *testing.T) {
	validPod := PodIdentity{
		PodKey:           "container",
		InfraContainerID: "container",
		PodName:          "pod",
		PodNamespace:     "namespace",
	}
	podTests := []struct {
		name   string
		mutate func(*PodIdentity)
	}{
		{name: "empty pod key", mutate: func(pod *PodIdentity) { pod.PodKey = "" }},
		{name: "empty infra container", mutate: func(pod *PodIdentity) { pod.InfraContainerID = "" }},
		{name: "pod key differs without interface", mutate: func(pod *PodIdentity) { pod.PodKey = "other" }},
		{name: "interface differs from pod key", mutate: func(pod *PodIdentity) { pod.InterfaceID = "other" }},
		{name: "empty pod name", mutate: func(pod *PodIdentity) { pod.PodName = "" }},
		{name: "empty pod namespace", mutate: func(pod *PodIdentity) { pod.PodNamespace = "" }},
	}
	for _, tt := range podTests {
		t.Run("pod/"+tt.name, func(t *testing.T) {
			pod := validPod
			tt.mutate(&pod)
			_, err := normalizePodIdentity(pod, true)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}

	t.Run("pod normalization", func(t *testing.T) {
		pod := validPod
		pod.PodKey = " container "
		pod.InfraContainerID = " container "
		pod.PodName = " pod "
		pod.PodNamespace = " namespace "
		got, err := normalizePodIdentity(pod, true)
		require.NoError(t, err)
		assert.Equal(t, validPod, got)
	})

	assignmentTests := []struct {
		name   string
		record AssignmentRecord
	}{
		{name: "no IPs", record: AssignmentRecord{Pod: validPod}},
		{name: "empty IP", record: AssignmentRecord{Pod: validPod, IPIDs: []string{""}}},
		{name: "duplicate IP", record: AssignmentRecord{Pod: validPod, IPIDs: []string{"ip", " ip "}}},
	}
	for _, tt := range assignmentTests {
		t.Run("assignment/"+tt.name, func(t *testing.T) {
			_, err := normalizeAssignment(tt.record, true)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}

	validEndpoint := EndpointRecord{
		PodName:      "pod",
		PodNamespace: "namespace",
		IfnameToIPMap: map[string]*IPInfoRecord{
			"eth0": {
				IPv4: []net.IPNet{mustIPNetValue("10.0.0.4", 24)},
			},
		},
	}
	endpointTests := []struct {
		name        string
		containerID string
		record      EndpointRecord
	}{
		{name: "empty container", record: validEndpoint},
		{
			name:        "empty pod name",
			containerID: "container",
			record: EndpointRecord{
				PodNamespace:  "namespace",
				IfnameToIPMap: validEndpoint.IfnameToIPMap,
			},
		},
		{
			name:        "empty pod namespace",
			containerID: "container",
			record: EndpointRecord{
				PodName:       "pod",
				IfnameToIPMap: validEndpoint.IfnameToIPMap,
			},
		},
		{
			name:        "no interfaces",
			containerID: "container",
			record:      EndpointRecord{PodName: "pod", PodNamespace: "namespace"},
		},
		{
			name:        "empty interface",
			containerID: "container",
			record: EndpointRecord{
				PodName:       "pod",
				PodNamespace:  "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{" ": {}},
			},
		},
		{
			name:        "duplicate normalized interface",
			containerID: "container",
			record: EndpointRecord{
				PodName:      "pod",
				PodNamespace: "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{
					"eth0":   {IPv4: []net.IPNet{mustIPNetValue("10.0.0.4", 24)}},
					" eth0 ": {IPv4: []net.IPNet{mustIPNetValue("10.0.0.5", 24)}},
				},
			},
		},
		{
			name:        "null interface",
			containerID: "container",
			record: EndpointRecord{
				PodName:       "pod",
				PodNamespace:  "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{"eth0": nil},
			},
		},
		{
			name:        "invalid MAC",
			containerID: "container",
			record: EndpointRecord{
				PodName:      "pod",
				PodNamespace: "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{
					"eth0": {IPv4: []net.IPNet{mustIPNetValue("10.0.0.4", 24)}, MACAddress: "bad"},
				},
			},
		},
		{
			name:        "no IPs",
			containerID: "container",
			record: EndpointRecord{
				PodName:       "pod",
				PodNamespace:  "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{"eth0": {}},
			},
		},
		{
			name:        "IPv6 in IPv4 list",
			containerID: "container",
			record: EndpointRecord{
				PodName:      "pod",
				PodNamespace: "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{
					"eth0": {IPv4: []net.IPNet{mustIPNetValue("fd00::4", 64)}},
				},
			},
		},
		{
			name:        "IPv4 in IPv6 list",
			containerID: "container",
			record: EndpointRecord{
				PodName:      "pod",
				PodNamespace: "namespace",
				IfnameToIPMap: map[string]*IPInfoRecord{
					"eth0": {IPv6: []net.IPNet{mustIPNetValue("10.0.0.4", 24)}},
				},
			},
		},
	}
	for _, tt := range endpointTests {
		t.Run("endpoint/"+tt.name, func(t *testing.T) {
			_, err := normalizeEndpoint(tt.containerID, tt.record)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestNormalizeDurableStateRejectsMalformedMappings(t *testing.T) {
	valid := NewDurableState()
	valid.NetworkContainers["nc"] = testNetworkContainer("nc")
	valid.IPs["ip"] = IPRecord{ID: "ip", IPAddress: "10.0.0.4", NCID: "nc"}
	valid.Networks["network"] = NetworkRecord{NetworkName: "network"}
	valid.OrchestratorContexts["context"] = []string{"nc"}
	valid.PnPIDByMAC["00:11:22:33:44:55"] = "pnp"

	tests := []struct {
		name   string
		mutate func(*DurableState)
	}{
		{
			name: "network container key mismatch",
			mutate: func(value *DurableState) {
				value.NetworkContainers = map[string]NetworkContainerRecord{"other": testNetworkContainer("nc")}
			},
		},
		{
			name: "network container request mismatch",
			mutate: func(value *DurableState) {
				record := testNetworkContainer("nc")
				record.Request.NetworkContainerid = "other"
				value.NetworkContainers = map[string]NetworkContainerRecord{"nc": record}
			},
		},
		{
			name:   "IP key mismatch",
			mutate: func(value *DurableState) { value.IPs = map[string]IPRecord{"other": valid.IPs["ip"]} },
		},
		{
			name: "network key mismatch",
			mutate: func(value *DurableState) {
				value.Networks = map[string]NetworkRecord{"other": valid.Networks["network"]}
			},
		},
		{
			name: "empty orchestrator context",
			mutate: func(value *DurableState) {
				value.OrchestratorContexts = map[string][]string{"": {"nc"}}
			},
		},
		{
			name:   "invalid MAC",
			mutate: func(value *DurableState) { value.PnPIDByMAC = map[string]string{"bad": "pnp"} },
		},
		{
			name:   "empty PnP ID",
			mutate: func(value *DurableState) { value.PnPIDByMAC = map[string]string{"00:11:22:33:44:55": ""} },
		},
		{
			name: "duplicate canonical MAC",
			mutate: func(value *DurableState) {
				value.PnPIDByMAC = map[string]string{
					"00:11:22:33:44:55": "one",
					"00-11-22-33-44-55": "two",
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := DurableState{
				NetworkContainers:    cloneMap(valid.NetworkContainers),
				IPs:                  cloneMap(valid.IPs),
				Networks:             cloneMap(valid.Networks),
				OrchestratorContexts: cloneMap(valid.OrchestratorContexts),
				PnPIDByMAC:           cloneMap(valid.PnPIDByMAC),
			}
			tt.mutate(&input)
			_, err := normalizeDurableState(input)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestLegacyNetworkValidationRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cns.CreateNetworkContainerRequest)
	}{
		{
			name: "invalid host primary IP",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.HostPrimaryIP = "bad"
			},
		},
		{
			name: "invalid local IPv6 family",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.LocalIPConfiguration.IPSubnetV6.IPAddress = "10.0.0.1"
			},
		},
		{
			name: "invalid DNS server",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.IPConfiguration.DNSServers = []string{"bad"}
			},
		},
		{
			name: "invalid gateway",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.IPConfiguration.GatewayIPAddress = "bad"
			},
		},
		{
			name: "prefix without address",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.IPv6Configuration.IPSubnet.PrefixLength = 64
			},
		},
		{
			name: "invalid subnet address",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.IPConfiguration.IPSubnet.IPAddress = "bad"
			},
		},
		{
			name: "invalid subnet prefix",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.IPConfiguration.IPSubnet.PrefixLength = 33
			},
		},
		{
			name: "invalid route destination",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.Routes = []cns.Route{{IPAddress: "bad"}}
			},
		},
		{
			name: "invalid route gateway",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.Routes = []cns.Route{{GatewayIPAddress: "bad"}}
			},
		},
		{
			name: "invalid MAC",
			mutate: func(request *cns.CreateNetworkContainerRequest) {
				request.NetworkInterfaceInfo.MACAddress = "bad"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := completeNetworkContainerRequest()
			tt.mutate(&request)
			require.Error(t, validateLegacyNetworkContainerRequest(request))
		})
	}
}

func TestTransactionInputErrorsPreserveState(t *testing.T) {
	db, _ := openTestDB(t)
	validEndpoint := testEndpoint("pod", "namespace", "10.0.0.4", "")
	validAssignment := testAssignment("container", "pod", "namespace", "ip")
	tests := []struct {
		name string
		run  func(*WriteTx) error
	}{
		{name: "network container empty ID", run: func(tx *WriteTx) error {
			return tx.PutNetworkContainer(NetworkContainerRecord{})
		}},
		{name: "network container request mismatch", run: func(tx *WriteTx) error {
			record := testNetworkContainer("nc")
			record.Request.NetworkContainerid = "other"
			return tx.PutNetworkContainer(record)
		}},
		{name: "IP empty ID", run: func(tx *WriteTx) error { return tx.PutIP(IPRecord{}) }},
		{name: "network empty name", run: func(tx *WriteTx) error { return tx.PutNetwork(NetworkRecord{}) }},
		{name: "context empty ID", run: func(tx *WriteTx) error {
			return tx.PutOrchestratorContext("", nil)
		}},
		{name: "PnP invalid MAC", run: func(tx *WriteTx) error { return tx.PutPnPID("bad", "pnp") }},
		{name: "PnP empty ID", run: func(tx *WriteTx) error {
			return tx.PutPnPID("00:11:22:33:44:55", "")
		}},
		{name: "assignment malformed", run: func(tx *WriteTx) error {
			return tx.PutAssignment(AssignmentRecord{})
		}},
		{name: "owner empty IP", run: func(tx *WriteTx) error { return tx.PutIPOwner("", "pod") }},
		{name: "owner empty pod", run: func(tx *WriteTx) error { return tx.PutIPOwner("ip", "") }},
		{name: "endpoint empty container", run: func(tx *WriteTx) error {
			return tx.PutEndpoint("", validEndpoint)
		}},
		{name: "delete intent empty container", run: func(tx *WriteTx) error {
			return tx.PutDeleteIntent("", DeleteIntent{CreatedAt: testNow})
		}},
		{name: "delete intent zero timestamp", run: func(tx *WriteTx) error {
			return tx.PutDeleteIntent("container", DeleteIntent{})
		}},
		{name: "delete empty durable ID", run: func(tx *WriteTx) error {
			return tx.DeleteNetworkContainer("")
		}},
		{name: "delete empty ownership ID", run: func(tx *WriteTx) error {
			return tx.DeleteAssignment("")
		}},
		{name: "valid assignment missing inventory", run: func(tx *WriteTx) error {
			return tx.PutAssignment(validAssignment)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := readMetadata(t, db).Generation
			err := db.Update(context.Background(), tt.run)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Equal(t, before, readMetadata(t, db).Generation)
		})
	}
}

func TestTransactionCorruptionFaultsAreCategorized(t *testing.T) {
	tests := []struct {
		name       string
		bucketName []byte
		run        func(*bolt.Tx) error
	}{
		{
			name:       "get missing bucket",
			bucketName: bucketIPs,
			run: func(tx *bolt.Tx) error {
				_, err := getJSONValue[IPRecord](
					&ReadTx{tx: tx, ctx: context.Background()},
					bucketIPs,
					"ip",
				)
				return err
			},
		},
		{
			name:       "list missing bucket",
			bucketName: bucketIPs,
			run: func(tx *bolt.Tx) error {
				_, err := listJSONValues[IPRecord](
					&ReadTx{tx: tx, ctx: context.Background()},
					bucketIPs,
				)
				return err
			},
		},
		{
			name:       "put missing bucket",
			bucketName: bucketIPs,
			run: func(tx *bolt.Tx) error {
				return putJSONValue(tx, bucketIPs, "ip", IPRecord{ID: "ip"})
			},
		},
		{
			name:       "delete missing bucket",
			bucketName: bucketAssignments,
			run: func(tx *bolt.Tx) error {
				return deleteJSONValue(tx, bucketAssignments, "pod")
			},
		},
		{
			name:       "replace missing bucket",
			bucketName: bucketNetworks,
			run: func(tx *bolt.Tx) error {
				return replaceJSONBucket(tx, bucketNetworks, map[string][]byte{})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			err := db.db.Update(func(tx *bolt.Tx) error {
				require.NoError(t, tx.DeleteBucket(tt.bucketName))
				require.ErrorIs(t, tt.run(tx), ErrCorrupt)
				return errAbort
			})
			require.ErrorIs(t, err, errAbort)
		})
	}

	t.Run("malformed stored value", func(t *testing.T) {
		db, _ := openTestDB(t)
		putRaw(t, db, bucketIPs, []byte("ip"), []byte(`{"id":`))
		require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
			_, err := tx.GetIP("ip")
			require.ErrorIs(t, err, ErrCorrupt)
			_, err = tx.ListIPs()
			require.ErrorIs(t, err, ErrCorrupt)
			return nil
		}))
	})
}

func TestValidationHelpersPreserveErrorCategories(t *testing.T) {
	require.NoError(t, validateNonemptyKeys(bucketIPs, map[string]int{"ip": 1}))
	require.ErrorIs(t, validateNonemptyKeys(bucketIPs, map[string]int{"": 1}), ErrInconsistentState)

	require.NoError(t, validateOptionalAddress(""))
	require.NoError(t, validateOptionalAddress("10.0.0.1"))
	require.Error(t, validateOptionalAddress("bad"))
	require.NoError(t, validateOptionalIPValue("10.0.0.0/24"))

	_, err := endpointsEqual(
		EndpointRecord{},
		EndpointRecord{},
	)
	require.ErrorIs(t, err, ErrInvalidInput)
	_, err = endpointsEqual(
		testEndpoint("pod", "namespace", "10.0.0.4", ""),
		EndpointRecord{},
	)
	require.ErrorIs(t, err, ErrInvalidInput)
	require.True(t, errors.Is(invalidInput("detail", errAbort), ErrInvalidInput))
	require.True(t, errors.Is(corrupt("detail", errAbort), ErrCorrupt))
}

func TestReleaseIdentityRejectsConflictingOwnership(t *testing.T) {
	snapshot := completeSnapshot()
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		pod    PodIdentity
	}{
		{
			name: "retained endpoint pod mismatch",
			pod: PodIdentity{
				PodKey:           "iface-primary",
				InfraContainerID: "container-1",
				InterfaceID:      "iface-primary",
				PodName:          "other",
				PodNamespace:     "ns-1",
			},
		},
		{
			name: "assignment infra container mismatch",
			pod: PodIdentity{
				PodKey:           "iface-primary",
				InfraContainerID: "other-container",
				InterfaceID:      "iface-primary",
				PodName:          "pod-1",
				PodNamespace:     "ns-1",
			},
		},
		{
			name: "container assignment pod mismatch",
			mutate: func(value *Snapshot) {
				delete(value.Endpoints, "container-1")
				record := value.Assignments["iface-primary"]
				record.Pod.PodName = "other"
				value.Assignments["iface-primary"] = record
			},
			pod: PodIdentity{
				PodKey:           "unknown-interface",
				InfraContainerID: "container-1",
				InterfaceID:      "unknown-interface",
				PodName:          "pod-1",
				PodNamespace:     "ns-1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneSnapshot(snapshot)
			if tt.mutate != nil {
				tt.mutate(&candidate)
			}
			require.ErrorIs(t, validateReleaseIdentity(candidate, tt.pod), ErrInvalidInput)
		})
	}
}

func TestSnapshotValidationAdditionalErrorPaths(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		want   error
	}{
		{
			name: "network container record has empty ID",
			mutate: func(snapshot *Snapshot) {
				snapshot.NetworkContainers["nc-1"] = NetworkContainerRecord{}
			},
			want: ErrInconsistentState,
		},
		{
			name:   "IP record has empty ID",
			mutate: func(snapshot *Snapshot) { snapshot.IPs["ip-v4"] = IPRecord{} },
			want:   ErrInconsistentState,
		},
		{
			name: "IP record has empty address",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.IPs["ip-v4"]
				record.IPAddress = ""
				snapshot.IPs["ip-v4"] = record
			},
			want: ErrCorrupt,
		},
		{
			name: "IP record has empty network container",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.IPs["ip-v4"]
				record.NCID = ""
				snapshot.IPs["ip-v4"] = record
			},
			want: ErrInconsistentState,
		},
		{
			name:   "network record has empty name",
			mutate: func(snapshot *Snapshot) { snapshot.Networks["network-1"] = NetworkRecord{} },
			want:   ErrInconsistentState,
		},
		{
			name: "assignment record has empty pod key",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.Pod.PodKey = ""
				snapshot.Assignments["iface-primary"] = record
			},
			want: ErrInconsistentState,
		},
		{
			name: "assignment contains empty IP",
			mutate: func(snapshot *Snapshot) {
				record := snapshot.Assignments["iface-primary"]
				record.IPIDs = []string{""}
				snapshot.Assignments["iface-primary"] = record
			},
			want: ErrInconsistentState,
		},
		{
			name:   "owner is empty",
			mutate: func(snapshot *Snapshot) { snapshot.IPOwners["ip-v4"] = "" },
			want:   ErrInconsistentState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := completeSnapshot()
			tt.mutate(&snapshot)
			require.ErrorIs(t, snapshot.Validate(), tt.want)
		})
	}

	ipNetTests := []struct {
		name         string
		value        net.IPNet
		expectedBits int
	}{
		{name: "invalid IP", value: net.IPNet{IP: net.IP{1, 2}, Mask: net.CIDRMask(24, 32)}, expectedBits: 32},
		{name: "wrong mask size", value: mustIPNetValue("10.0.0.4", 24), expectedBits: 128},
		{name: "IPv6 in IPv4", value: mustIPNetValue("fd00::4", 64), expectedBits: 32},
		{name: "IPv4 in IPv6", value: mustIPNetValue("10.0.0.4", 24), expectedBits: 128},
	}
	for _, tt := range ipNetTests {
		t.Run("IPNet/"+tt.name, func(t *testing.T) {
			_, err := validateIPNet(tt.value, tt.expectedBits)
			require.Error(t, err)
		})
	}
}

func TestDurableOperationAdditionalInputErrors(t *testing.T) {
	db, _ := openTestDB(t)
	record := testNetworkContainer("nc")
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "network container record invalid",
			run: func() error {
				_, err := db.ApplyNetworkContainer(context.Background(), NetworkContainerRecord{}, nil)
				return err
			},
		},
		{
			name: "IP has empty ID",
			run: func() error {
				_, err := db.ApplyNetworkContainer(context.Background(), record, []IPRecord{{NCID: "nc"}})
				return err
			},
		},
		{
			name: "IP references another network container",
			run: func() error {
				_, err := db.ApplyNetworkContainer(
					context.Background(),
					record,
					[]IPRecord{{ID: "ip", NCID: "other"}},
				)
				return err
			},
		},
		{
			name: "duplicate IP ID",
			run: func() error {
				_, err := db.ApplyNetworkContainer(
					context.Background(),
					record,
					[]IPRecord{
						{ID: "ip", IPAddress: "10.0.0.4", NCID: "nc"},
						{ID: "ip", IPAddress: "10.0.0.5", NCID: "nc"},
					},
				)
				return err
			},
		},
		{
			name: "delete empty network container",
			run: func() error {
				_, err := db.DeleteNetworkContainer(context.Background(), "")
				return err
			},
		},
		{
			name: "readiness empty network container",
			run: func() error {
				_, err := db.UpdateReadinessObservation(context.Background(), "", ReadinessObservation{})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, tt.run(), ErrInvalidInput)
			assert.Zero(t, readMetadata(t, db).Generation)
		})
	}
}

func TestOwnershipOperationAdditionalInputErrors(t *testing.T) {
	db, _ := openTestDB(t)
	validAssignment := testAssignment("container", "pod", "namespace", "ip")
	validEndpoint := testEndpoint("pod", "namespace", "10.0.0.4", "")
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "assignment and endpoint pod mismatch",
			run: func() error {
				endpoint := validEndpoint
				endpoint.PodName = "other"
				_, err := db.AssignEndpoint(
					context.Background(),
					validAssignment,
					endpoint,
					testNow,
					testDeleteIntentTTL,
				)
				return err
			},
		},
		{
			name: "assignment zero current time",
			run: func() error {
				_, err := db.AssignEndpoint(
					context.Background(),
					validAssignment,
					validEndpoint,
					time.Time{},
					testDeleteIntentTTL,
				)
				return err
			},
		},
		{
			name: "assignment nonpositive TTL",
			run: func() error {
				_, err := db.AssignEndpoint(
					context.Background(),
					validAssignment,
					validEndpoint,
					testNow,
					0,
				)
				return err
			},
		},
		{
			name: "release zero timestamp",
			run: func() error {
				_, err := db.ReleaseEndpoint(context.Background(), validAssignment.Pod, time.Time{})
				return err
			},
		},
		{
			name: "patch pod mismatch",
			run: func() error {
				endpoint := validEndpoint
				endpoint.PodNamespace = "other"
				_, err := db.PatchEndpoint(
					context.Background(),
					validAssignment.Pod,
					endpoint,
					testNow,
					testDeleteIntentTTL,
				)
				return err
			},
		},
		{
			name: "delete empty endpoint",
			run: func() error {
				_, err := db.DeleteEndpointRecord(context.Background(), "")
				return err
			},
		},
		{
			name: "prune zero timestamp",
			run: func() error {
				_, err := db.PruneDeleteIntents(context.Background(), time.Time{}, testDeleteIntentTTL)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, tt.run(), ErrInvalidInput)
			assert.Zero(t, readMetadata(t, db).Generation)
		})
	}
}

func TestAdditionalLegacyImportStructuralErrors(t *testing.T) {
	valid, _ := completeLegacyImportData(t)
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "orchestrator contexts null",
			mutate: func(source map[string]any) {
				source["ContainerIDByOrchestratorContext"] = nil
			},
		},
		{name: "networks null", mutate: func(source map[string]any) { source["Networks"] = nil }},
		{
			name: "PnP mappings null",
			mutate: func(source map[string]any) {
				source["PnpIDByMacAddress"] = nil
			},
		},
		{name: "zero timestamp", mutate: func(source map[string]any) { source["TimeStamp"] = "0001-01-01T00:00:00Z" }},
		{
			name: "network record null",
			mutate: func(source map[string]any) {
				source["Networks"].(map[string]any)["network-1"] = nil
			},
		},
		{
			name: "invalid PnP MAC",
			mutate: func(source map[string]any) {
				source["PnpIDByMacAddress"] = map[string]any{"bad": "pnp"}
			},
		},
		{
			name: "duplicate canonical PnP MAC",
			mutate: func(source map[string]any) {
				source["PnpIDByMacAddress"] = map[string]any{
					"00:11:22:33:44:55": "one",
					"00-11-22-33-44-55": "two",
				}
			},
		},
		{
			name: "duplicate orchestrator network container",
			mutate: func(source map[string]any) {
				source["ContainerIDByOrchestratorContext"] = map[string]any{"context": "nc,nc"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := mutateLegacyCNS(t, valid, tt.mutate)
			_, err := parseLegacyCNS(data, "boot")
			require.ErrorIs(t, err, ErrLegacyImportSource)
		})
	}
}

func TestAdditionalLegacyEndpointStructuralErrors(t *testing.T) {
	cnsData, valid := completeLegacyImportData(t)
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "endpoints null", mutate: func(envelope map[string]any) { envelope["Endpoints"] = nil }},
		{
			name: "endpoint record null",
			mutate: func(envelope map[string]any) {
				envelope["Endpoints"].(map[string]any)["container-1"] = nil
			},
		},
		{
			name: "noncanonical container ID",
			mutate: func(envelope map[string]any) {
				endpoints := envelope["Endpoints"].(map[string]any)
				endpoints[" container "] = endpoints["container-1"]
				delete(endpoints, "container-1")
			},
		},
		{
			name: "delete intents null",
			mutate: func(envelope map[string]any) {
				envelope["DeleteIntents"] = nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := parseLegacyCNS(cnsData, "boot")
			require.NoError(t, err)
			data := mutateLegacyEndpointEnvelope(t, valid, tt.mutate)
			require.ErrorIs(t, addLegacyEndpoints(&snapshot, data), ErrLegacyImportSource)
		})
	}
}

func TestStrictDecoderErrorPaths(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "null", data: []byte(" null ")},
		{name: "multiple values", data: []byte(`{} {}`)},
		{name: "malformed trailing value", data: []byte(`{} {`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var destination map[string]any
			require.Error(t, decodeStrictJSON(tt.data, &destination))
		})
	}
}

func TestMetadataMarkerHelpersRejectMissingBuckets(t *testing.T) {
	db, _ := openTestDB(t)
	err := db.db.Update(func(tx *bolt.Tx) error {
		require.NoError(t, tx.DeleteBucket(bucketMetadata))
		_, importErr := legacyImportComplete(tx.Bucket(bucketMetadata))
		require.ErrorIs(t, importErr, ErrCorrupt)
		_, exportErr := rollbackExportComplete(tx.Bucket(bucketMetadata))
		require.ErrorIs(t, exportErr, ErrCorrupt)
		require.ErrorIs(t, setRollbackExportComplete(tx), ErrCorrupt)
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
}

func TestEncodingRejectsUnsupportedNetworkOptions(t *testing.T) {
	durable := NewDurableState()
	durable.Networks["network"] = NetworkRecord{
		NetworkName: "network",
		Options:     map[string]any{"unsupported": make(chan struct{})},
	}
	_, err := encodeDurableState(durable)
	require.ErrorIs(t, err, ErrInvalidInput)

	db, _ := openTestDB(t)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		err := putJSONValue(tx, bucketNetworks, "network", durable.Networks["network"])
		require.ErrorIs(t, err, ErrInvalidInput)
		return nil
	}))
}
