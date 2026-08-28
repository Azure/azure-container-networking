// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (s *DB) ApplyNetworkContainer(
	ctx context.Context,
	record NetworkContainerRecord,
	ips []IPRecord,
) (bool, error) {
	normalized, err := normalizeNetworkContainer(record)
	if err != nil {
		return false, err
	}
	inventory := make(map[string]IPRecord, len(ips))
	for _, ip := range ips {
		switch {
		case ip.ID == "":
			return false, invalidInput("IP ID is empty", nil)
		case ip.NCID != normalized.ID:
			return false, invalidInput(
				fmt.Sprintf("IP %q NC %q does not match network container %q", ip.ID, ip.NCID, normalized.ID),
				nil,
			)
		}
		if _, ok := inventory[ip.ID]; ok {
			return false, invalidInput(fmt.Sprintf("duplicate IP ID %q", ip.ID), nil)
		}
		inventory[ip.ID] = ip
	}

	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		candidate := cloneSnapshot(current)
		candidate.NetworkContainers[normalized.ID] = normalized
		oldIPIDs := []string{}
		for id, ip := range candidate.IPs {
			if ip.NCID == normalized.ID {
				oldIPIDs = append(oldIPIDs, id)
				delete(candidate.IPs, id)
			}
		}
		for id, ip := range inventory {
			candidate.IPs[id] = ip
		}
		if verr := validateInput(candidate); verr != nil {
			return false, verr
		}
		if verr := validateEndpointIPPreservation(current, candidate); verr != nil {
			return false, verr
		}

		ncData, err := encodeJSONInput(normalized)
		if err != nil {
			return false, invalidInput("encoding network container", err)
		}
		ipData, err := encodeJSONMap(inventory)
		if err != nil {
			return false, err
		}

		if err := tx.tx.Bucket(bucketNetworkContainers).Put([]byte(normalized.ID), ncData); err != nil {
			return false, fmt.Errorf("writing network container %q: %w", normalized.ID, err)
		}
		ipBucket := tx.tx.Bucket(bucketIPs)
		for _, id := range oldIPIDs {
			if err := ipBucket.Delete([]byte(id)); err != nil {
				return false, fmt.Errorf("deleting IP %q: %w", id, err)
			}
		}
		for _, id := range sortedKeys(ipData) {
			if err := ipBucket.Put([]byte(id), ipData[id]); err != nil {
				return false, fmt.Errorf("writing IP %q: %w", id, err)
			}
		}
		return true, nil
	})
}

func (s *DB) DeleteNetworkContainer(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, invalidInput("network container ID is empty", nil)
	}
	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if _, ok := current.NetworkContainers[id]; !ok {
			return false, nil
		}

		candidate := cloneSnapshot(current)
		delete(candidate.NetworkContainers, id)
		oldIPIDs := []string{}
		for ipID, ip := range candidate.IPs {
			if ip.NCID == id {
				oldIPIDs = append(oldIPIDs, ipID)
				delete(candidate.IPs, ipID)
			}
		}
		if err := validateInput(candidate); err != nil {
			return false, err
		}
		if err := validateEndpointIPPreservation(current, candidate); err != nil {
			return false, err
		}

		if err := tx.tx.Bucket(bucketNetworkContainers).Delete([]byte(id)); err != nil {
			return false, fmt.Errorf("deleting network container %q: %w", id, err)
		}
		ipBucket := tx.tx.Bucket(bucketIPs)
		for _, ipID := range oldIPIDs {
			if err := ipBucket.Delete([]byte(ipID)); err != nil {
				return false, fmt.Errorf("deleting IP %q: %w", ipID, err)
			}
		}
		return true, nil
	})
}

