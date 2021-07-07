// +build unit

package client

import (
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	ver "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

func TestGetState(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"./azure-vnet"}, Stdout: `{"ContainerInterfaces":{"3f813b02-eth0":{"PodName":"metrics-server-77c8679d7d-6ksdh","PodNamespace":"kube-system","PodEndpointID":"3f813b02-eth0","ContainerID":"3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46","IPAddresses":[{"IP":"10.241.0.17","Mask":"//8AAA=="}]},"6e688597-eth0":{"PodName":"tunnelfront-5d96f9b987-65xbn","PodNamespace":"kube-system","PodEndpointID":"6e688597-eth0","ContainerID":"6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed","IPAddresses":[{"IP":"10.241.0.13","Mask":"//8AAA=="}]}}}`},
	}

	fakeexec, _ := testutils.GetFakeExecWithScripts(calls)

	c := New(fakeexec)
	state, err := c.GetEndpointState()
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			"3f813b02-eth0": testGetPodNetworkInterfaceInfo("3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system", "3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "10.241.0.17/16"),
			"6e688597-eth0": testGetPodNetworkInterfaceInfo("6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system", "6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "10.241.0.13/16"),
		},
	}

	require.Equal(t, res, state)
}

func TestGetVersion(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"./azure-vnet", "-v"}, Stdout: `Azure CNI Version v1.4.0-2-g984c5a5e-dirty`},
	}

	fakeexec, _ := testutils.GetFakeExecWithScripts(calls)

	c := New(fakeexec)
	version, err := c.GetVersion()
	require.NoError(t, err)

	expectedVersion, err := ver.NewVersion("v1.4.0-2-g984c5a5e-dirty")
	require.NoError(t, err)

	require.Equal(t, expectedVersion, version)
}
