package middlewares

import "testing"

func TestBuildSubnetWidthPrimaryIP(t *testing.T) {
	tests := []struct {
		name          string
		primaryIPCIDR string
		subnetSpace   string
		want          string
	}{
		{
			name:          "happy path: replaces NC width with subnet width",
			primaryIPCIDR: "165.0.0.16/28",
			subnetSpace:   "165.0.0.0/20",
			want:          "165.0.0.16/20",
		},
		{
			name:          "bare IP without prefix takes subnet width",
			primaryIPCIDR: "165.0.0.16",
			subnetSpace:   "165.0.0.0/20",
			want:          "165.0.0.16/20",
		},
		{
			name:          "missing subnet space falls back to primary CIDR",
			primaryIPCIDR: "165.0.0.16/28",
			subnetSpace:   "",
			want:          "165.0.0.16/28",
		},
		{
			name:          "empty primary returns empty",
			primaryIPCIDR: "",
			subnetSpace:   "165.0.0.0/20",
			want:          "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSubnetWidthPrimaryIP(tc.primaryIPCIDR, tc.subnetSpace)
			if got != tc.want {
				t.Errorf("buildSubnetWidthPrimaryIP(%q, %q) = %q, want %q",
					tc.primaryIPCIDR, tc.subnetSpace, got, tc.want)
			}
		})
	}
}
