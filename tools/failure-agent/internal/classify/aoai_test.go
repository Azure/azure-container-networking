package classify

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

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

type fakeCredential struct{}

func (fakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestNewAzureClientWithCredentialRequiresInputs(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		deployment string
		apiVersion string
		cred       azcore.TokenCredential
	}{
		{
			name:       "missing endpoint",
			deployment: "dep",
			apiVersion: "2024-10-21",
			cred:       fakeCredential{},
		},
		{
			name:       "missing deployment",
			endpoint:   "https://example.openai.azure.com",
			apiVersion: "2024-10-21",
			cred:       fakeCredential{},
		},
		{
			name:       "missing api version",
			endpoint:   "https://example.openai.azure.com",
			deployment: "dep",
			cred:       fakeCredential{},
		},
		{
			name:       "missing credential",
			endpoint:   "https://example.openai.azure.com",
			deployment: "dep",
			apiVersion: "2024-10-21",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAzureClientWithCredential(tt.endpoint, tt.deployment, tt.apiVersion, tt.cred); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestNewAzureClientWithCredential(t *testing.T) {
	got, err := NewAzureClientWithCredential("https://example.openai.azure.com", "dep", "2024-10-21", fakeCredential{})
	if err != nil {
		t.Fatalf("NewAzureClientWithCredential returned error: %v", err)
	}
	if got == nil {
		t.Fatal("NewAzureClientWithCredential returned nil client")
	}
}
