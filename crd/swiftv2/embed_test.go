package swiftv2

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const podNetworkFilename = "manifests/acn.azure.com_podnetworks.yaml"

func TestEmbedPodNetwork(t *testing.T) {
	b, err := os.ReadFile(podNetworkFilename)
	require.NoError(t, err)
	assert.Equal(t, b, PodNetworkYAML)
}

func TestGetPodNetworks(t *testing.T) {
	_, err := GetPodNetworks()
	require.NoError(t, err)
}

const workloadNetworkConfigFilename = "manifests/acn.azure.com_workloadnetworkconfigs.yaml"

func TestEmbedWorkloadNetworkConfig(t *testing.T) {
	b, err := os.ReadFile(workloadNetworkConfigFilename)
	require.NoError(t, err)
	assert.Equal(t, b, WorkloadNetworkConfigYAML)
}

func TestGetWorkloadNetworkConfigs(t *testing.T) {
	_, err := GetWorkloadNetworkConfigs()
	require.NoError(t, err)
}
