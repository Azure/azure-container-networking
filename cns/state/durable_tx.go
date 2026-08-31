// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrNotFound        = errors.New("cns state: not found")
	ErrStaleGeneration = errors.New("cns state: stale generation")
	ErrInvalidInput    = errors.New("cns state: invalid input")
)

func (r *ReadTx) GetNetworkContainer(id string) (NetworkContainerRecord, error) {
	return getJSONValue[NetworkContainerRecord](r, bucketNetworkContainers, id)
}

func (r *ReadTx) ListNetworkContainers() (map[string]NetworkContainerRecord, error) {
	return listJSONValues[NetworkContainerRecord](r, bucketNetworkContainers)
}

func (w *WriteTx) PutNetworkContainer(record NetworkContainerRecord) error {
	normalized, err := normalizeNetworkContainer(record)
	if err != nil {
		return err
	}
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	candidate.NetworkContainers[normalized.ID] = normalized
	if err := validateInput(candidate); err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketNetworkContainers, normalized.ID, normalized)
}

func (w *WriteTx) DeleteNetworkContainer(id string) error {
	return w.deleteCandidateValue(
		bucketNetworkContainers,
		id,
		func(snapshot *Snapshot) { delete(snapshot.NetworkContainers, id) },
	)
}

func (r *ReadTx) GetIP(id string) (IPRecord, error) {
	return getJSONValue[IPRecord](r, bucketIPs, id)
}

func (r *ReadTx) ListIPs() (map[string]IPRecord, error) {
	return listJSONValues[IPRecord](r, bucketIPs)
}

func (w *WriteTx) PutIP(record IPRecord) error {
	if record.ID == "" {
		return invalidInput("IP ID is empty", nil)
	}
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	candidate.IPs[record.ID] = record
	if err := validateInput(candidate); err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketIPs, record.ID, record)
}

func (w *WriteTx) DeleteIP(id string) error {
	return w.deleteCandidateValue(bucketIPs, id, func(snapshot *Snapshot) { delete(snapshot.IPs, id) })
}

func (r *ReadTx) GetNetwork(name string) (NetworkRecord, error) {
	return getJSONValue[NetworkRecord](r, bucketNetworks, name)
}

func (r *ReadTx) ListNetworks() (map[string]NetworkRecord, error) {
	return listJSONValues[NetworkRecord](r, bucketNetworks)
}

func (w *WriteTx) PutNetwork(record NetworkRecord) error {
	if record.NetworkName == "" {
		return invalidInput("network name is empty", nil)
	}
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	candidate.Networks[record.NetworkName] = record
	if err := validateInput(candidate); err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketNetworks, record.NetworkName, record)
}

func (w *WriteTx) DeleteNetwork(name string) error {
	return w.deleteCandidateValue(bucketNetworks, name, func(snapshot *Snapshot) {
		delete(snapshot.Networks, name)
	})
}

func (r *ReadTx) GetOrchestratorContext(id string) ([]string, error) {
	value, err := getJSONValue[[]string](r, bucketOrchestratorContexts, id)
	return append([]string{}, value...), err
}

func (r *ReadTx) ListOrchestratorContexts() (map[string][]string, error) {
	values, err := listJSONValues[[]string](r, bucketOrchestratorContexts)
	if err != nil {
		return nil, err
	}
	for key, value := range values {
		values[key] = append([]string{}, value...)
	}
	return values, nil
}

func (w *WriteTx) PutOrchestratorContext(id string, ncIDs []string) error {
	if id == "" {
		return invalidInput("orchestrator context ID is empty", nil)
	}
	value := append([]string{}, ncIDs...)
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	candidate.OrchestratorContexts[id] = value
	if err := validateInput(candidate); err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketOrchestratorContexts, id, value)
}

func (w *WriteTx) DeleteOrchestratorContext(id string) error {
	return w.deleteCandidateValue(bucketOrchestratorContexts, id, func(snapshot *Snapshot) {
		delete(snapshot.OrchestratorContexts, id)
	})
}

func (r *ReadTx) GetPnPID(macAddress string) (string, error) {
	key, err := canonicalMAC(macAddress)
	if err != nil {
		return "", err
	}
	return getJSONValue[string](r, bucketPnPIDByMAC, key)
}

func (r *ReadTx) ListPnPIDs() (map[string]string, error) {
	return listJSONValues[string](r, bucketPnPIDByMAC)
}

