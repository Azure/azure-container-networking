package validate

import (
	"testing"
)

func TestHNSStateFileIPs(t *testing.T) {
	tests := []struct {
		name      string
		result    string
		wantIPs   map[string]string
		wantError bool
	}{
		{
			name:      "empty output",
			result:    "",
			wantError: true,
		},
		{
			name:      "whitespace output",
			result:    " \r\n\t",
			wantError: true,
		},
		{
			name:   "single endpoint",
			result: `{"MacAddress":"00-11-22-33-44-55","IPAddress":"10.0.0.4","IPv6Address":"fd00::4"}`,
			wantIPs: map[string]string{
				"10.0.0.4": "00-11-22-33-44-55",
				"fd00::4":  "00-11-22-33-44-55",
			},
		},
		{
			name: "endpoint list excludes remote endpoints",
			result: `[
				{"MacAddress":"00-11-22-33-44-55","IPAddress":"10.0.0.4"},
				{"MacAddress":"00-11-22-33-44-66","IPAddress":"10.0.0.5","IsRemoteEndpoint":true}
			]`,
			wantIPs: map[string]string{
				"10.0.0.4": "00-11-22-33-44-55",
			},
		},
		{
			name:      "malformed output",
			result:    "not-json",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hnsStateFileIPs([]byte(tt.result))
			if tt.wantError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("hnsStateFileIPs() error = %v", err)
			}
			if len(got) != len(tt.wantIPs) {
				t.Fatalf("hnsStateFileIPs() returned %v, want %v", got, tt.wantIPs)
			}
			for ip, mac := range tt.wantIPs {
				if got[ip] != mac {
					t.Errorf("hnsStateFileIPs()[%q] = %q, want %q", ip, got[ip], mac)
				}
			}
		})
	}
}
