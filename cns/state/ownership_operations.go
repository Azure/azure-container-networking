// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"
)

func (s *DB) AssignEndpoint(
	ctx context.Context,
	assignment AssignmentRecord,
	endpoint EndpointRecord,
	now time.Time,
	deleteIntentTTL time.Duration,
) (bool, error) {
	normalizedAssignment, err := normalizeAssignment(assignment, true)
	if err != nil {
		return false, err
	}
	normalizedEndpoint, err := normalizeEndpoint(normalizedAssignment.Pod.InfraContainerID, endpoint)
	if err != nil {
		return false, err
	}
	if err := validateEndpointPod(normalizedAssignment.Pod, normalizedEndpoint); err != nil {
		return false, err
	}
	now, err = normalizeNow(now, deleteIntentTTL)
	if err != nil {
		return false, err
	}

	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		candidate := cloneSnapshot(current)
		containerID := normalizedAssignment.Pod.InfraContainerID
		removeExpiredIntent := false
		if intent, ok := current.DeleteIntents[containerID]; ok {
			if deleteIntentLive(intent, now, deleteIntentTTL) {
				return false, fmt.Errorf("%w: infra container %q", ErrDeleteIntent, containerID)
			}
			delete(candidate.DeleteIntents, containerID)
			removeExpiredIntent = true
		}

		if err := preflightAssignmentIdentity(current, normalizedAssignment); err != nil {
			return false, err
		}
		for _, ipID := range normalizedAssignment.IPIDs {
			if _, ok := current.IPs[ipID]; !ok {
				return false, invalidInput(fmt.Sprintf("assignment references missing IP %q", ipID), nil)
			}
			if owner, ok := current.IPOwners[ipID]; ok && owner != normalizedAssignment.Pod.PodKey {
				return false, fmt.Errorf("%w: IP %q is owned by %q", ErrIPAlreadyAssigned, ipID, owner)
			}
		}

		existingAssignment, assignmentExists := current.Assignments[normalizedAssignment.Pod.PodKey]
		if assignmentExists && !assignmentEqual(existingAssignment, normalizedAssignment) {
			return false, invalidInput(
				fmt.Sprintf("assignment %q requires explicit release before change", normalizedAssignment.Pod.PodKey),
				nil,
			)
		}
		existingEndpoint, endpointExists := current.Endpoints[containerID]
		if assignmentExists && endpointExists {
			equal, err := endpointsEqual(existingEndpoint, normalizedEndpoint)
			if err != nil {
				return false, err
			}
			if !equal {
				return false, invalidInput(
					fmt.Sprintf("endpoint %q requires PatchEndpoint before change", containerID),
					nil,
				)
			}
		}

		candidate.Assignments[normalizedAssignment.Pod.PodKey] = normalizedAssignment
		for _, ipID := range normalizedAssignment.IPIDs {
			candidate.IPOwners[ipID] = normalizedAssignment.Pod.PodKey
		}
		candidate.Endpoints[containerID] = normalizedEndpoint
		if err := validateInput(candidate); err != nil {
			return false, err
		}

		assignmentData, err := encodeJSONInput(normalizedAssignment)
		if err != nil {
			return false, invalidInput("encoding assignment", err)
		}
		endpointData, err := encodeJSONInput(normalizedEndpoint)
		if err != nil {
			return false, invalidInput("encoding endpoint", err)
		}
		ownerData, err := encodeJSONMap(ownersFromAssignment(normalizedAssignment))
		if err != nil {
			return false, err
		}

		endpointChanged := true
		if endpointExists {
			equal, equalErr := endpointsEqual(existingEndpoint, normalizedEndpoint)
			err = equalErr
			if err != nil {
				return false, err
			}
			endpointChanged = !equal
		}
		if assignmentExists && !endpointChanged && !removeExpiredIntent {
			return false, nil
		}

		if removeExpiredIntent {
			if err := tx.tx.Bucket(bucketDeleteIntents).Delete([]byte(containerID)); err != nil {
				return false, fmt.Errorf("deleting expired intent %q: %w", containerID, err)
			}
		}
		if err := tx.tx.Bucket(bucketAssignments).Put([]byte(normalizedAssignment.Pod.PodKey), assignmentData); err != nil {
			return false, fmt.Errorf("writing assignment %q: %w", normalizedAssignment.Pod.PodKey, err)
		}
		ownerBucket := tx.tx.Bucket(bucketIPOwners)
		for _, ipID := range sortedKeys(ownerData) {
			if err := ownerBucket.Put([]byte(ipID), ownerData[ipID]); err != nil {
				return false, fmt.Errorf("writing IP owner %q: %w", ipID, err)
			}
		}
		if err := tx.tx.Bucket(bucketEndpoints).Put([]byte(containerID), endpointData); err != nil {
			return false, fmt.Errorf("writing endpoint %q: %w", containerID, err)
		}
		return true, nil
	})
}

