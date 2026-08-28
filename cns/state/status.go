// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

const BackendBolt = "bbolt"

type InvariantStatus string

const (
	InvariantHealthy InvariantStatus = "healthy"
	InvariantFailed  InvariantStatus = "failed"
)

type RecordCounts struct {
	NetworkContainers int `json:"networkContainers"`
	IPs               int `json:"ips"`
	Networks          int `json:"networks"`
	Endpoints         int `json:"endpoints"`
	Assignments       int `json:"assignments"`
	Owners            int `json:"owners"`
	DeleteIntents     int `json:"deleteIntents"`
}

type Status struct {
	Backend          string          `json:"backend"`
	Authority        Authority       `json:"authority,omitempty"`
	SchemaVersion    uint32          `json:"schemaVersion,omitempty"`
	Generation       uint64          `json:"generation,omitempty"`
	BootPresent      bool            `json:"bootPresent"`
	StoragePresent   bool            `json:"storagePresent"`
	DatabaseBytes    int64           `json:"databaseBytes"`
	Records          RecordCounts    `json:"records"`
	InvariantStatus  InvariantStatus `json:"invariantStatus"`
	FailedInvariant  InvariantName   `json:"failedInvariant,omitempty"`
	LegacyImported   bool            `json:"legacyImported"`
	RollbackExported bool            `json:"rollbackExported"`
}

func (s Status) RecordCount(recordType RecordType) int {
	switch recordType {
	case RecordNetworkContainer:
		return s.Records.NetworkContainers
	case RecordIP:
		return s.Records.IPs
	case RecordNetwork:
		return s.Records.Networks
	case RecordEndpoint:
		return s.Records.Endpoints
	case RecordAssignment:
		return s.Records.Assignments
	case RecordOwner:
		return s.Records.Owners
	case RecordDeleteIntent:
		return s.Records.DeleteIntents
	default:
		return 0
	}
}

func (s *DB) Status(ctx context.Context) (Status, error) {
	status := Status{
		Backend:         BackendBolt,
		InvariantStatus: InvariantHealthy,
	}
	err := s.View(ctx, func(tx *ReadTx) error {
		var err error
		status, err = statusFromTx(ctx, tx)
		return err
	})
	if err != nil {
		return Status{}, fmt.Errorf("reading persistent state status: %w", err)
	}
	if status.InvariantStatus == InvariantFailed {
		_ = s.metrics.ObserveInvariant(status.FailedInvariant)
	}
	return status, nil
}

func (s *DB) RefreshMetrics(ctx context.Context) (Status, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	if err := s.metrics.Refresh(status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (s *DB) summary(ctx context.Context) (Status, error) {
	var status Status
	err := s.View(ctx, func(tx *ReadTx) error {
		metadata, err := tx.Metadata()
		if err != nil {
			return err
		}
		status = Status{
			Backend:          BackendBolt,
			Authority:        metadata.Authority,
			SchemaVersion:    metadata.SchemaVersion,
			Generation:       metadata.Generation,
			BootPresent:      metadata.BootID != "",
			StoragePresent:   true,
			DatabaseBytes:    tx.tx.Size(),
			InvariantStatus:  InvariantHealthy,
			FailedInvariant:  InvariantNone,
			LegacyImported:   metadata.LegacyImportComplete,
			RollbackExported: metadata.RollbackExportComplete,
			Records: RecordCounts{
				NetworkContainers: bucketKeyCount(tx.tx, bucketNetworkContainers),
				IPs:               bucketKeyCount(tx.tx, bucketIPs),
				Networks:          bucketKeyCount(tx.tx, bucketNetworks),
				Endpoints:         bucketKeyCount(tx.tx, bucketEndpoints),
				Assignments:       bucketKeyCount(tx.tx, bucketAssignments),
				Owners:            bucketKeyCount(tx.tx, bucketIPOwners),
				DeleteIntents:     bucketKeyCount(tx.tx, bucketDeleteIntents),
			},
		}
		return nil
	})
	if err != nil {
		return Status{}, fmt.Errorf("reading persistent state summary: %w", err)
	}
	return status, nil
}

func bucketKeyCount(tx *bolt.Tx, name []byte) int {
	bucket := tx.Bucket(name)
	if bucket == nil {
		return 0
	}
	return bucket.Stats().KeyN
}

func statusFromTx(ctx context.Context, tx *ReadTx) (Status, error) {
	status := Status{
		Backend:         BackendBolt,
		StoragePresent:  true,
		DatabaseBytes:   tx.tx.Size(),
		InvariantStatus: InvariantHealthy,
		FailedInvariant: InvariantNone,
	}
	if name := metadataInvariant(tx.tx); name != InvariantNone {
		status.InvariantStatus = InvariantFailed
		status.FailedInvariant = name
		return status, nil
	}

	snapshot, err := snapshotFromTx(ctx, tx)
	if err != nil {
		if name := boundedInvariant(err); name != InvariantNone {
			status.InvariantStatus = InvariantFailed
			status.FailedInvariant = name
			return status, nil
		}
		return Status{}, err
	}
	status.Authority = snapshot.Metadata.Authority
	status.SchemaVersion = snapshot.Metadata.SchemaVersion
	status.Generation = snapshot.Metadata.Generation
	status.BootPresent = snapshot.Metadata.BootID != ""
	status.LegacyImported = snapshot.Metadata.LegacyImportComplete
	status.RollbackExported = snapshot.Metadata.RollbackExportComplete
	status.Records = RecordCounts{
		NetworkContainers: len(snapshot.NetworkContainers),
		IPs:               len(snapshot.IPs),
		Networks:          len(snapshot.Networks),
		Endpoints:         len(snapshot.Endpoints),
		Assignments:       len(snapshot.Assignments),
		Owners:            len(snapshot.IPOwners),
		DeleteIntents:     len(snapshot.DeleteIntents),
	}
	if err := snapshot.Validate(); err != nil {
		status.InvariantStatus = InvariantFailed
		status.FailedInvariant = boundedInvariant(err)
	}
	return status, nil
}

func metadataInvariant(tx *bolt.Tx) InvariantName {
	meta := tx.Bucket(bucketMetadata)
	if meta == nil {
		return InvariantStructural
	}
	version, err := decodeUint32(meta.Get(metaKeySchemaVersion))
	if err != nil {
		return InvariantStructural
	}
	if version != SchemaVersion {
		return InvariantSchema
	}
	if err := validateAuthority(Authority(meta.Get(metaKeyAuthority))); err != nil {
		return InvariantAuthority
	}
	return InvariantNone
}

func boundedInvariant(err error) InvariantName {
	switch {
	case errors.Is(err, ErrSchemaMismatch):
		return InvariantSchema
	case errors.Is(err, ErrCorrupt), errors.Is(err, ErrInconsistentState):
		return InvariantStructural
	default:
		return InvariantStructural
	}
}
