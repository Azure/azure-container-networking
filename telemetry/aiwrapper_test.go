package telemetry

import (
	"testing"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/stretchr/testify/require"
)

func TestCreateAITelemetryHandle(t *testing.T) {
	tests := []struct {
		name          string
		aiConfig      aitelemetry.AIConfig
		disableAll    bool
		disableMetric bool
		disableTrace  bool
		wantErr       bool
	}{
		{
			name:          "disable telemetry",
			aiConfig:      aitelemetry.AIConfig{},
			disableAll:    false,
			disableMetric: true,
			disableTrace:  true,
			wantErr:       true,
		},
		{
			name:          "empty aiconfig",
			aiConfig:      aitelemetry.AIConfig{},
			disableAll:    true,
			disableMetric: true,
			disableTrace:  true,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
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
