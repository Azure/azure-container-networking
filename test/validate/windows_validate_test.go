package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHNSNetworkState(t *testing.T) {
	for _, input := range []string{
		`{"Name":"azure","State":1,"ManagementIP":"10.0.0.4","IPv6":true,"ManagementIPv6":"fd00::4"}`,
		`[{"Name":"azure","State":1,"ManagementIP":"10.0.0.4","IPv6":true,"ManagementIPv6":"fd00::4"}]`,
	} {
		t.Run(input, func(t *testing.T) {
			networks, err := hnsNetworkState([]byte(input))
			require.NoError(t, err)
			require.Equal(t, []HNSNetwork{{
				Name: "azure", State: 1, ManagementIP: "10.0.0.4",
				IPv6: true, ManagementIPv6: "fd00::4",
			}}, networks)
			require.NoError(t, validateHNSNetworks(networks))
		})
	}
	for _, input := range []string{"", "\x00", `{"Name":`, `"azure"`} {
		_, err := hnsNetworkState([]byte(input))
		require.Error(t, err)
	}
}

func TestValidateHNSNetworks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		err   string
	}{
		{"azure and ext", `[{"Name":"azure","State":1,"ManagementIP":"10.0.0.4"},{"Name":"ext","State":1,"ManagementIP":"10.0.0.5"}]`, ""},
		{"no networks", `[]`, "no HNS networks found"},
		{"null", `null`, "no HNS networks found"},
		{"no azure", `[{"Name":"ext","State":1,"ManagementIP":"10.0.0.4"}]`, "HNS 'azure' network does not exist"},
		{"wrong state", `{"Name":"azure","State":0,"ManagementIP":"10.0.0.4"}`, "windows HNS network state is not correct"},
		{"missing ipv4", `{"Name":"azure","State":1}`, "windows HNS network is missing ipv4 management IP"},
		{"missing ipv6", `{"Name":"azure","State":1,"ManagementIP":"10.0.0.4","IPv6":true}`, "windows HNS network is missing ipv6 management IP"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			networks, err := hnsNetworkState([]byte(test.input))
			require.NoError(t, err)
			err = validateHNSNetworks(networks)
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}
