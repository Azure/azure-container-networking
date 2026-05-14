package validate

import (
	"testing"
)

func TestParseCiliumIngressIPs(t *testing.T) {
	tests := []struct {
		name     string
		output   []byte
		expected []string
	}{
		{
			name:     "single IP",
			output:   []byte("10.224.0.55\n"),
			expected: []string{"10.224.0.55"},
		},
		{
			name:     "multiple IPs",
			output:   []byte("10.224.0.55\n10.224.0.60\n"),
			expected: []string{"10.224.0.55", "10.224.0.60"},
		},
		{
			name:     "empty output",
			output:   []byte(""),
			expected: nil,
		},
		{
			name:     "whitespace only",
			output:   []byte("   \n  \n"),
			expected: nil,
		},
		{
			name:     "trailing newlines and spaces",
			output:   []byte("  10.224.0.55  \n  10.224.0.60  \n\n"),
			expected: []string{"10.224.0.55", "10.224.0.60"},
		},
		{
			name:     "single IP no trailing newline",
			output:   []byte("10.0.0.1"),
			expected: []string{"10.0.0.1"},
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
