package podnetwork

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_podnetworks.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkYAML)
}

func TestGetPodNetworks(t *testing.T) {
	_, err := GetPodNetworks()
	assert.NoError(t, err)
}