func (s *DB) ReleaseEndpoint(
	ctx context.Context,
	pod PodIdentity,
	now time.Time,
) (bool, error) {
	normalizedPod, err := normalizePodIdentity(pod, true)
	if err != nil {
		return false, err
	}
	now, err = normalizeTimestamp(now, "release time")
	if err != nil {
		return false, err
	}

	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if err := validateReleaseIdentity(current, normalizedPod); err != nil {
			return false, err
		}

		candidate := cloneSnapshot(current)
		assignmentKeys := assignmentsForContainer(current, normalizedPod.InfraContainerID)
		for _, assignmentKey := range assignmentKeys {
			assignment := current.Assignments[assignmentKey]
			delete(candidate.Assignments, assignmentKey)
			for _, ipID := range assignment.IPIDs {
				if owner, ok := current.IPOwners[ipID]; !ok || owner != assignmentKey {
					return false, inconsistent(
						"assignment %q IP %q owner is %q",
						assignmentKey,
						ipID,
						owner,
					)
				}
				delete(candidate.IPOwners, ipID)
			}
		}

		_, intentExists := current.DeleteIntents[normalizedPod.InfraContainerID]
		intent := DeleteIntent{CreatedAt: now}
		if intentExists {
			intent = current.DeleteIntents[normalizedPod.InfraContainerID]
		} else {
			candidate.DeleteIntents[normalizedPod.InfraContainerID] = intent
		}
		if err := validateInput(candidate); err != nil {
			return false, err
		}
		intentData, err := encodeJSONInput(intent)
		if err != nil {
			return false, invalidInput("encoding delete intent", err)
		}
		if len(assignmentKeys) == 0 && intentExists {
			return false, nil
		}

		if !intentExists {
			if err := tx.tx.Bucket(bucketDeleteIntents).Put(
				[]byte(normalizedPod.InfraContainerID),
				intentData,
			); err != nil {
				return false, fmt.Errorf("writing delete intent %q: %w", normalizedPod.InfraContainerID, err)
			}
		}
		assignmentBucket := tx.tx.Bucket(bucketAssignments)
		ownerBucket := tx.tx.Bucket(bucketIPOwners)
		for _, assignmentKey := range assignmentKeys {
			assignment := current.Assignments[assignmentKey]
			if err := assignmentBucket.Delete([]byte(assignmentKey)); err != nil {
				return false, fmt.Errorf("deleting assignment %q: %w", assignmentKey, err)
			}
			for _, ipID := range assignment.IPIDs {
				if err := ownerBucket.Delete([]byte(ipID)); err != nil {
					return false, fmt.Errorf("deleting IP owner %q: %w", ipID, err)
				}
			}
		}
		return true, nil
	})
}

