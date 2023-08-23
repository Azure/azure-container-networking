package external

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filenamePodNetwork = "manifests/public.acn.azure.com_podnetworks.yaml"

func TestEmbedPodNetwork(t *testing.T) {
	b, err := os.ReadFile(filenamePodNetwork)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkYAML)
}

func TestGetPodNetworks(t *testing.T) {
	_, err := GetPodNetworks()
	assert.NoError(t, err)
}

const filenamePodNetworkInstance = "manifests/public.acn.azure.com_podnetworkinstances.yaml"

func TestEmbedPodNetworkInstance(t *testing.T) {
	b, err := os.ReadFile(filenamePodNetworkInstance)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkInstanceYAML)
}

func TestGetPodNetworkInstances(t *testing.T) {
	_, err := GetPodNetworkInstances()
	assert.NoError(t, err)
}
