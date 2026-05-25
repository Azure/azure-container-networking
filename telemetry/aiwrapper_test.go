package telemetry

import (
	"testing"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/stretchr/testify/require"
)

func TestCreateAITelemetryHandle(t *testing.T) {
	tests := []struct {
		name             string
		aiConfig         aitelemetry.AIConfig
		connectionString string
		disableAll       bool
		disableMetric    bool
		disableTrace     bool
		wantErr          bool
	}{
		{
			name:          "disabled telemetry with empty aiconfig",
			aiConfig:      aitelemetry.AIConfig{},
			disableAll:    true,
			disableMetric: true,
			disableTrace:  true,
			wantErr:       true,
		},
		{
			name:             "telemetry handle created with connection string",
			aiConfig:         aitelemetry.AIConfig{},
			connectionString: "InstrumentationKey=abc;IngestionEndpoint=https://x/",
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			SetAIConnectionString(tt.connectionString)
			t.Cleanup(func() {
				SetAIConnectionString("")
			})

			tb := NewTelemetryBuffer(nil)
			err := tb.CreateAITelemetryHandle(tt.aiConfig, tt.disableAll, tt.disableMetric, tt.disableTrace)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