func (s *DB) PatchEndpoint(
	ctx context.Context,
	pod PodIdentity,
	endpoint EndpointRecord,
	now time.Time,
	deleteIntentTTL time.Duration,
) (bool, error) {
	normalizedPod, err := normalizePodIdentity(pod, true)
	if err != nil {
		return false, err
	}
	normalizedEndpoint, err := normalizeEndpoint(normalizedPod.InfraContainerID, endpoint)
	if err != nil {
		return false, err
	}
	if err := validateEndpointPod(normalizedPod, normalizedEndpoint); err != nil {
		return false, err
	}
	now, err = normalizeNow(now, deleteIntentTTL)
	if err != nil {
		return false, err
	}

	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if intent, ok := current.DeleteIntents[normalizedPod.InfraContainerID]; ok &&
			deleteIntentLive(intent, now, deleteIntentTTL) {
			return false, fmt.Errorf("%w: infra container %q", ErrDeleteIntent, normalizedPod.InfraContainerID)
		}
		assignment, ok := current.Assignments[normalizedPod.PodKey]
		if !ok {
			return false, fmt.Errorf("%w: assignment %q", ErrNotFound, normalizedPod.PodKey)
		}
		if assignment.Pod != normalizedPod {
			return false, invalidInput(fmt.Sprintf("assignment %q pod identity mismatch", normalizedPod.PodKey), nil)
		}
		if _, ok := current.Endpoints[normalizedPod.InfraContainerID]; !ok {
			return false, fmt.Errorf("%w: endpoint %q", ErrNotFound, normalizedPod.InfraContainerID)
		}

		candidate := cloneSnapshot(current)
		candidate.Endpoints[normalizedPod.InfraContainerID] = normalizedEndpoint
		if err := validateInput(candidate); err != nil {
			return false, err
		}
		equal, err := endpointsEqual(current.Endpoints[normalizedPod.InfraContainerID], normalizedEndpoint)
		if err != nil {
			return false, err
		}
		if equal {
			return false, nil
		}
		data, err := encodeJSONInput(normalizedEndpoint)
		if err != nil {
			return false, invalidInput("encoding endpoint", err)
		}
		if err := tx.tx.Bucket(bucketEndpoints).Put([]byte(normalizedPod.InfraContainerID), data); err != nil {
			return false, fmt.Errorf("writing endpoint %q: %w", normalizedPod.InfraContainerID, err)
		}
		return true, nil
	})
}

func (s *DB) DeleteEndpointRecord(ctx context.Context, infraContainerID string) (bool, error) {
	infraContainerID = normalizeID(infraContainerID)
	if infraContainerID == "" {
		return false, invalidInput("infra container ID is empty", nil)
	}
	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if _, ok := current.Endpoints[infraContainerID]; !ok {
			return false, nil
		}
		candidate := cloneSnapshot(current)
		delete(candidate.Endpoints, infraContainerID)
		if err := validateInput(candidate); err != nil {
			return false, err
		}
		if err := tx.tx.Bucket(bucketEndpoints).Delete([]byte(infraContainerID)); err != nil {
			return false, fmt.Errorf("deleting endpoint %q: %w", infraContainerID, err)
		}
		return true, nil
	})
}

