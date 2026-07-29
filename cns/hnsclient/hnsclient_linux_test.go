//go:build linux
// +build linux

package hnsclient

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
)

func TestLinuxHnsNetwork(t *testing.T) {
	c := NewClient()
	// these functions are unimplemented and should error on linux
	require.Error(t, c.CreateDefaultExtNetwork(""))
	require.Error(t, c.DeleteDefaultExtNetwork())
	require.Error(t, c.CreateHnsNetwork(cns.CreateHnsNetworkRequest{}))
	require.Error(t, c.DeleteHnsNetwork(""))
	// these no-op but return no error
	_, err := c.CreateHostNCApipaEndpoint("", cns.IPConfiguration{}, false, false, []cns.NetworkContainerRequestPolicies{})
	require.NoError(t, err)
	require.NoError(t, c.DeleteHostNCApipaEndpoint(""))
}
