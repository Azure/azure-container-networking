// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
)

var ErrCNIImportConflict = errors.New("cns state: CNI import conflicts with session state")

type CNIImportCounts struct {
	Containers  uint32
	Interfaces  uint32
	Assignments uint32
	IPs         uint32
}

// CNIImportPreflight is an immutable plan for importing live stateful CNI
// ownership. It contains no source path or raw CNI response.
type CNIImportPreflight struct {
	ExpectedGeneration uint64
	Counts             CNIImportCounts
	IdentityDigest     string

	allowReplacement bool
	identities       []string
	endpoints        map[string]EndpointRecord
	assignments      map[string]AssignmentRecord
	owners           map[string]string
	seal             [sha256.Size]byte
}

type cniImportSession struct {
	Endpoints   map[string]EndpointRecord   `json:"endpoints"`
	Assignments map[string]AssignmentRecord `json:"assignments"`
	Owners      map[string]string           `json:"owners"`
}

type normalizedCNIEndpoint struct {
	InfraContainerID string         `json:"infraContainerID"`
	PodEndpointID    string         `json:"podEndpointID"`
	PodName          string         `json:"podName"`
	PodNamespace     string         `json:"podNamespace"`
	InterfaceKey     string         `json:"interfaceKey"`
	InterfaceName    string         `json:"interfaceName"`
	Prefixes         []netip.Prefix `json:"prefixes"`
}

func (s *DB) PreflightCNIEndpointImport(
	ctx context.Context,
	records []cns.CNIEndpointState,
	allowSessionReplacement bool,
) (CNIImportPreflight, error) {
	if ctx == nil {
		return CNIImportPreflight{}, errors.New("preflighting CNI endpoint import: context is nil")
	}
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return CNIImportPreflight{}, fmt.Errorf("preflighting CNI endpoint import: %w", err)
	}
	plan, _, err := buildCNIImportPreflight(ctx, snapshot, records, allowSessionReplacement)
	if err != nil {
		return CNIImportPreflight{}, fmt.Errorf("preflighting CNI endpoint import: %w", err)
	}
	return plan, nil
}

func (s *DB) ImportCNIEndpointState(
	ctx context.Context,
	records []cns.CNIEndpointState,
	plan CNIImportPreflight,
) (bool, error) {
	if ctx == nil {
		return false, errors.New("importing CNI endpoint state: context is nil")
	}
	if err := validateCNIImportPlan(plan); err != nil {
		return false, fmt.Errorf("importing CNI endpoint state: %w", err)
	}
	changed, err := s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if current.Metadata.Generation != plan.ExpectedGeneration {
			return false, fmt.Errorf(
				"%w: expected=%d actual=%d",
				ErrStaleGeneration,
				plan.ExpectedGeneration,
				current.Metadata.Generation,
			)
		}
		recomputed, candidate, err := buildCNIImportPreflight(
			ctx,
			current,
			records,
			plan.allowReplacement,
		)
		if err != nil {
			return false, err
		}
		if !sameCNIImportPlan(plan, recomputed) {
			return false, invalidInput("CNI import preflight identity changed", nil)
		}
		if sessionEqual(current, candidate) {
			return false, nil
		}

		encoded, err := encodeCNIImportSession(candidate)
		if err != nil {
			return false, err
		}
		for _, replacement := range encoded {
			if err := replaceJSONBucket(tx.tx, replacement.name, replacement.values); err != nil {
				return false, err
			}
		}
		if s.cniImportBeforeCommit != nil {
			if err := s.cniImportBeforeCommit(); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("importing CNI endpoint state: %w", err)
	}
	return changed, nil
}

// SnapshotForProjection applies this preflight's session candidate to a fresh
// copy of base. The plan seal and generation are checked before any data is
// returned.
func (p CNIImportPreflight) SnapshotForProjection(base Snapshot) (Snapshot, error) {
	if err := validateCNIImportPlan(p); err != nil {
		return Snapshot{}, err
	}
	if base.Metadata.Generation != p.ExpectedGeneration {
		return Snapshot{}, fmt.Errorf(
			"%w: expected=%d actual=%d",
			ErrStaleGeneration,
			p.ExpectedGeneration,
			base.Metadata.Generation,
		)
	}
	candidate := cloneSnapshot(base)
	candidate.Endpoints = cloneCNIEndpoints(p.endpoints)
	candidate.Assignments = cloneCNIAssignments(p.assignments)
	candidate.IPOwners = cloneMap(p.owners)
	if err := candidate.Validate(); err != nil {
		return Snapshot{}, err
	}
	return candidate, nil
}