func (s *DB) PruneDeleteIntents(
	ctx context.Context,
	now time.Time,
	deleteIntentTTL time.Duration,
) (int, error) {
	now, err := normalizeNow(now, deleteIntentTTL)
	if err != nil {
		return 0, err
	}
	count := 0
	_, err = s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		expired := make([]string, 0)
		for _, containerID := range sortedKeys(current.DeleteIntents) {
			if !deleteIntentLive(current.DeleteIntents[containerID], now, deleteIntentTTL) {
				expired = append(expired, containerID)
			}
		}
		if len(expired) == 0 {
			return false, nil
		}
		candidate := cloneSnapshot(current)
		for _, containerID := range expired {
			delete(candidate.DeleteIntents, containerID)
		}
		if err := validateInput(candidate); err != nil {
			return false, err
		}
		bucket := tx.tx.Bucket(bucketDeleteIntents)
		for _, containerID := range expired {
			if err := bucket.Delete([]byte(containerID)); err != nil {
				return false, fmt.Errorf("deleting expired intent %q: %w", containerID, err)
			}
		}
		count = len(expired)
		return true, nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func normalizeAssignment(record AssignmentRecord, requirePodNames bool) (AssignmentRecord, error) {
	pod, err := normalizePodIdentity(record.Pod, requirePodNames)
	if err != nil {
		return AssignmentRecord{}, err
	}
	if len(record.IPIDs) == 0 {
		return AssignmentRecord{}, invalidInput("assignment has no IP IDs", nil)
	}
	ipIDs := make([]string, 0, len(record.IPIDs))
	seen := make(map[string]struct{}, len(record.IPIDs))
	for _, value := range record.IPIDs {
		ipID := normalizeID(value)
		if ipID == "" {
			return AssignmentRecord{}, invalidInput("assignment contains an empty IP ID", nil)
		}
		if _, ok := seen[ipID]; ok {
			return AssignmentRecord{}, invalidInput(fmt.Sprintf("duplicate IP ID %q", ipID), nil)
		}
		seen[ipID] = struct{}{}
		ipIDs = append(ipIDs, ipID)
	}
	sort.Strings(ipIDs)
	return AssignmentRecord{Pod: pod, IPIDs: ipIDs}, nil
}

func normalizePodIdentity(pod PodIdentity, requirePodNames bool) (PodIdentity, error) {
	normalized := PodIdentity{
		PodKey:           normalizeID(pod.PodKey),
		InfraContainerID: normalizeID(pod.InfraContainerID),
		InterfaceID:      normalizeID(pod.InterfaceID),
		PodName:          strings.TrimSpace(pod.PodName),
		PodNamespace:     strings.TrimSpace(pod.PodNamespace),
	}
	switch {
	case normalized.PodKey == "":
		return PodIdentity{}, invalidInput("pod key is empty", nil)
	case normalized.InfraContainerID == "":
		return PodIdentity{}, invalidInput("infra container ID is empty", nil)
	case normalized.InterfaceID == "" && normalized.PodKey != normalized.InfraContainerID:
		return PodIdentity{}, invalidInput("pod key must equal infra container ID without an interface ID", nil)
	case normalized.InterfaceID != "" && normalized.PodKey != normalized.InterfaceID:
		return PodIdentity{}, invalidInput("pod key must equal interface ID", nil)
	case requirePodNames && normalized.PodName == "":
		return PodIdentity{}, invalidInput("pod name is empty", nil)
	case requirePodNames && normalized.PodNamespace == "":
		return PodIdentity{}, invalidInput("pod namespace is empty", nil)
	}
	return normalized, nil
}

func normalizeEndpoint(infraContainerID string, record EndpointRecord) (EndpointRecord, error) {
	if normalizeID(infraContainerID) == "" {
		return EndpointRecord{}, invalidInput("infra container ID is empty", nil)
	}
	normalized := EndpointRecord{
		PodName:       strings.TrimSpace(record.PodName),
		PodNamespace:  strings.TrimSpace(record.PodNamespace),
		IfnameToIPMap: make(map[string]*IPInfoRecord, len(record.IfnameToIPMap)),
	}
	if normalized.PodName == "" {
		return EndpointRecord{}, invalidInput("endpoint pod name is empty", nil)
	}
	if normalized.PodNamespace == "" {
		return EndpointRecord{}, invalidInput("endpoint pod namespace is empty", nil)
	}
	if len(record.IfnameToIPMap) == 0 {
		return EndpointRecord{}, invalidInput("endpoint has no interfaces", nil)
	}
	for _, rawIfname := range sortedKeys(record.IfnameToIPMap) {
		ifname := strings.TrimSpace(rawIfname)
		if ifname == "" {
			return EndpointRecord{}, invalidInput("endpoint interface name is empty", nil)
		}
		if _, ok := normalized.IfnameToIPMap[ifname]; ok {
			return EndpointRecord{}, invalidInput(fmt.Sprintf("duplicate interface name %q", ifname), nil)
		}
		info := record.IfnameToIPMap[rawIfname]
		if info == nil {
			return EndpointRecord{}, invalidInput(fmt.Sprintf("interface %q is null", ifname), nil)
		}
		value, err := normalizeIPInfo(ifname, *info)
		if err != nil {
			return EndpointRecord{}, err
		}
		normalized.IfnameToIPMap[ifname] = &value
	}
	return normalized, nil
}

func normalizeIPInfo(ifname string, input IPInfoRecord) (IPInfoRecord, error) {
	output := IPInfoRecord{
		HNSEndpointID:      normalizeID(input.HNSEndpointID),
		HNSNetworkID:       normalizeID(input.HNSNetworkID),
		HostVethName:       strings.TrimSpace(input.HostVethName),
		MACAddress:         strings.TrimSpace(input.MACAddress),
		NetworkContainerID: normalizeID(input.NetworkContainerID),
		NICType:            input.NICType,
		IPv4:               make([]net.IPNet, 0, len(input.IPv4)),
		IPv6:               make([]net.IPNet, 0, len(input.IPv6)),
	}
	if output.MACAddress != "" {
		mac, err := net.ParseMAC(output.MACAddress)
		if err != nil {
			return IPInfoRecord{}, invalidInput(
				fmt.Sprintf("interface %q has invalid MAC address %q", ifname, output.MACAddress),
				err,
			)
		}
		output.MACAddress = mac.String()
	}
	for _, family := range []struct {
		name   string
		bits   int
		input  []net.IPNet
		output *[]net.IPNet
	}{
		{name: "IPv4", bits: 32, input: input.IPv4, output: &output.IPv4},
		{name: "IPv6", bits: 128, input: input.IPv6, output: &output.IPv6},
	} {
		for index, prefix := range family.input {
			address, err := validateIPNet(prefix, family.bits)
			if err != nil {
				return IPInfoRecord{}, invalidInput(
					fmt.Sprintf("interface %q %s prefix %d", ifname, family.name, index),
					err,
				)
			}
			ones, _ := prefix.Mask.Size()
			*family.output = append(*family.output, net.IPNet{
				IP:   net.IP(address.AsSlice()),
				Mask: net.CIDRMask(ones, family.bits),
			})
		}
	}
	if len(output.IPv4)+len(output.IPv6) == 0 {
		return IPInfoRecord{}, invalidInput(fmt.Sprintf("interface %q has no IPs", ifname), nil)
	}
	return output, nil
}

func normalizeDeleteIntent(intent DeleteIntent) (DeleteIntent, error) {
	createdAt, err := normalizeTimestamp(intent.CreatedAt, "delete intent creation time")
	if err != nil {
		return DeleteIntent{}, err
	}
	return DeleteIntent{CreatedAt: createdAt}, nil
}

func normalizeNow(now time.Time, ttl time.Duration) (time.Time, error) {
	if ttl <= 0 {
		return time.Time{}, invalidInput("delete intent TTL must be positive", nil)
	}
	return normalizeTimestamp(now, "current time")
}

func normalizeTimestamp(value time.Time, name string) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, invalidInput(name+" is zero", nil)
	}
	return value.Round(0).UTC(), nil
}

