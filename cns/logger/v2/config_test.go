package logger

import (
	"encoding/json"
	"testing"

	cores "github.com/Azure/azure-container-networking/cns/logger/v2/cores"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		have    []byte
		want    *Config
		wantErr bool
	}{
		{
			name: "valid",
			have: []byte(`{"level":"info"}`),
			want: &Config{
				Level: "info",
				level: 0,
			},
		},
		{
			name:    "invalid level",
			have:    []byte(`{"level":"invalid"}`),
			wantErr: true,
		},
		{
			name: "valid with file",
			have: []byte(`{"level":"info","file":{"filepath":"/k/azurecns/azure-cns.log"}}`),
			want: &Config{
				Level: "info",
				level: 0,
				File: &cores.FileConfig{
					Filepath: "/k/azurecns/azure-cns.log",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			err := json.Unmarshal(tt.have, c)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, c)
		})
	}
}
