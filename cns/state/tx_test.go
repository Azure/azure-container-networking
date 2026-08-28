// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestMetadataRoundTripPreservesCoreKeys(t *testing.T) {
	db, _ := openTestDB(t)
	timestamp := time.Date(2026, time.July, 23, 22, 0, 0, 0, time.UTC)
	input := Metadata{
		SchemaVersion:    99,
		Authority:        AuthorityJSON,
		Generation:       99,
		BootID:           "boot-1",
		OrchestratorType: "KubernetesCRD",
		NodeID:           testNodeID,
		Location:         "westus2",
		NetworkType:      "azure",
		Initialized:      true,
		TimeStamp:        timestamp,
	}
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(input)
	}))

	got := readMetadata(t, db)
	assert.Equal(t, SchemaVersion, got.SchemaVersion)
	assert.Equal(t, AuthorityJSON, got.Authority)
	assert.Equal(t, uint64(1), got.Generation)
	assert.Equal(t, input.BootID, got.BootID)
	assert.Equal(t, input.OrchestratorType, got.OrchestratorType)
	assert.Equal(t, input.NodeID, got.NodeID)
	assert.Equal(t, input.Location, got.Location)
	assert.Equal(t, input.NetworkType, got.NetworkType)
	assert.Equal(t, input.Initialized, got.Initialized)
	assert.Equal(t, input.TimeStamp, got.TimeStamp)

	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		var stored map[string]any
		require.NoError(t, json.Unmarshal(tx.Bucket(bucketMetadata).Get(metaKeyService), &stored))
		assert.NotContains(t, stored, "schemaVersion")
		assert.NotContains(t, stored, "authority")
		assert.NotContains(t, stored, "generation")
		assert.NotContains(t, stored, "bootID")
		return nil
	}))
}

func TestPutMetadataRejectsUnknownAuthority(t *testing.T) {
	db, _ := openTestDB(t)
	err := db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{Authority: Authority("other"), NodeID: testNodeID})
	})
	require.Error(t, err)
	metadata := readMetadata(t, db)
	assert.Equal(t, AuthorityBolt, metadata.Authority)
	assert.Empty(t, metadata.NodeID)
	assert.Zero(t, metadata.Generation)
}

func TestIntegerEncoding(t *testing.T) {
	t.Run("uint32 round trips", func(t *testing.T) {
		for _, value := range []uint32{0, 1, math.MaxUint32} {
			t.Run(strconv.FormatUint(uint64(value), 10), func(t *testing.T) {
				got, err := decodeUint32(encodeUint32(value))
				require.NoError(t, err)
				assert.Equal(t, value, got)
			})
		}
	})

	t.Run("uint64 round trips", func(t *testing.T) {
		for _, value := range []uint64{0, 1, math.MaxUint64} {
			t.Run(strconv.FormatUint(value, 10), func(t *testing.T) {
				got, err := decodeUint64(encodeUint64(value))
				require.NoError(t, err)
				assert.Equal(t, value, got)
			})
		}
	})

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "uint32 empty",
			run: func() error {
				_, err := decodeUint32(nil)
				return err
			},
		},
		{
			name: "uint32 short",
			run: func() error {
				_, err := decodeUint32([]byte{1})
				return err
			},
		},
		{
			name: "uint32 long",
			run: func() error {
				_, err := decodeUint32(make([]byte, 5))
				return err
			},
		},
		{
			name: "uint64 empty",
			run: func() error {
				_, err := decodeUint64(nil)
				return err
			},
		},
		{
			name: "uint64 short",
			run: func() error {
				_, err := decodeUint64([]byte{1})
				return err
			},
		},
		{
			name: "uint64 long",
			run: func() error {
				_, err := decodeUint64(make([]byte, 9))
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.run())
		})
	}
}