func deleteIntentLive(intent DeleteIntent, now time.Time, ttl time.Duration) bool {
	// The expiry boundary is exclusive: an intent is live while now is before CreatedAt+ttl.
	return now.Before(intent.CreatedAt.Add(ttl))
}

func validateEndpointPod(pod PodIdentity, endpoint EndpointRecord) error {
	if endpoint.PodName != pod.PodName || endpoint.PodNamespace != pod.PodNamespace {
		return invalidInput("endpoint pod identity does not match assignment", nil)
	}
	return nil
}

func preflightAssignmentIdentity(current Snapshot, requested AssignmentRecord) error {
	for _, key := range sortedKeys(current.Assignments) {
		existing := current.Assignments[key]
		sameContainer := existing.Pod.InfraContainerID == requested.Pod.InfraContainerID
		samePod := existing.Pod.PodName == requested.Pod.PodName &&
			existing.Pod.PodNamespace == requested.Pod.PodNamespace
		if sameContainer && !samePod {
			return invalidInput(
				fmt.Sprintf("infra container %q is owned by another pod", requested.Pod.InfraContainerID),
				nil,
			)
		}
		if samePod && !sameContainer {
			return invalidInput(
				fmt.Sprintf("pod %q/%q requires explicit release before changing containers",
					requested.Pod.PodNamespace,
					requested.Pod.PodName,
				),
				nil,
			)
		}
	}
	for _, containerID := range sortedKeys(current.Endpoints) {
		endpoint := current.Endpoints[containerID]
		samePod := endpoint.PodName == requested.Pod.PodName &&
			endpoint.PodNamespace == requested.Pod.PodNamespace
		if samePod && containerID != requested.Pod.InfraContainerID {
			return invalidInput(
				fmt.Sprintf("pod %q/%q has retained endpoint %q",
					requested.Pod.PodNamespace,
					requested.Pod.PodName,
					containerID,
				),
				nil,
			)
		}
	}
	if endpoint, ok := current.Endpoints[requested.Pod.InfraContainerID]; ok {
		return validateEndpointPod(requested.Pod, endpoint)
	}
	return nil
}

