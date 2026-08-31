// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
)

var ErrInconsistentState = errors.New("cns state: inconsistent state")

var (
	errJSONValueNull     = errors.New("json value is null")
	errJSONValueMultiple = errors.New("multiple JSON values")
	errInvalidIPPrefix   = errors.New("invalid IP prefix")
)

type Snapshot struct {
	Metadata             Metadata
	NetworkContainers    map[string]NetworkContainerRecord
	IPs                  map[string]IPRecord
	Networks             map[string]NetworkRecord
	OrchestratorContexts map[string][]string
	PnPIDByMAC           map[string]string
	Assignments          map[string]AssignmentRecord
	IPOwners             map[string]string
	Endpoints            map[string]EndpointRecord
	DeleteIntents        map[string]DeleteIntent
}

func NewSnapshot() Snapshot {
	return Snapshot{
		NetworkContainers:    map[string]NetworkContainerRecord{},
		IPs:                  map[string]IPRecord{},
		Networks:             map[string]NetworkRecord{},
		OrchestratorContexts: map[string][]string{},
		PnPIDByMAC:           map[string]string{},
		Assignments:          map[string]AssignmentRecord{},
		IPOwners:             map[string]string{},
		Endpoints:            map[string]EndpointRecord{},
		DeleteIntents:        map[string]DeleteIntent{},
	}
}

func (s *DB) Snapshot(ctx context.Context) (Snapshot, error) {
	snapshot := NewSnapshot()
	err := s.View(ctx, func(tx *ReadTx) error {
		metadata, err := tx.Metadata()
		if err != nil {
			return err
		}
		if metadata.SchemaVersion != SchemaVersion {
			return fmt.Errorf(
				"%w: database=%d code=%d",
				ErrSchemaMismatch,
				metadata.SchemaVersion,
				SchemaVersion,
			)
		}
		snapshot.Metadata = metadata

		if err := decodeBucket(ctx, tx, bucketNetworkContainers, snapshot.NetworkContainers); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketIPs, snapshot.IPs); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketNetworks, snapshot.Networks); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketOrchestratorContexts, snapshot.OrchestratorContexts); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketPnPIDByMAC, snapshot.PnPIDByMAC); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketAssignments, snapshot.Assignments); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketIPOwners, snapshot.IPOwners); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketEndpoints, snapshot.Endpoints); err != nil {
			return err
		}
		if err := decodeBucket(ctx, tx, bucketDeleteIntents, snapshot.DeleteIntents); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("taking cns state snapshot: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("taking cns state snapshot: %w", err)
	}
	if err := snapshot.validate(); err != nil {
		return Snapshot{}, fmt.Errorf("taking cns state snapshot: %w", err)
	}
	return snapshot, nil
}

func decodeBucket[T any](ctx context.Context, tx *ReadTx, name []byte, destination map[string]T) error {
	bucket := tx.tx.Bucket(name)
	if bucket == nil {
		return corrupt(fmt.Sprintf("missing bucket %q", name), nil)
	}
	if err := bucket.ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("decoding bucket %q: %w", name, err)
		}
		if value == nil {
			return corrupt(fmt.Sprintf("bucket %q key %q is not a value", name, key), nil)
		}
		var record T
		if err := decodeJSONValue(value, &record); err != nil {
			return corrupt(fmt.Sprintf("decoding bucket %q key %q", name, key), err)
		}
		destination[string(key)] = record
		return nil
	}); err != nil {
		return fmt.Errorf("iterating bucket %q: %w", name, err)
	}
	return nil
}

func decodeJSONValue(data []byte, destination any) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errJSONValueNull
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decoding json value: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errJSONValueMultiple
		}
		return fmt.Errorf("decoding json value: %w", err)
	}
	return nil
}

func (s Snapshot) validate() error {
	if err := s.validateNetworkContainers(); err != nil {
		return err
	}
	ipAddresses, ipsErr := s.validateIPs()
	if ipsErr != nil {
		return ipsErr
	}
	if err := s.validateNetworks(); err != nil {
		return err
	}
	if err := s.validateOrchestratorContexts(); err != nil {
		return err
	}
	if err := s.validatePnPIDs(); err != nil {
		return err
	}
	if err := s.validateAssignments(); err != nil {
		return err
	}
	if err := s.validateOwners(); err != nil {
		return err
	}
	endpointIPs, endpointsErr := s.validateEndpoints()
	if endpointsErr != nil {
		return endpointsErr
	}
	if err := s.validateAssignmentEndpoints(ipAddresses, endpointIPs); err != nil {
		return err
	}
	return validateNonemptyKeys(bucketDeleteIntents, s.DeleteIntents)
}

