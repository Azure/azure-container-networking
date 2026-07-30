package classify

import "testing"

func TestNewAzureClientRequiresInputs(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		deployment string
		apiVersion string
		apiKey     string
	}{
		{
			name:       "missing endpoint",
			deployment: "dep",
			apiVersion: "2024-10-21",
			apiKey:     "key",
		},
		{
			name:       "missing deployment",
			endpoint:   "https://example.openai.azure.com",
			apiVersion: "2024-10-21",
			apiKey:     "key",
		},
		{
			name:       "missing api version",
			endpoint:   "https://example.openai.azure.com",
			deployment: "dep",
			apiKey:     "key",
		},
		{
			name:       "missing api key",
			endpoint:   "https://example.openai.azure.com",
			deployment: "dep",
			apiVersion: "2024-10-21",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAzureClient(tt.endpoint, tt.deployment, tt.apiVersion, tt.apiKey); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestNewAzureClientWithAPIKey(t *testing.T) {
	got, err := NewAzureClient("https://example.openai.azure.com", "dep", "2024-10-21", "key")
	if err != nil {
		t.Fatalf("NewAzureClient returned error: %v", err)
	}
	if got == nil {
		t.Fatal("NewAzureClient returned nil client")
	}
}
