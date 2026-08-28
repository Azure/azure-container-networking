//go:build windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

var errBootIDRegistry = errors.New("registry failure")

func TestBootIDNativeQueryBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       uint64
		queryErr error
		want     string
		wantErr  string
	}{
		{
			name: "DWORD value conversion",
			id:   uint64(^uint32(0)),
			want: "4294967295",
		},
		{
			name:     "registry failure",
			queryErr: errBootIDRegistry,
			wantErr:  "query windows boot ID: registry failure",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			query := func() (uint64, error) {
				calls++
				return test.id, test.queryErr
			}

			got, err := bootID(query)
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, got)
			}
			require.Equal(t, 1, calls)
			if test.queryErr != nil {
				require.ErrorIs(t, err, test.queryErr)
			}
		})
	}
}

func TestValidateBootIDRegistryValue(t *testing.T) {
	t.Parallel()

	id, err := validateBootIDRegistryValue(42, registry.DWORD)
	require.NoError(t, err)
	require.Equal(t, uint64(42), id)

	id, err = validateBootIDRegistryValue(42, registry.QWORD)
	require.EqualError(t, err, "unexpected boot ID registry type 11")
	require.Zero(t, id)
}
