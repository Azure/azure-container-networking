// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootIDReaderBoundary(t *testing.T) {
	t.Parallel()

	errRead := errors.New("read failure")
	tests := []struct {
		name    string
		data    string
		readErr error
		want    string
		wantErr string
	}{
		{
			name: "value",
			data: "550e8400-e29b-41d4-a716-446655440000",
			want: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "surrounding whitespace",
			data: "\t 550e8400-e29b-41d4-a716-446655440000 \r\n",
			want: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:    "empty value",
			data:    " \r\n\t",
			wantErr: "linux boot ID is empty",
		},
		{
			name:    "read failure",
			readErr: errRead,
			wantErr: "read linux boot ID: read failure",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			read := func(path string) ([]byte, error) {
				calls++
				require.Equal(t, linuxBootIDPath, path)
				return []byte(test.data), test.readErr
			}

			got, err := bootID(read)
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, got)
			}
			require.Equal(t, 1, calls)
			if test.readErr != nil {
				require.ErrorIs(t, err, test.readErr)
			}
		})
	}
}
