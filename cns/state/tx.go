// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type ReadTx struct {
	tx *bolt.Tx
}

type WriteTx struct {
	ReadTx
}

type serviceMetadata struct {
	OrchestratorType string    `json:"orchestratorType,omitempty"`
	NodeID           string    `json:"nodeID,omitempty"`
	Location         string    `json:"location,omitempty"`
	NetworkType      string    `json:"networkType,omitempty"`
	Initialized      bool      `json:"initialized"`
	TimeStamp        time.Time `json:"timestamp,omitempty"`
}

func (r *ReadTx) Metadata() (Metadata, error) {
	metaBucket := r.tx.Bucket(bucketMetadata)
	if metaBucket == nil {
		return Metadata{}, corrupt(fmt.Sprintf("missing bucket %q", bucketMetadata), nil)
	}
	schemaVersion, err := decodeUint32(metaBucket.Get(metaKeySchemaVersion))
	if err != nil {
		return Metadata{}, corrupt("invalid schema version", err)
	}
	authority := Authority(metaBucket.Get(metaKeyAuthority))
	if err = validateAuthority(authority); err != nil {
		return Metadata{}, corrupt("invalid authority", err)
	}
	generation, err := decodeUint64(metaBucket.Get(metaKeyGeneration))
	if err != nil {
		return Metadata{}, corrupt("invalid generation", err)
	}

	meta := Metadata{
		SchemaVersion: schemaVersion,
		Authority:     authority,
		Generation:    generation,
		BootID:        string(metaBucket.Get(metaKeyBootID)),
	}
	if data := metaBucket.Get(metaKeyService); data != nil {
		var serviceMeta serviceMetadata
		if err := decodeJSONValue(data, &serviceMeta); err != nil {
			return Metadata{}, corrupt(
				fmt.Sprintf("decoding bucket %q key %q", bucketMetadata, metaKeyService),
				err,
			)
		}
		meta.OrchestratorType = serviceMeta.OrchestratorType
		meta.NodeID = serviceMeta.NodeID
		meta.Location = serviceMeta.Location
		meta.NetworkType = serviceMeta.NetworkType
		meta.Initialized = serviceMeta.Initialized
		meta.TimeStamp = serviceMeta.TimeStamp
	}
	return meta, nil
}

// PutMetadata replaces the service-level metadata fields (OrchestratorType,
// NodeID, Location, NetworkType, Initialized, TimeStamp) with exactly the
// values in meta. This is a full replacement, not a merge: zero-valued
// fields in meta are written as zero and overwrite any previously stored
// non-zero value for that field. Callers that want to change only a subset
// of fields must first read the current record with Metadata(), mutate the
// returned struct, and pass it back in full (read-modify-write) -- see how
// boot-ID and authority-rollback updates are expected to be applied.
//
// Authority and BootID are the exception: each is written only when the
// corresponding meta field is non-empty, so passing a zero value for either
// preserves whatever is already stored rather than clearing it.
func (w *WriteTx) PutMetadata(meta Metadata) error {
	if meta.Authority != "" {
		if err := validateAuthority(meta.Authority); err != nil {
			return fmt.Errorf("writing state authority: %w", err)
		}
	}

	serviceMeta := serviceMetadata{
		OrchestratorType: meta.OrchestratorType,
		NodeID:           meta.NodeID,
		Location:         meta.Location,
		NetworkType:      meta.NetworkType,
		Initialized:      meta.Initialized,
		TimeStamp:        meta.TimeStamp,
	}
	data, err := json.Marshal(serviceMeta)
	if err != nil {
		return fmt.Errorf("encoding service metadata: %w", err)
	}
	bucket := w.tx.Bucket(bucketMetadata)
	if err := bucket.Put(metaKeyService, data); err != nil {
		return fmt.Errorf("writing service metadata: %w", err)
	}
	if meta.BootID != "" {
		if err := bucket.Put(metaKeyBootID, []byte(meta.BootID)); err != nil {
			return fmt.Errorf("writing boot ID: %w", err)
		}
	}
	if meta.Authority != "" {
		if err := bucket.Put(metaKeyAuthority, []byte(meta.Authority)); err != nil {
			return fmt.Errorf("writing state authority: %w", err)
		}
	}
	return nil
}