func (w *WriteTx) PutPnPID(macAddress, pnpID string) error {
	key, err := canonicalMAC(macAddress)
	if err != nil {
		return err
	}
	if pnpID == "" {
		return invalidInput("PnP ID is empty", nil)
	}
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	candidate.PnPIDByMAC[key] = pnpID
	if err := validateInput(candidate); err != nil {
		return err
	}
	return putJSONValue(w.tx, bucketPnPIDByMAC, key, pnpID)
}

func (w *WriteTx) DeletePnPID(macAddress string) error {
	key, err := canonicalMAC(macAddress)
	if err != nil {
		return err
	}
	return w.deleteCandidateValue(bucketPnPIDByMAC, key, func(snapshot *Snapshot) {
		delete(snapshot.PnPIDByMAC, key)
	})
}

func (w *WriteTx) validSnapshot() (Snapshot, error) {
	snapshot, err := snapshotFromTx(w.ctx, &w.ReadTx)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (w *WriteTx) deleteCandidateValue(
	bucketName []byte,
	key string,
	remove func(*Snapshot),
) error {
	if key == "" {
		return invalidInput("ID is empty", nil)
	}
	bucket := w.tx.Bucket(bucketName)
	if bucket == nil {
		return corrupt(fmt.Sprintf("missing bucket %q", bucketName), nil)
	}
	if bucket.Get([]byte(key)) == nil {
		return fmt.Errorf("%w: bucket %q key %q", ErrNotFound, bucketName, key)
	}
	candidate, err := w.validSnapshot()
	if err != nil {
		return err
	}
	remove(&candidate)
	if err := validateInput(candidate); err != nil {
		return err
	}
	if err := bucket.Delete([]byte(key)); err != nil {
		return fmt.Errorf("deleting bucket %q key %q: %w", bucketName, key, err)
	}
	return nil
}

func getJSONValue[T any](r *ReadTx, bucketName []byte, key string) (T, error) {
	var zero T
	if key == "" {
		return zero, invalidInput("ID is empty", nil)
	}
	bucket := r.tx.Bucket(bucketName)
	if bucket == nil {
		return zero, corrupt(fmt.Sprintf("missing bucket %q", bucketName), nil)
	}
	data := bucket.Get([]byte(key))
	if data == nil {
		return zero, fmt.Errorf("%w: bucket %q key %q", ErrNotFound, bucketName, key)
	}
	var value T
	if err := decodeJSONValue(data, &value); err != nil {
		return zero, corruptValue(bucketName, key, "decoding value", err)
	}
	return value, nil
}

func listJSONValues[T any](r *ReadTx, bucketName []byte) (map[string]T, error) {
	values := map[string]T{}
	if err := decodeBucket(r.ctx, r, bucketName, values); err != nil {
		return nil, err
	}
	return values, nil
}

func putJSONValue[T any](tx *bolt.Tx, bucketName []byte, key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return invalidInput(fmt.Sprintf("encoding bucket %q key %q", bucketName, key), err)
	}
	bucket := tx.Bucket(bucketName)
	if bucket == nil {
		return corrupt(fmt.Sprintf("missing bucket %q", bucketName), nil)
	}
	if err := bucket.Put([]byte(key), data); err != nil {
		return fmt.Errorf("writing bucket %q key %q: %w", bucketName, key, err)
	}
	return nil
}

func normalizeNetworkContainer(record NetworkContainerRecord) (NetworkContainerRecord, error) {
	switch {
	case record.ID == "":
		return NetworkContainerRecord{}, invalidInput("network container ID is empty", nil)
	case record.Request.NetworkContainerid != record.ID:
		return NetworkContainerRecord{}, invalidInput(
			fmt.Sprintf(
				"network container ID %q does not match request ID %q",
				record.ID,
				record.Request.NetworkContainerid,
			),
			nil,
		)
	}
	return NewNetworkContainerRecord(
		record.ID,
		record.VMVersion,
		record.HostVersion,
		record.VFPUpdateComplete,
		record.Request,
	), nil
}

func canonicalMAC(value string) (string, error) {
	if value == "" {
		return "", invalidInput("MAC address is empty", nil)
	}
	mac, err := net.ParseMAC(value)
	if err != nil {
		return "", invalidInput(fmt.Sprintf("invalid MAC address %q", value), err)
	}
	return mac.String(), nil
}

func validateInput(snapshot Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: candidate state: %w", ErrInvalidInput, err)
	}
	return nil
}

func invalidInput(detail string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrInvalidInput, detail)
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalidInput, detail, err)
}
