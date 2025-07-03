package appinsights

import "testing"

func TestTelemetryConfiguration(t *testing.T) {
	testKey := "test"
	defaultEndpoint := "https://dc.services.visualstudio.com/v2/track"

	config := NewTelemetryConfiguration(testKey)

	if config.InstrumentationKey != testKey {
		t.Errorf("InstrumentationKey is %s, want %s", config.InstrumentationKey, testKey)
	}

	if config.EndpointUrl != defaultEndpoint {
		t.Errorf("EndpointUrl is %s, want %s", config.EndpointUrl, defaultEndpoint)
	}

	if config.Client != nil {
		t.Errorf("Client is not nil, want nil")
	}
}

func TestTelemetryConfigurationWithConnectionString(t *testing.T) {
	config := NewTelemetryConfigurationWithConnectionString(connection_string)

	if config.InstrumentationKey != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("InstrumentationKey is %s, want 00000000-0000-0000-0000-000000000000", config.InstrumentationKey)
	}

	if config.EndpointUrl != "https://ingestion.endpoint.com/v2/track" {
		t.Errorf("EndpointUrl is %s, want https://ingestion.endpoint.com/v2/track", config.EndpointUrl)
	}

	if config.Client != nil {
		t.Errorf("Client is not nil, want nil")
	}
}
