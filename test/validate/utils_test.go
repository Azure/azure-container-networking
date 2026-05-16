package validate

import (
	"encoding/json"
	"testing"
)

func makeCiliumEndpointJSON(endpoints []CiliumEndpointStatus) []byte {
	b, _ := json.Marshal(endpoints)
	return b
}

func TestParseCiliumIngressIPs(t *testing.T) {
	tests := []struct {
		name     string
		output   []byte
		expected []string
	}{
		{
			name: "single IP",
			output: makeCiliumEndpointJSON([]CiliumEndpointStatus{
				{Status: NetworkingStatus{
					Labels:     EndpointLabels{SecurityRelevant: []string{"reserved:ingress"}},
					Networking: NetworkingAddressing{Addresses: []Address{{IPv4: "10.224.0.55"}}},
				}},
			}),
			expected: []string{"10.224.0.55"},
		},
		{
			name: "multiple IPs",
			output: makeCiliumEndpointJSON([]CiliumEndpointStatus{
				{Status: NetworkingStatus{
					Labels:     EndpointLabels{SecurityRelevant: []string{"reserved:ingress"}},
					Networking: NetworkingAddressing{Addresses: []Address{{IPv4: "10.224.0.55"}, {IPv4: "10.224.0.60"}}},
				}},
			}),
			expected: []string{"10.224.0.55", "10.224.0.60"},
		},
		{
			name:     "empty output",
			output:   []byte(""),
			expected: nil,
		},
		{
			name:     "empty JSON array",
			output:   []byte("[]"),
			expected: nil,
		},
		{
			name: "non-ingress endpoint ignored",
			output: makeCiliumEndpointJSON([]CiliumEndpointStatus{
				{Status: NetworkingStatus{
					Labels:     EndpointLabels{SecurityRelevant: []string{"reserved:host"}},
					Networking: NetworkingAddressing{Addresses: []Address{{IPv4: "10.224.0.55"}}},
				}},
			}),
			expected: nil,
		},
		{
			name: "dualstack IPs",
			output: makeCiliumEndpointJSON([]CiliumEndpointStatus{
				{Status: NetworkingStatus{
					Labels:     EndpointLabels{SecurityRelevant: []string{"reserved:ingress"}},
					Networking: NetworkingAddressing{Addresses: []Address{{IPv4: "10.224.0.55", IPv6: "fd00::1"}}},
				}},
			}),
			expected: []string{"10.224.0.55", "fd00::1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCiliumIngressIPs(tt.output)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d IPs, got %d: %v", len(tt.expected), len(got), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("IP[%d]: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}