func (s *DB) ReplaceDurableState(
	ctx context.Context,
	expectedGeneration uint64,
	durable DurableState,
) (bool, error) {
	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		current, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if current.Metadata.Generation != expectedGeneration {
			return false, fmt.Errorf(
				"%w: expected=%d actual=%d",
				ErrStaleGeneration,
				expectedGeneration,
				current.Metadata.Generation,
			)
		}

		normalized, err := normalizeDurableState(durable)
		if err != nil {
			return false, err
		}
		candidate := cloneSnapshot(current)
		candidate.NetworkContainers = normalized.NetworkContainers
		candidate.IPs = normalized.IPs
		candidate.Networks = normalized.Networks
		candidate.OrchestratorContexts = normalized.OrchestratorContexts
		candidate.PnPIDByMAC = normalized.PnPIDByMAC
		if verr := validateInput(candidate); verr != nil {
			return false, verr
		}
		if verr := validateEndpointIPPreservation(current, candidate); verr != nil {
			return false, verr
		}

		encoded, err := encodeDurableState(normalized)
		if err != nil {
			return false, err
		}
		for _, replacement := range encoded {
			if err := replaceJSONBucket(tx.tx, replacement.name, replacement.values); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

func (s *DB) UpdateReadinessObservation(
	ctx context.Context,
	id string,
	observation ReadinessObservation,
) (bool, error) {
	if id == "" {
		return false, invalidInput("network container ID is empty", nil)
	}
	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		candidate, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		record, ok := candidate.NetworkContainers[id]
		if !ok {
			return false, fmt.Errorf("%w: network container %q", ErrNotFound, id)
		}
		if record.VMVersion == observation.VMVersion &&
			record.HostVersion == observation.HostVersion &&
			record.VFPUpdateComplete == observation.VFPUpdateComplete {
			return false, nil
		}
		record.VMVersion = observation.VMVersion
		record.HostVersion = observation.HostVersion
		record.VFPUpdateComplete = observation.VFPUpdateComplete
		candidate.NetworkContainers[id] = record
		if verr := validateInput(candidate); verr != nil {
			return false, verr
		}
		data, err := encodeJSONInput(record)
		if err != nil {
			return false, invalidInput("encoding network container readiness", err)
		}
		if err := tx.tx.Bucket(bucketNetworkContainers).Put([]byte(id), data); err != nil {
			return false, fmt.Errorf("writing network container %q readiness: %w", id, err)
		}
		return true, nil
	})
}

