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
		name      string
		id        uint64
		valueType uint32
		queryErr  error
		wantCause error
		want      string
		wantErr   string
	}{
		{
			name:      "DWORD value conversion",
			id:        uint64(^uint32(0)),
			valueType: registry.DWORD,
			want:      "4294967295",
		},
		{
			name:      "non DWORD registry value",
			valueType: registry.QWORD,
			wantErr:   "platform: boot ID registry value is not a DWORD: 11",
			wantCause: errWindowsBootIDNotDWORD,
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
			query := func() (uint64, uint32, error) {
				calls++
				return test.id, test.valueType, test.queryErr
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
			if test.wantCause != nil {
				require.ErrorIs(t, err, test.wantCause)
			}
		})
	}
}