func (s Snapshot) validateNetworkContainers() error {
	for _, key := range sortedKeys(s.NetworkContainers) {
		record := s.NetworkContainers[key]
		switch {
		case key == "":
			return inconsistent("network container key is empty")
		case record.ID == "":
			return inconsistent("network container %q has empty ID", key)
		case record.ID != key:
			return inconsistent("network container key %q does not match record ID %q", key, record.ID)
		case record.Request.NetworkContainerid != key:
			return inconsistent(
				"network container key %q does not match request ID %q",
				key,
				record.Request.NetworkContainerid,
			)
		case record.Request.AuthorizationToken != "":
			return inconsistent("network container %q contains an authorization token", key)
		case record.Request.SecondaryIPConfigs != nil:
			return inconsistent("network container %q contains embedded secondary IP configs", key)
		}
	}
	return nil
}

func (s Snapshot) validateIPs() (map[string]netip.Addr, error) {
	addresses := make(map[string]netip.Addr, len(s.IPs))
	owners := make(map[netip.Addr]string, len(s.IPs))
	for _, key := range sortedKeys(s.IPs) {
		record := s.IPs[key]
		switch {
		case key == "":
			return nil, inconsistent("IP key is empty")
		case record.ID == "":
			return nil, inconsistent("IP %q has empty ID", key)
		case record.ID != key:
			return nil, inconsistent("IP key %q does not match record ID %q", key, record.ID)
		case record.IPAddress == "":
			return nil, corruptValue(bucketIPs, key, "empty address", nil)
		case record.NCID == "":
			return nil, inconsistent("IP %q has empty NC ID", key)
		}
		address, err := netip.ParseAddr(record.IPAddress)
		if err != nil {
			return nil, corruptValue(bucketIPs, key, fmt.Sprintf("invalid address %q", record.IPAddress), err)
		}
		address = address.Unmap()
		if other, ok := owners[address]; ok {
			return nil, inconsistent("IPs %q and %q have duplicate address %q", other, key, address)
		}
		if _, ok := s.NetworkContainers[record.NCID]; !ok {
			return nil, inconsistent("IP %q references missing network container %q", key, record.NCID)
		}
		addresses[key] = address
		owners[address] = key
	}
	return addresses, nil
}

func (s Snapshot) validateNetworks() error {
	for _, key := range sortedKeys(s.Networks) {
		record := s.Networks[key]
		switch {
		case key == "":
			return inconsistent("network key is empty")
		case record.NetworkName == "":
			return inconsistent("network %q has empty name", key)
		case record.NetworkName != key:
			return inconsistent("network key %q does not match record name %q", key, record.NetworkName)
		}
		if record.NicInfo == nil {
			continue
		}
		if record.NicInfo.Subnet != "" {
			if _, err := netip.ParsePrefix(record.NicInfo.Subnet); err != nil {
				return corruptValue(
					bucketNetworks,
					key,
					fmt.Sprintf("invalid subnet %q", record.NicInfo.Subnet),
					err,
				)
			}
		}
		addresses := append(
			[]string{record.NicInfo.Gateway, record.NicInfo.PrimaryIP},
			record.NicInfo.SecondaryIPs...,
		)
		for _, address := range addresses {
			if address == "" {
				continue
			}
			if _, err := netip.ParseAddr(address); err != nil {
				return corruptValue(bucketNetworks, key, fmt.Sprintf("invalid address %q", address), err)
			}
		}
	}
	return nil
}

func (s Snapshot) validateOrchestratorContexts() error {
	for _, key := range sortedKeys(s.OrchestratorContexts) {
		if key == "" {
			return inconsistent("orchestrator context key is empty")
		}
		seen := make(map[string]struct{}, len(s.OrchestratorContexts[key]))
		for _, ncID := range s.OrchestratorContexts[key] {
			if _, ok := seen[ncID]; ok {
				return inconsistent("orchestrator context %q contains duplicate network container %q", key, ncID)
			}
			if _, ok := s.NetworkContainers[ncID]; !ok {
				return inconsistent("orchestrator context %q references missing network container %q", key, ncID)
			}
			seen[ncID] = struct{}{}
		}
	}
	return nil
}