func buildCNIImportPreflight(
	ctx context.Context,
	snapshot Snapshot,
	records []cns.CNIEndpointState,
	allowReplacement bool,
) (CNIImportPreflight, Snapshot, error) {
	if err := snapshot.Validate(); err != nil {
		return CNIImportPreflight{}, Snapshot{}, err
	}
	if len(snapshot.DeleteIntents) != 0 {
		return CNIImportPreflight{}, Snapshot{}, fmt.Errorf("%w: endpoint import is blocked", ErrDeleteIntent)
	}
	normalized, err := normalizeCNIEndpointRecords(ctx, records)
	if err != nil {
		return CNIImportPreflight{}, Snapshot{}, err
	}
	inventory, err := cniImportInventory(snapshot)
	if err != nil {
		return CNIImportPreflight{}, Snapshot{}, err
	}
	candidate := cloneSnapshot(snapshot)
	candidate.Endpoints = make(map[string]EndpointRecord)
	candidate.Assignments = make(map[string]AssignmentRecord)
	candidate.IPOwners = make(map[string]string)
	identities := make([]string, 0, len(normalized))
	pods := make(map[string]string)

	for _, record := range normalized {
		if err := ctx.Err(); err != nil {
			return CNIImportPreflight{}, Snapshot{}, err
		}
		podIdentity := record.PodNamespace + "\x00" + record.PodName
		if other, ok := pods[podIdentity]; ok && other != record.InfraContainerID {
			return CNIImportPreflight{}, Snapshot{}, invalidInput(
				fmt.Sprintf("pod is associated with containers %q and %q", other, record.InfraContainerID),
				nil,
			)
		}
		pods[podIdentity] = record.InfraContainerID

		endpoint, ok := candidate.Endpoints[record.InfraContainerID]
		if ok && (endpoint.PodName != record.PodName || endpoint.PodNamespace != record.PodNamespace) {
			return CNIImportPreflight{}, Snapshot{}, invalidInput(
				fmt.Sprintf("infra container %q has conflicting pod identity", record.InfraContainerID),
				nil,
			)
		}
		if !ok {
			endpoint = EndpointRecord{
				PodName:       record.PodName,
				PodNamespace:  record.PodNamespace,
				IfnameToIPMap: make(map[string]*IPInfoRecord),
			}
		}
		if _, ok := endpoint.IfnameToIPMap[record.InterfaceName]; ok {
			return CNIImportPreflight{}, Snapshot{}, invalidInput(
				fmt.Sprintf(
					"infra container %q has duplicate interface %q",
					record.InfraContainerID,
					record.InterfaceName,
				),
				nil,
			)
		}
		info := IPInfoRecord{
			IPv4: make([]net.IPNet, 0),
			IPv6: make([]net.IPNet, 0),
		}
		ipIDs := make([]string, 0, len(record.Prefixes))
		for _, prefix := range record.Prefixes {
			ip, ok := inventory[prefix.Addr()]
			if !ok {
				return CNIImportPreflight{}, Snapshot{}, invalidInput("CNI address is missing from IP inventory", nil)
			}
			if info.NetworkContainerID == "" {
				info.NetworkContainerID = ip.NCID
			} else if info.NetworkContainerID != ip.NCID {
				return CNIImportPreflight{}, Snapshot{}, invalidInput(
					fmt.Sprintf("interface %q spans network containers", record.InterfaceName),
					nil,
				)
			}
			ipIDs = append(ipIDs, ip.ID)
			value := net.IPNet{
				IP:   net.IP(prefix.Addr().AsSlice()),
				Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
			}
			if prefix.Addr().Is4() {
				info.IPv4 = append(info.IPv4, value)
			} else {
				info.IPv6 = append(info.IPv6, value)
			}
		}
		sort.Strings(ipIDs)
		endpoint.IfnameToIPMap[record.InterfaceName] = &info
		candidate.Endpoints[record.InfraContainerID] = endpoint

		if _, ok := candidate.Assignments[record.PodEndpointID]; ok {
			return CNIImportPreflight{}, Snapshot{}, invalidInput(
				fmt.Sprintf("duplicate assignment identity %q", record.PodEndpointID),
				nil,
			)
		}
		assignment := AssignmentRecord{
			Pod: PodIdentity{
				PodKey:           record.PodEndpointID,
				InfraContainerID: record.InfraContainerID,
				InterfaceID:      record.PodEndpointID,
				PodName:          record.PodName,
				PodNamespace:     record.PodNamespace,
			},
			IPIDs: ipIDs,
		}
		candidate.Assignments[record.PodEndpointID] = assignment
		for _, ipID := range ipIDs {
			if other, ok := candidate.IPOwners[ipID]; ok {
				return CNIImportPreflight{}, Snapshot{}, invalidInput(
					fmt.Sprintf("IP inventory ID is assigned to %q and %q", other, record.PodEndpointID),
					nil,
				)
			}
			candidate.IPOwners[ipID] = record.PodEndpointID
		}
		identity, err := json.Marshal(record)
		if err != nil {
			return CNIImportPreflight{}, Snapshot{}, invalidInput("encoding CNI identity", err)
		}
		identities = append(identities, string(identity))
	}
	if err := candidate.Validate(); err != nil {
		return CNIImportPreflight{}, Snapshot{}, fmt.Errorf("%w: candidate CNI import: %v", ErrInvalidInput, err)
	}
	if !allowReplacement && !sessionEmpty(snapshot) && !sessionEqual(snapshot, candidate) {
		return CNIImportPreflight{}, Snapshot{}, ErrCNIImportConflict
	}
	counts, err := cniImportCounts(candidate, len(normalized))
	if err != nil {
		return CNIImportPreflight{}, Snapshot{}, err
	}
	digest, err := cniImportDigest(normalized, candidate)
	if err != nil {
		return CNIImportPreflight{}, Snapshot{}, err
	}
	plan := CNIImportPreflight{
		ExpectedGeneration: snapshot.Metadata.Generation,
		Counts:             counts,
		IdentityDigest:     digest,
		allowReplacement:   allowReplacement,
		identities:         append([]string{}, identities...),
		endpoints:          cloneCNIEndpoints(candidate.Endpoints),
		assignments:        cloneCNIAssignments(candidate.Assignments),
		owners:             cloneMap(candidate.IPOwners),
	}
	plan.seal = sealCNIImportPlan(plan)
	return plan, candidate, nil
}