func validateReleaseIdentity(current Snapshot, requested PodIdentity) error {
	if endpoint, ok := current.Endpoints[requested.InfraContainerID]; ok {
		if err := validateEndpointPod(requested, endpoint); err != nil {
			return err
		}
	}
	if assignment, ok := current.Assignments[requested.PodKey]; ok &&
		assignment.Pod.InfraContainerID != requested.InfraContainerID {
		return invalidInput(fmt.Sprintf("assignment %q infra container mismatch", requested.PodKey), nil)
	}
	for _, key := range assignmentsForContainer(current, requested.InfraContainerID) {
		existing := current.Assignments[key].Pod
		if existing.PodName != requested.PodName || existing.PodNamespace != requested.PodNamespace {
			return invalidInput(fmt.Sprintf("infra container %q pod identity mismatch", requested.InfraContainerID), nil)
		}
	}
	return nil
}

func assignmentsForContainer(snapshot Snapshot, infraContainerID string) []string {
	keys := make([]string, 0)
	for _, key := range sortedKeys(snapshot.Assignments) {
		if snapshot.Assignments[key].Pod.InfraContainerID == infraContainerID {
			keys = append(keys, key)
		}
	}
	return keys
}

func assignmentEqual(left, right AssignmentRecord) bool {
	return left.Pod == right.Pod && slices.Equal(left.IPIDs, right.IPIDs)
}

func endpointsEqual(left, right EndpointRecord) (bool, error) {
	leftNormalized, err := normalizeEndpoint("comparison", left)
	if err != nil {
		return false, err
	}
	rightNormalized, err := normalizeEndpoint("comparison", right)
	if err != nil {
		return false, err
	}
	leftData, err := json.Marshal(leftNormalized)
	if err != nil {
		return false, invalidInput("encoding existing endpoint", err)
	}
	rightData, err := json.Marshal(rightNormalized)
	if err != nil {
		return false, invalidInput("encoding endpoint", err)
	}
	return bytes.Equal(leftData, rightData), nil
}

func ownersFromAssignment(record AssignmentRecord) map[string]string {
	owners := make(map[string]string, len(record.IPIDs))
	for _, ipID := range record.IPIDs {
		owners[ipID] = record.Pod.PodKey
	}
	return owners
}

func normalizeID(value string) string {
	return strings.TrimSpace(value)
}

func endpointAddresses(record EndpointRecord) ([]netip.Addr, error) {
	addresses := make([]netip.Addr, 0)
	for _, ifname := range sortedKeys(record.IfnameToIPMap) {
		info := record.IfnameToIPMap[ifname]
		if info == nil {
			return nil, corruptValue(bucketEndpoints, "", fmt.Sprintf("interface %q is null", ifname), nil)
		}
		for _, family := range []struct {
			bits     int
			prefixes []net.IPNet
		}{
			{bits: 32, prefixes: info.IPv4},
			{bits: 128, prefixes: info.IPv6},
		} {
			for _, prefix := range family.prefixes {
				address, err := validateIPNet(prefix, family.bits)
				if err != nil {
					return nil, err
				}
				addresses = append(addresses, address)
			}
		}
	}
	slices.SortFunc(addresses, func(left, right netip.Addr) int {
		return left.Compare(right)
	})
	return addresses, nil
}
