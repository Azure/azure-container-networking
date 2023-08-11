package multitenantpodnetworkconfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_multitenantpodnetworkconfigs.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, MultitenantPodNetworkConfigsYAML)
}

func TestGetMultitenantPodNetworkConfigs(t *testing.T) {
	_, err := GetMultitenantPodNetworkConfigs()
	assert.NoError(t, err)
}
