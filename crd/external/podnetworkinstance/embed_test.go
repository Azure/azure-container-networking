package podnetworkinstance

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_podnetworkinstances.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkInstanceYAML)
}

func TestGetPodNetworkInstances(t *testing.T) {
	_, err := GetPodNetworkInstances()
	assert.NoError(t, err)
}
