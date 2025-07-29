package aitelemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const connectionString = "InstrumentationKey=0000-0000-0000-0000-0000;IngestionEndpoint=https://ingestion.endpoint.com/;LiveEndpoint=https://live.endpoint.com/;ApplicationId=1111-1111-1111-1111-1111"

func TestParseConnectionString(t *testing.T) {
	tests := []struct {
		name             string
		connectionString string
		want             *connectionVars
		wantErr          bool
	}{
		{
			name:             "Valid connection string and instrumentation key",
			connectionString: connectionString,
			want: &connectionVars{
				instrumentationKey: "0000-0000-0000-0000-0000",
				ingestionURL:       "https://ingestion.endpoint.com/v2.1/track",
			},
			wantErr: false,
		},
		{
			name:             "Invalid connection string format",
			connectionString: "InvalidConnectionString",
			want:             nil,
			wantErr:          true,
		},
		{
			name:             "Valid instrumentation key with missing ingestion endpoint",
			connectionString: "InstrumentationKey=0000-0000-0000-0000-0000;IngestionEndpoint=",
			want:             nil,
			wantErr:          true,
		},
		{
			name:             "Missing instrumentation key with valid ingestion endpoint",
			connectionString: "InstrumentationKey=;IngestionEndpoint=https://ingestion.endpoint.com/",
			want:             nil,
			wantErr:          true,
		},
		{
			name:             "Empty connection string",
			connectionString: "",
			want:             nil,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConnectionString(tt.connectionString)
			if tt.wantErr {
				require.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Expected no error but got one")
				require.NotNil(t, got, "Expected a non-nil result")
				require.Equal(t, tt.want.instrumentationKey, got.instrumentationKey, "Instrumentation Key does not match")
				require.Equal(t, tt.want.ingestionURL, got.ingestionURL, "Ingestion URL does not match")
			}
		})
	}
}
