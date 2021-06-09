// +build linux
// +build integration

package client

import (
	"io"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"
)

// todo: enable this test in CI, requires built azure vnet
func TestGetStateFromAzureCNI(t *testing.T) {
	testutils.RequireRootforTest(t)

	// copy test state file to /var/run/azure-vnet.json
	in, err := os.Open("./testresources/azure-vnet-test.json")
	require.NoError(t, err)

	defer in.Close()

	out, err := os.Create("/var/run/azure-vnet.json")
	require.NoError(t, err)

	defer func() {
		out.Close()
		err := os.Remove("/var/run/azure-vnet.json")
		require.NoError(t, err)
	}()

	_, err = io.Copy(out, in)
	require.NoError(t, err)

	out.Close()

	realexec := exec.New()
	c := NewCNIClient(realexec)
	state, err := c.GetEndpointState()
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			"3f813b02-eth0": testGetPodNetworkInterfaceInfo("3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system", "3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "10.241.0.17/16"),
			"6e688597-eth0": testGetPodNetworkInterfaceInfo("6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system", "6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "10.241.0.13/16"),
		},
	}

	require.Exactly(t, res, state)
}
