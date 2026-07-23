//go:build windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootIDNativeQueryBoundary(t *testing.T) {
	t.Parallel()

	errRegistry := errors.New("registry failure")
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
			queryErr: errRegistry,
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