func normalizeCNIEndpointRecords(
	ctx context.Context,
	records []cns.CNIEndpointState,
) ([]normalizedCNIEndpoint, error) {
	normalized := make([]normalizedCNIEndpoint, 0, len(records))
	seenAddresses := make(map[netip.Addr]string)
	seenEndpointIDs := make(map[string]string)
	seenKeys := make(map[string]string)
	seenInterfaces := make(map[string]string)
	for index, input := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record := normalizedCNIEndpoint{
			InfraContainerID: strings.TrimSpace(input.InfraContainerID),
			PodEndpointID:    strings.TrimSpace(input.PodEndpointID),
			PodName:          strings.TrimSpace(input.PodName),
			PodNamespace:     strings.TrimSpace(input.PodNamespace),
			InterfaceKey:     strings.TrimSpace(input.InterfaceKey),
			InterfaceName:    strings.TrimSpace(input.InterfaceName),
			Prefixes:         make([]netip.Prefix, 0, len(input.IPAddresses)),
		}
		switch {
		case record.InfraContainerID == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty infra container ID", index), nil)
		case record.PodEndpointID == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty pod endpoint ID", index), nil)
		case record.PodName == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty pod name", index), nil)
		case record.PodNamespace == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty pod namespace", index), nil)
		case record.InterfaceKey == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty interface key", index), nil)
		case record.InterfaceName == "":
			return nil, invalidInput(fmt.Sprintf("CNI record %d has empty interface name", index), nil)
		case len(input.IPAddresses) == 0:
			return nil, invalidInput(fmt.Sprintf("CNI record %d has no IPs", index), nil)
		}
		if other, ok := seenEndpointIDs[record.PodEndpointID]; ok {
			return nil, invalidInput(
				fmt.Sprintf("CNI records %q and %q have duplicate pod endpoint identity", other, record.InterfaceKey),
				nil,
			)
		}
		if other, ok := seenKeys[record.InterfaceKey]; ok {
			return nil, invalidInput(
				fmt.Sprintf("CNI records %q and %q have duplicate interface key", other, record.InterfaceKey),
				nil,
			)
		}
		interfaceIdentity := record.InfraContainerID + "\x00" + record.InterfaceName
		if other, ok := seenInterfaces[interfaceIdentity]; ok {
			return nil, invalidInput(
				fmt.Sprintf("CNI records %q and %q have duplicate interface identity", other, record.InterfaceKey),
				nil,
			)
		}
		for ipIndex, value := range input.IPAddresses {
			address, prefix, err := canonicalCNIIPNet(value)
			if err != nil {
				return nil, invalidInput(
					fmt.Sprintf("CNI record %q IP %d", record.InterfaceKey, ipIndex),
					err,
				)
			}
			if other, ok := seenAddresses[address]; ok {
				return nil, invalidInput(
					fmt.Sprintf("CNI records %q and %q contain duplicate address", other, record.InterfaceKey),
					nil,
				)
			}
			seenAddresses[address] = record.InterfaceKey
			record.Prefixes = append(record.Prefixes, prefix)
		}
		sort.Slice(record.Prefixes, func(i, j int) bool {
			return record.Prefixes[i].Addr().Compare(record.Prefixes[j].Addr()) < 0
		})
		seenEndpointIDs[record.PodEndpointID] = record.InterfaceKey
		seenKeys[record.InterfaceKey] = record.InterfaceKey
		seenInterfaces[interfaceIdentity] = record.InterfaceKey
		normalized = append(normalized, record)
	}
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.InfraContainerID != right.InfraContainerID {
			return left.InfraContainerID < right.InfraContainerID
		}
		if left.PodEndpointID != right.PodEndpointID {
			return left.PodEndpointID < right.PodEndpointID
		}
		return left.InterfaceKey < right.InterfaceKey
	})
	return normalized, nil
}