func (s *DB) ApplyBoot(ctx context.Context, bootID string, policy BootPolicy) (bool, error) {
	if bootID == "" {
		return false, invalidInput("boot ID is empty", nil)
	}
	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		candidate, err := tx.validSnapshot()
		if err != nil {
			return false, err
		}
		if candidate.Metadata.BootID == bootID {
			return false, nil
		}

		candidate.Metadata.BootID = bootID
		candidate.DeleteIntents = map[string]DeleteIntent{}
		if policy.ClearEndpoints {
			candidate.Assignments = map[string]AssignmentRecord{}
			candidate.IPOwners = map[string]string{}
			candidate.Endpoints = map[string]EndpointRecord{}
		} else {
			assignments, owners, err := reconstructRetainedEndpointOwnership(candidate)
			if err != nil {
				return false, err
			}
			candidate.Assignments = assignments
			candidate.IPOwners = owners
		}
		if policy.ResetReadiness {
			for id := range candidate.NetworkContainers {
				record := candidate.NetworkContainers[id]
				record.HostVersion = ""
				record.VFPUpdateComplete = false
				candidate.NetworkContainers[id] = record
			}
		}
		if verr := validateInput(candidate); verr != nil {
			return false, verr
		}

		assignmentData, err := encodeJSONMap(candidate.Assignments)
		if err != nil {
			return false, err
		}
		ownerData, err := encodeJSONMap(candidate.IPOwners)
		if err != nil {
			return false, err
		}
		var ncData map[string][]byte
		if policy.ResetReadiness {
			ncData, err = encodeJSONMap(candidate.NetworkContainers)
			if err != nil {
				return false, err
			}
		}
		if err := tx.tx.Bucket(bucketMetadata).Put(metaKeyBootID, []byte(bootID)); err != nil {
			return false, fmt.Errorf("writing boot ID: %w", err)
		}
		if err := replaceJSONBucket(tx.tx, bucketAssignments, assignmentData); err != nil {
			return false, err
		}
		if err := replaceJSONBucket(tx.tx, bucketIPOwners, ownerData); err != nil {
			return false, err
		}
		if err := clearBucket(tx.tx.Bucket(bucketDeleteIntents)); err != nil {
			return false, fmt.Errorf("clearing bucket %q: %w", bucketDeleteIntents, err)
		}

		if policy.ClearEndpoints {
			if err := clearBucket(tx.tx.Bucket(bucketEndpoints)); err != nil {
				return false, fmt.Errorf("clearing bucket %q: %w", bucketEndpoints, err)
			}
		}
		if policy.ResetReadiness {
			if err := replaceJSONBucket(tx.tx, bucketNetworkContainers, ncData); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

func reconstructRetainedEndpointOwnership(
	snapshot Snapshot,
) (map[string]AssignmentRecord, map[string]string, error) {
	ipAddresses, err := snapshot.validateIPs()
	if err != nil {
		return nil, nil, err
	}
	ipIDByAddress := make(map[string]string, len(ipAddresses))
	for _, ipID := range sortedKeys(ipAddresses) {
		address := ipAddresses[ipID].String()
		if other, ok := ipIDByAddress[address]; ok {
			return nil, nil, invalidInput(
				fmt.Sprintf("retained endpoint IP %q has duplicate inventory IDs %q and %q", address, other, ipID),
				nil,
			)
		}
		ipIDByAddress[address] = ipID
	}

	assignments := make(map[string]AssignmentRecord, len(snapshot.Assignments)+len(snapshot.Endpoints))
	for key, assignment := range snapshot.Assignments {
		assignment.IPIDs = append([]string(nil), assignment.IPIDs...)
		assignments[key] = assignment
	}
	owners := make(map[string]string, len(snapshot.IPOwners)+len(snapshot.IPs))
	for ipID, owner := range snapshot.IPOwners {
		owners[ipID] = owner
	}
	for _, containerID := range sortedKeys(snapshot.Endpoints) {
		if len(assignmentsForContainer(snapshot, containerID)) != 0 {
			continue
		}
		endpoint := snapshot.Endpoints[containerID]
		addresses, err := endpointAddresses(endpoint)
		if err != nil {
			return nil, nil, invalidInput(fmt.Sprintf("retained endpoint %q", containerID), err)
		}
		ipIDs := make([]string, 0, len(addresses))
		for _, address := range addresses {
			ipID, ok := ipIDByAddress[address.String()]
			if !ok {
				return nil, nil, invalidInput(
					fmt.Sprintf("retained endpoint %q IP %q is missing from inventory", containerID, address),
					nil,
				)
			}
			ipIDs = append(ipIDs, ipID)
		}
		assignment, err := normalizeAssignment(AssignmentRecord{
			Pod: PodIdentity{
				PodKey:           containerID,
				InfraContainerID: containerID,
				PodName:          endpoint.PodName,
				PodNamespace:     endpoint.PodNamespace,
			},
			IPIDs: ipIDs,
		}, false)
		if err != nil {
			return nil, nil, err
		}
		assignments[containerID] = assignment
		for _, ipID := range assignment.IPIDs {
			if other, ok := owners[ipID]; ok {
				return nil, nil, invalidInput(
					fmt.Sprintf("retained endpoint IP %q is owned by %q and %q", ipID, other, containerID),
					nil,
				)
			}
			owners[ipID] = containerID
		}
	}
	return assignments, owners, nil
}

func normalizeDurableState(input DurableState) (DurableState, error) {
	normalized := NewDurableState()
	for key := range input.NetworkContainers {
		record := input.NetworkContainers[key]
		if key == "" || record.ID != key {
			return DurableState{}, invalidInput(
				fmt.Sprintf("network container key %q does not match record ID %q", key, record.ID),
				nil,
			)
		}
		value, err := normalizeNetworkContainer(record)
		if err != nil {
			return DurableState{}, err
		}
		normalized.NetworkContainers[key] = value
	}
	for key, record := range input.IPs {
		if key == "" || record.ID != key {
			return DurableState{}, invalidInput(
				fmt.Sprintf("IP key %q does not match record ID %q", key, record.ID),
				nil,
			)
		}
		normalized.IPs[key] = record
	}
	for key, record := range input.Networks {
		if key == "" || record.NetworkName != key {
			return DurableState{}, invalidInput(
				fmt.Sprintf("network key %q does not match record name %q", key, record.NetworkName),
				nil,
			)
		}
		normalized.Networks[key] = record
	}
	for key, ncIDs := range input.OrchestratorContexts {
		if key == "" {
			return DurableState{}, invalidInput("orchestrator context ID is empty", nil)
		}
		normalized.OrchestratorContexts[key] = append([]string{}, ncIDs...)
	}
	for key, id := range input.PnPIDByMAC {
		canonical, err := canonicalMAC(key)
		if err != nil {
			return DurableState{}, err
		}
		if id == "" {
			return DurableState{}, invalidInput(fmt.Sprintf("PnP ID for MAC %q is empty", key), nil)
		}
		if _, ok := normalized.PnPIDByMAC[canonical]; ok {
			return DurableState{}, invalidInput(fmt.Sprintf("duplicate canonical MAC address %q", canonical), nil)
		}
		normalized.PnPIDByMAC[canonical] = id
	}
	return normalized, nil
}

func cloneSnapshot(input Snapshot) Snapshot {
	cloned := input
	cloned.NetworkContainers = cloneMap(input.NetworkContainers)
	cloned.IPs = cloneMap(input.IPs)
	cloned.Networks = cloneMap(input.Networks)
	cloned.OrchestratorContexts = cloneMap(input.OrchestratorContexts)
	cloned.PnPIDByMAC = cloneMap(input.PnPIDByMAC)
	cloned.Assignments = cloneMap(input.Assignments)
	cloned.IPOwners = cloneMap(input.IPOwners)
	cloned.Endpoints = cloneMap(input.Endpoints)
	cloned.DeleteIntents = cloneMap(input.DeleteIntents)
	return cloned
}

func cloneMap[T any](input map[string]T) map[string]T {
	cloned := make(map[string]T, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func validateEndpointIPPreservation(current, candidate Snapshot) error {
	currentIPs, err := current.validateIPs()
	if err != nil {
		return err
	}
	endpointIPs, err := current.validateEndpoints()
	if err != nil {
		return err
	}
	candidateIPs, err := candidate.validateIPs()
	if err != nil {
		return fmt.Errorf("%w: candidate state: %w", ErrInvalidInput, err)
	}
	available := make(map[string]struct{}, len(candidateIPs))
	for _, address := range candidateIPs {
		available[address.String()] = struct{}{}
	}
	for ipID, address := range currentIPs {
		if _, referenced := endpointIPs[address]; !referenced {
			continue
		}
		if _, ok := available[address.String()]; !ok {
			return invalidInput(fmt.Sprintf("endpoint-referenced IP %q would be removed", ipID), nil)
		}
	}
	return nil
}

type encodedBucket struct {
	name   []byte
	values map[string][]byte
}

func encodeDurableState(durable DurableState) ([]encodedBucket, error) {
	networkContainers, err := encodeJSONMap(durable.NetworkContainers)
	if err != nil {
		return nil, err
	}
	ips, err := encodeJSONMap(durable.IPs)
	if err != nil {
		return nil, err
	}
	networks, err := encodeJSONMap(durable.Networks)
	if err != nil {
		return nil, err
	}
	orchestratorContexts, err := encodeJSONMap(durable.OrchestratorContexts)
	if err != nil {
		return nil, err
	}
	pnpIDs, err := encodeJSONMap(durable.PnPIDByMAC)
	if err != nil {
		return nil, err
	}
	return []encodedBucket{
		{name: bucketNetworkContainers, values: networkContainers},
		{name: bucketIPs, values: ips},
		{name: bucketNetworks, values: networks},
		{name: bucketOrchestratorContexts, values: orchestratorContexts},
		{name: bucketPnPIDByMAC, values: pnpIDs},
	}, nil
}

func encodeJSONMap[T any](values map[string]T) (map[string][]byte, error) {
	encoded := make(map[string][]byte, len(values))
	for key, value := range values {
		data, err := encodeJSONInput(value)
		if err != nil {
			return nil, invalidInput(fmt.Sprintf("encoding key %q", key), err)
		}
		encoded[key] = data
	}
	return encoded, nil
}

func encodeJSONInput[T any](value T) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshaling value: %w", err)
	}
	return data, nil
}

func replaceJSONBucket(tx *bolt.Tx, name []byte, values map[string][]byte) error {
	bucket := tx.Bucket(name)
	if bucket == nil {
		return corrupt(fmt.Sprintf("missing bucket %q", name), nil)
	}
	if err := clearBucket(bucket); err != nil {
		return fmt.Errorf("clearing bucket %q: %w", name, err)
	}
	for _, key := range sortedKeys(values) {
		if err := bucket.Put([]byte(key), values[key]); err != nil {
			return fmt.Errorf("writing bucket %q key %q: %w", name, key, err)
		}
	}
	return nil
}

func clearBucket(bucket *bolt.Bucket) error {
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.First() {
		if err := cursor.Delete(); err != nil {
			return fmt.Errorf("deleting key %q: %w", key, err)
		}
	}
	return nil
}
