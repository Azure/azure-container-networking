package appinsights

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const connection_string = "InstrumentationKey=00000000-0000-0000-0000-000000000000;IngestionEndpoint=https://ingestion.endpoint.com/;LiveEndpoint=https://live.endpoint.com/;ApplicationId=11111111-1111-1111-1111-111111111111"

func TestConnectionStringParser(t *testing.T) {
	tests := []struct {
		name    string
		cString string
		want    *ConnectionParams
		wantErr bool
	}{
		{
			name:    "Valid connection string and instrumentation key",
			cString: connection_string,
			want: &ConnectionParams{
				InstrumentationKey: "00000000-0000-0000-0000-000000000000",
				IngestionEndpoint:  "https://ingestion.endpoint.com/v2/track",
			},
			wantErr: false,
		},
		{
			name:    "Invalid connection string format",
			cString: "Invalid connection string",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Valid instrumentation key with missing ingestion endpoint",
			cString: "InstrumentationKey=00000000-0000-0000-0000-000000000000;IngestionEndpoint=",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Missing instrumentation key with valid ingestion endpoint",
			cString: "InstrumentationKey=;IngestionEndpoint=https://ingestion.endpoint.com/v2/track",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Empty connection string",
			cString: "",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConnectionString(tt.cString)
			if tt.wantErr {
				require.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Expected no error but got one")
				require.NotNil(t, got, "Expected a non-nil result")
				require.Equal(t, tt.want.InstrumentationKey, got.InstrumentationKey, "Instrumentation Key does not match")
				require.Equal(t, tt.want.IngestionEndpoint, got.IngestionEndpoint, "Ingestion Endpoint does not match")
			}
		})
	}
}
