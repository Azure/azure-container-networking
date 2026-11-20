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

func TestParseNetworkConfigCNSRequestTimeoutSeconds(t *testing.T) {
	config, err := ParseNetworkConfig([]byte(`{"cnsRequestTimeoutSeconds":30}`))
	require.NoError(t, err)
	require.Equal(t, 30, config.CNSRequestTimeoutSeconds)
}
