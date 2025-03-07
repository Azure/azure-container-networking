package time

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDurationMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		have    Duration
		want    []byte
		wantErr bool
	}{
		{
			name:    "valid",
			have:    Duration{30 * time.Second},
			want:    []byte(`"30s"`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.have)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDurationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		have    []byte
		want    Duration
		wantErr bool
	}{
		{
			name:    "valid",
			have:    []byte(`"30s"`),
			want:    Duration{30 * time.Second},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &Duration{}
			err := json.Unmarshal(tt.have, got)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, *got)
		})
	}
}