func (s Snapshot) validatePnPIDs() error {
	canonical := make(map[string]string, len(s.PnPIDByMAC))
	for _, key := range sortedKeys(s.PnPIDByMAC) {
		if key == "" {
			return inconsistent("PnP MAC key is empty")
		}
		mac, err := net.ParseMAC(key)
		if err != nil {
			return corruptValue(bucketPnPIDByMAC, key, "invalid MAC address", err)
		}
		normalized := mac.String()
		if other, ok := canonical[normalized]; ok {
			return inconsistent("PnP MAC keys %q and %q are aliases", other, key)
		}
		if s.PnPIDByMAC[key] == "" {
			return inconsistent("PnP MAC key %q has empty ID", key)
		}
		canonical[normalized] = key
	}
	return nil
}

func (s Snapshot) validateAssignments() error {
	assigned := make(map[string]string, len(s.IPs))
	for _, key := range sortedKeys(s.Assignments) {
		record := s.Assignments[key]
		switch {
		case key == "":
			return inconsistent("assignment key is empty")
		case record.Pod.PodKey == "":
			return inconsistent("assignment %q has empty pod key", key)
		case record.Pod.PodKey != key:
			return inconsistent("assignment key %q does not match pod key %q", key, record.Pod.PodKey)
		case record.Pod.InfraContainerID == "":
			return inconsistent("assignment %q has empty infra container ID", key)
		case record.Pod.InterfaceID == "" && record.Pod.PodKey != record.Pod.InfraContainerID:
			return inconsistent("assignment %q without interface ID must use its infra container ID as pod key", key)
		case record.Pod.InterfaceID != "" && record.Pod.PodKey != record.Pod.InterfaceID:
			return inconsistent("assignment %q interface ID %q does not match pod key", key, record.Pod.InterfaceID)
		case len(record.IPIDs) == 0:
			return inconsistent("assignment %q has no IPs", key)
		}

		withinAssignment := make(map[string]struct{}, len(record.IPIDs))
		for _, ipID := range record.IPIDs {
			if ipID == "" {
				return inconsistent("assignment %q contains an empty IP ID", key)
			}
			if _, ok := withinAssignment[ipID]; ok {
				return inconsistent("assignment %q contains duplicate IP %q", key, ipID)
			}
			if _, ok := s.IPs[ipID]; !ok {
				return inconsistent("assignment %q references missing IP %q", key, ipID)
			}
			if other, ok := assigned[ipID]; ok {
				return inconsistent("assignments %q and %q both contain IP %q", other, key, ipID)
			}
			owner, ok := s.IPOwners[ipID]
			if !ok {
				return inconsistent("assignment %q IP %q has no owner", key, ipID)
			}
			if owner != key {
				return inconsistent("assignment %q IP %q is owned by %q", key, ipID, owner)
			}
			withinAssignment[ipID] = struct{}{}
			assigned[ipID] = key
		}
	}
	return nil
}

func (s Snapshot) validateOwners() error {
	for _, ipID := range sortedKeys(s.IPOwners) {
		owner := s.IPOwners[ipID]
		switch {
		case ipID == "":
			return inconsistent("IP owner key is empty")
		case owner == "":
			return inconsistent("IP %q has empty owner", ipID)
		}
		if _, ok := s.IPs[ipID]; !ok {
			return inconsistent("owner %q references missing IP %q", owner, ipID)
		}
		assignment, ok := s.Assignments[owner]
		if !ok {
			return inconsistent("IP %q references missing assignment %q", ipID, owner)
		}
		if !contains(assignment.IPIDs, ipID) {
			return inconsistent("IP %q owner %q does not contain the IP", ipID, owner)
		}
	}
	return nil
}

type endpointIPLocation struct {
	endpointKey string
	ncID        string
}