func canonicalCNIIPNet(value net.IPNet) (netip.Addr, netip.Prefix, error) {
	address, ok := netip.AddrFromSlice(value.IP)
	if !ok {
		return netip.Addr{}, netip.Prefix{}, errors.New("invalid IP address")
	}
	address = address.Unmap()
	ones, bits := value.Mask.Size()
	if bits != address.BitLen() || ones < 0 {
		return netip.Addr{}, netip.Prefix{}, errors.New("invalid IP mask")
	}
	prefix := netip.PrefixFrom(address, ones)
	if !prefix.IsValid() {
		return netip.Addr{}, netip.Prefix{}, errors.New("invalid IP prefix")
	}
	return address, prefix, nil
}

func cniImportInventory(snapshot Snapshot) (map[netip.Addr]IPRecord, error) {
	inventory := make(map[netip.Addr]IPRecord, len(snapshot.IPs))
	for _, id := range sortedKeys(snapshot.IPs) {
		ip := snapshot.IPs[id]
		address, err := netip.ParseAddr(ip.IPAddress)
		if err != nil {
			return nil, corruptValue(bucketIPs, id, "invalid address", err)
		}
		address = address.Unmap()
		if other, ok := inventory[address]; ok {
			return nil, invalidInput(
				fmt.Sprintf("IP inventory IDs %q and %q contain duplicate address", other.ID, id),
				nil,
			)
		}
		inventory[address] = ip
	}
	return inventory, nil
}

func cniImportCounts(candidate Snapshot, interfaceCount int) (CNIImportCounts, error) {
	values := []int{
		len(candidate.Endpoints),
		interfaceCount,
		len(candidate.Assignments),
		len(candidate.IPOwners),
	}
	for _, value := range values {
		if uint64(value) > math.MaxUint32 {
			return CNIImportCounts{}, invalidInput("CNI import count exceeds uint32", nil)
		}
	}
	return CNIImportCounts{
		Containers:  uint32(values[0]),
		Interfaces:  uint32(values[1]),
		Assignments: uint32(values[2]),
		IPs:         uint32(values[3]),
	}, nil
}

