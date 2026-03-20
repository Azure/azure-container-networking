//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkContainerSubnetAddressSpaceV6JSONRoundTrip(t *testing.T) {
	nc := NetworkContainer{
		ID:                   "test-nc-id",
		PrimaryIP:            "10.0.0.1",
		PrimaryIPV6:          "fd12:3456:789a::1",
		SubnetAddressSpace:   "10.0.0.0/24",
		SubnetAddressSpaceV6: "fd12:3456:789a::/48",
		DefaultGateway:       "10.0.0.1",
		DefaultGatewayV6:     "fd12:3456:789a::1",
		SubnetName:           "testsubnet",
		Version:              1,
	}

	data, err := json.Marshal(nc)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"subnetAddressSpaceV6":"fd12:3456:789a::/48"`)

	var decoded NetworkContainer
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, nc.SubnetAddressSpaceV6, decoded.SubnetAddressSpaceV6)
	assert.Equal(t, nc.SubnetAddressSpace, decoded.SubnetAddressSpace)
	assert.Equal(t, nc.PrimaryIPV6, decoded.PrimaryIPV6)
	assert.Equal(t, nc.DefaultGatewayV6, decoded.DefaultGatewayV6)
}

func TestNetworkContainerSubnetAddressSpaceV6OmitEmpty(t *testing.T) {
	nc := NetworkContainer{
		SubnetAddressSpace: "10.0.0.0/24",
		Version:            1,
	}

	data, err := json.Marshal(nc)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "subnetAddressSpaceV6")
}
