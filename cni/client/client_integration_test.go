// +build linux
// +build integration

package client

import (
	"io"
	"log"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	ver "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"
)

const (
	stateFilePath = "/var/run/azure-vnet.json"
)

func setTestFile() error {
	var err error
	// copy test state file to /var/run/azure-vnet.json
	in, err := os.Open("./testresources/azure-vnet-test.json")
	if err != nil {
		return err
	}

	out, err := os.Create(stateFilePath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func cleanupTestFile() {
	err := os.Remove(stateFilePath)
	if err != nil {
		log.Print(err)
	}
}

func TestMain(m *testing.M) {
	testutils.RequireRootforTestMain()
	err := setTestFile()
	if err != nil {
		log.Panic(err)
	}

	os.Exit(m.Run())
	cleanupTestFile()
}

// todo: enable this test in CI, requires built azure vnet
func TestGetStateFromAzureCNI(t *testing.T) {
	c := New(exec.New())
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

func TestGetVersion(t *testing.T) {
	c := New(exec.New())
	version, err := c.GetVersion()
	require.NoError(t, err)

	expectedVersion, err := ver.NewVersion("v1.4.0-2-g984c5a5e-dirty")
	require.NoError(t, err)

	require.Equal(t, expectedVersion, version)
}

func TestGetStateWithEmptyStateFile(t *testing.T) {
	defer func() {
		cleanupTestFile()
		setTestFile()
	}()

	c := New(exec.New())

	err := os.Remove(stateFilePath)
	require.NoError(t, err)

	out, err := os.Create(stateFilePath)
	require.NoError(t, err)
	out.Close()

	state, err := c.GetEndpointState()
	require.NoError(t, err)
	require.Exactly(t, 0, len(state.ContainerInterfaces))

}
