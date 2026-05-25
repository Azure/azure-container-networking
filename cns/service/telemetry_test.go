// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package main

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/stretchr/testify/require"
)

func TestSelectAIMode(t *testing.T) {
	tests := []struct {
		name string
		ts   configuration.TelemetrySettings
		want aiMode
	}{
		{
			name: "connection string only",
			ts: configuration.TelemetrySettings{
				AppInsightsConnectionString: "InstrumentationKey=abc;IngestionEndpoint=https://x/",
			},
			want: aiConnectionString,
		},
		{
			name: "instrumentation key only",
			ts: configuration.TelemetrySettings{
				AppInsightsInstrumentationKey: "abc",
			},
			want: aiInstrumentationKey,
		},
		{
			name: "both are set but connection string is selected",
			ts: configuration.TelemetrySettings{
				AppInsightsConnectionString:   "InstrumentationKey=abc;IngestionEndpoint=https://x/",
				AppInsightsInstrumentationKey: "abc",
			},
			want: aiConnectionString,
		},
		{
			name: "neither are set, use build-time default",
			ts:   configuration.TelemetrySettings{},
			want: aiDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, selectAIMode(tt.ts))
		})
	}
}