func cniImportDigest(records []normalizedCNIEndpoint, candidate Snapshot) (string, error) {
	data, err := json.Marshal(struct {
		Records []normalizedCNIEndpoint `json:"records"`
		Session cniImportSession        `json:"session"`
	}{
		Records: records,
		Session: cniImportSession{
			Endpoints:   candidate.Endpoints,
			Assignments: candidate.Assignments,
			Owners:      candidate.IPOwners,
		},
	})
	if err != nil {
		return "", invalidInput("encoding CNI import identity", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sealCNIImportPlan(plan CNIImportPreflight) [sha256.Size]byte {
	hash := sha256.New()
	var generation [8]byte
	binary.LittleEndian.PutUint64(generation[:], plan.ExpectedGeneration)
	_, _ = hash.Write(generation[:])
	_, _ = hash.Write([]byte(plan.IdentityDigest))
	if plan.allowReplacement {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	countData, _ := json.Marshal(plan.Counts)
	_, _ = hash.Write(countData)
	identityData, _ := json.Marshal(plan.identities)
	_, _ = hash.Write(identityData)
	sessionData, _ := json.Marshal(cniImportSession{
		Endpoints:   plan.endpoints,
		Assignments: plan.assignments,
		Owners:      plan.owners,
	})
	_, _ = hash.Write(sessionData)
	var seal [sha256.Size]byte
	copy(seal[:], hash.Sum(nil))
	return seal
}

func validateCNIImportPlan(plan CNIImportPreflight) error {
	if plan.IdentityDigest == "" {
		return invalidInput("CNI import preflight digest is empty", nil)
	}
	expectedSeal := sealCNIImportPlan(plan)
	if !bytes.Equal(plan.seal[:], expectedSeal[:]) {
		return invalidInput("CNI import preflight was modified", nil)
	}
	return nil
}

func sameCNIImportPlan(left, right CNIImportPreflight) bool {
	return left.ExpectedGeneration == right.ExpectedGeneration &&
		left.Counts == right.Counts &&
		left.IdentityDigest == right.IdentityDigest &&
		left.allowReplacement == right.allowReplacement &&
		slices.Equal(left.identities, right.identities) &&
		bytes.Equal(left.seal[:], right.seal[:])
}

func sessionEmpty(snapshot Snapshot) bool {
	return len(snapshot.Endpoints) == 0 && len(snapshot.Assignments) == 0 && len(snapshot.IPOwners) == 0
}

func sessionEqual(left, right Snapshot) bool {
	leftData, leftErr := json.Marshal(cniImportSession{
		Endpoints:   left.Endpoints,
		Assignments: left.Assignments,
		Owners:      left.IPOwners,
	})
	rightData, rightErr := json.Marshal(cniImportSession{
		Endpoints:   right.Endpoints,
		Assignments: right.Assignments,
		Owners:      right.IPOwners,
	})
	return leftErr == nil && rightErr == nil && bytes.Equal(leftData, rightData)
}

func encodeCNIImportSession(candidate Snapshot) ([]encodedBucket, error) {
	endpoints, err := encodeJSONMap(candidate.Endpoints)
	if err != nil {
		return nil, err
	}
	assignments, err := encodeJSONMap(candidate.Assignments)
	if err != nil {
		return nil, err
	}
	owners, err := encodeJSONMap(candidate.IPOwners)
	if err != nil {
		return nil, err
	}
	return []encodedBucket{
		{name: bucketEndpoints, values: endpoints},
		{name: bucketAssignments, values: assignments},
		{name: bucketIPOwners, values: owners},
	}, nil
}

func cloneCNIEndpoints(input map[string]EndpointRecord) map[string]EndpointRecord {
	cloned := make(map[string]EndpointRecord, len(input))
	for key, endpoint := range input {
		value := EndpointRecord{
			PodName:       endpoint.PodName,
			PodNamespace:  endpoint.PodNamespace,
			IfnameToIPMap: make(map[string]*IPInfoRecord, len(endpoint.IfnameToIPMap)),
		}
		for ifname, info := range endpoint.IfnameToIPMap {
			if info == nil {
				value.IfnameToIPMap[ifname] = nil
				continue
			}
			clonedInfo := *info
			clonedInfo.IPv4 = cloneIPNets(info.IPv4)
			clonedInfo.IPv6 = cloneIPNets(info.IPv6)
			value.IfnameToIPMap[ifname] = &clonedInfo
		}
		cloned[key] = value
	}
	return cloned
}

func cloneCNIAssignments(input map[string]AssignmentRecord) map[string]AssignmentRecord {
	cloned := make(map[string]AssignmentRecord, len(input))
	for key, assignment := range input {
		assignment.IPIDs = append([]string{}, assignment.IPIDs...)
		cloned[key] = assignment
	}
	return cloned
}

func cloneIPNets(input []net.IPNet) []net.IPNet {
	cloned := make([]net.IPNet, 0, len(input))
	for _, value := range input {
		cloned = append(cloned, net.IPNet{
			IP:   append(net.IP{}, value.IP...),
			Mask: append(net.IPMask{}, value.Mask...),
		})
	}
	return cloned
}