func (s Snapshot) validateEndpoints() (map[netip.Addr]endpointIPLocation, error) {
	addresses := map[netip.Addr]endpointIPLocation{}
	for _, endpointKey := range sortedKeys(s.Endpoints) {
		if endpointKey == "" {
			return nil, inconsistent("endpoint key is empty")
		}
		record := s.Endpoints[endpointKey]
		for _, ifname := range sortedKeys(record.IfnameToIPMap) {
			if ifname == "" {
				return nil, inconsistent("endpoint %q has an empty interface name", endpointKey)
			}
			info := record.IfnameToIPMap[ifname]
			if info == nil {
				return nil, corruptValue(
					bucketEndpoints,
					endpointKey,
					fmt.Sprintf("interface %q is null", ifname),
					nil,
				)
			}
			if info.MACAddress != "" {
				if _, err := net.ParseMAC(info.MACAddress); err != nil {
					return nil, corruptValue(
						bucketEndpoints,
						endpointKey,
						fmt.Sprintf("interface %q has invalid MAC %q", ifname, info.MACAddress),
						err,
					)
				}
			}
			if info.NetworkContainerID != "" {
				if _, ok := s.NetworkContainers[info.NetworkContainerID]; !ok {
					return nil, inconsistent(
						"endpoint %q interface %q references missing network container %q",
						endpointKey,
						ifname,
						info.NetworkContainerID,
					)
				}
			}
			for _, family := range []struct {
				name     string
				bits     int
				prefixes []net.IPNet
			}{
				{name: "IPv4", bits: 32, prefixes: info.IPv4},
				{name: "IPv6", bits: 128, prefixes: info.IPv6},
			} {
				for index, prefix := range family.prefixes {
					address, err := validateIPNet(prefix, family.bits)
					if err != nil {
						return nil, corruptValue(
							bucketEndpoints,
							endpointKey,
							fmt.Sprintf(
								"interface %q %s prefix %d",
								ifname,
								family.name,
								index,
							),
							err,
						)
					}
					if other, ok := addresses[address]; ok {
						return nil, inconsistent(
							"endpoint %q and %q contain duplicate IP %q",
							other.endpointKey,
							endpointKey,
							address,
						)
					}
					addresses[address] = endpointIPLocation{
						endpointKey: endpointKey,
						ncID:        info.NetworkContainerID,
					}
				}
			}
		}
	}
	return addresses, nil
}

func validateIPNet(value net.IPNet, expectedBits int) (netip.Addr, error) {
	address, ok := netip.AddrFromSlice(value.IP)
	if !ok {
		return netip.Addr{}, fmt.Errorf("%w: invalid IP %q", errInvalidIPPrefix, value.IP)
	}
	address = address.Unmap()
	ones, bits := value.Mask.Size()
	if bits != expectedBits {
		return netip.Addr{}, fmt.Errorf("%w: mask has %d bits, expected %d", errInvalidIPPrefix, bits, expectedBits)
	}
	if expectedBits == 32 && !address.Is4() {
		return netip.Addr{}, fmt.Errorf("%w: IPv6 address %q in IPv4 prefixes", errInvalidIPPrefix, address)
	}
	if expectedBits == 128 && !address.Is6() {
		return netip.Addr{}, fmt.Errorf("%w: IPv4 address %q in IPv6 prefixes", errInvalidIPPrefix, address)
	}
	if !netip.PrefixFrom(address, ones).IsValid() {
		return netip.Addr{}, fmt.Errorf("%w: invalid prefix length %d for %q", errInvalidIPPrefix, ones, address)
	}
	return address, nil
}

func (s Snapshot) validateAssignmentEndpoints(
	ipAddresses map[string]netip.Addr,
	endpointIPs map[netip.Addr]endpointIPLocation,
) error {
	for _, ipID := range sortedKeys(ipAddresses) {
		location, ok := endpointIPs[ipAddresses[ipID]]
		if ok && location.ncID != "" && location.ncID != s.IPs[ipID].NCID {
			return inconsistent(
				"endpoint %q IP %q NC %q does not match IP record NC %q",
				location.endpointKey,
				ipID,
				location.ncID,
				s.IPs[ipID].NCID,
			)
		}
	}
	for _, assignmentKey := range sortedKeys(s.Assignments) {
		assignment := s.Assignments[assignmentKey]
		endpoint, ok := s.Endpoints[assignment.Pod.InfraContainerID]
		if !ok {
			return inconsistent(
				"assignment %q references missing endpoint %q",
				assignmentKey,
				assignment.Pod.InfraContainerID,
			)
		}
		if endpoint.PodName != assignment.Pod.PodName || endpoint.PodNamespace != assignment.Pod.PodNamespace {
			return inconsistent("assignment %q pod identity does not match endpoint", assignmentKey)
		}
		for _, ipID := range assignment.IPIDs {
			address := ipAddresses[ipID]
			location, ok := endpointIPs[address]
			if !ok || location.endpointKey != assignment.Pod.InfraContainerID {
				return inconsistent("assignment %q IP %q is missing from its endpoint", assignmentKey, ipID)
			}
		}
	}
	return nil
}

func validateNonemptyKeys[T any](bucket []byte, values map[string]T) error {
	for _, key := range sortedKeys(values) {
		if key == "" {
			return inconsistent("bucket %q contains an empty key", bucket)
		}
	}
	return nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func inconsistent(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInconsistentState, fmt.Sprintf(format, args...))
}

func corruptValue(bucket []byte, key, detail string, err error) error {
	return corrupt(fmt.Sprintf("bucket %q key %q: %s", bucket, key, detail), err)
}
