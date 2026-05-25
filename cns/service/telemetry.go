// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package main

import "github.com/Azure/azure-container-networking/cns/configuration"

// aiMode is the AppInsights telemetry init path selected from the config.
type aiMode int

const (
	// aiDefault uses the build-time-injected instrumentation key.
	aiDefault aiMode = iota
	// aiInstrumentationKey uses the instrumentation key from config.
	aiInstrumentationKey
	// aiConnectionString uses the connection string from config.
	aiConnectionString
)

// selectAIMode picks the AppInsights init path. Connection string takes
// precedence over instrumentation key. If neither is set, the build-time
// default is used.
func selectAIMode(ts configuration.TelemetrySettings) aiMode {
	switch {
	case ts.AppInsightsConnectionString != "":
		return aiConnectionString
	case ts.AppInsightsInstrumentationKey != "":
		return aiInstrumentationKey
	default:
		return aiDefault
	}
}
