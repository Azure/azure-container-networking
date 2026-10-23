package cni

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNetworkConfigCNSSocketPath(t *testing.T) {
	config, err := ParseNetworkConfig([]byte(`{"cnsSocketPath":"/var/run/azure-cns/cns.sock"}`))
	require.NoError(t, err)
	require.Equal(t, "/var/run/azure-cns/cns.sock", config.CNSSocketPath)
}
