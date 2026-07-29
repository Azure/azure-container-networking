//go:build linux
// +build linux

package hnsclient

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
)

func TestLinuxHnsNetwork(t *testing.T) {
	// these functions are unimplemented and should error on linux
	require.Error(t, CreateDefaultExtNetwork(""))
	require.Error(t, DeleteDefaultExtNetwork())
	require.Error(t, CreateHnsNetwork(cns.CreateHnsNetworkRequest{}))
	require.Error(t, DeleteHnsNetwork(""))
	// these no-op but return no error
	_, err := CreateHostNCApipaEndpoint("", cns.IPConfiguration{}, false, false, []cns.NetworkContainerRequestPolicies{})
	require.NoError(t, err)
	require.NoError(t, DeleteHostNCApipaEndpoint(""))
}
