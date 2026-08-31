// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (r *ReadTx) GetAssignment(podKey string) (AssignmentRecord, error) {
	return getJSONValue[AssignmentRecord](r, bucketAssignments, podKey)
}

func (r *ReadTx) ListAssignments() (map[string]AssignmentRecord, error) {
	return listJSONValues[AssignmentRecord](r, bucketAssignments)
}

func (w *WriteTx) PutAssignment(record AssignmentRecord) error {
	normalized, err := normalizeAssignment(record, true)
	if err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketAssignments, normalized.Pod.PodKey, normalized)
}

func (w *WriteTx) DeleteAssignment(podKey string) error {
	return deleteJSONValue(w.tx, bucketAssignments, podKey)
}

func (r *ReadTx) GetIPOwner(ipID string) (string, error) {
	return getJSONValue[string](r, bucketIPOwners, ipID)
}

func (r *ReadTx) ListIPOwners() (map[string]string, error) {
	return listJSONValues[string](r, bucketIPOwners)
}

func (w *WriteTx) PutIPOwner(ipID, podKey string) error {
	ipID = normalizeID(ipID)
	podKey = normalizeID(podKey)
	if ipID == "" {
		return invalidInput("IP ID is empty", nil)
	}
	if podKey == "" {
		return invalidInput("pod key is empty", nil)
	}
	return putJSONValue(w.tx, bucketIPOwners, ipID, podKey)
}

func (w *WriteTx) DeleteIPOwner(ipID string) error {
	return deleteJSONValue(w.tx, bucketIPOwners, normalizeID(ipID))
}

func (r *ReadTx) GetEndpoint(infraContainerID string) (EndpointRecord, error) {
	return getJSONValue[EndpointRecord](r, bucketEndpoints, infraContainerID)
}

func (r *ReadTx) ListEndpoints() (map[string]EndpointRecord, error) {
	return listJSONValues[EndpointRecord](r, bucketEndpoints)
}

func (w *WriteTx) PutEndpoint(infraContainerID string, record EndpointRecord) error {
	key := normalizeID(infraContainerID)
	normalized, err := normalizeEndpoint(key, record)
	if err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketEndpoints, key, normalized)
}

func (w *WriteTx) DeleteEndpoint(infraContainerID string) error {
	return deleteJSONValue(w.tx, bucketEndpoints, normalizeID(infraContainerID))
}

func (r *ReadTx) GetDeleteIntent(infraContainerID string) (DeleteIntent, error) {
	return getJSONValue[DeleteIntent](r, bucketDeleteIntents, infraContainerID)
}

func (r *ReadTx) ListDeleteIntents() (map[string]DeleteIntent, error) {
	return listJSONValues[DeleteIntent](r, bucketDeleteIntents)
}

func (w *WriteTx) PutDeleteIntent(infraContainerID string, intent DeleteIntent) error {
	key := normalizeID(infraContainerID)
	if key == "" {
		return invalidInput("infra container ID is empty", nil)
	}
	normalized, err := normalizeDeleteIntent(intent)
	if err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketDeleteIntents, key, normalized)
}

func (w *WriteTx) DeleteDeleteIntent(infraContainerID string) error {
	return deleteJSONValue(w.tx, bucketDeleteIntents, normalizeID(infraContainerID))
}

func deleteJSONValue(tx *bolt.Tx, bucketName []byte, key string) error {
	if key == "" {
		return invalidInput("ID is empty", nil)
	}
	bucket := tx.Bucket(bucketName)
	if bucket == nil {
		return corrupt(fmt.Sprintf("missing bucket %q", bucketName), nil)
	}
	if bucket.Get([]byte(key)) == nil {
		return fmt.Errorf("%w: bucket %q key %q", ErrNotFound, bucketName, key)
	}
	if err := bucket.Delete([]byte(key)); err != nil {
		return fmt.Errorf("deleting bucket %q key %q: %w", bucketName, key, err)
	}
	return nil
}
