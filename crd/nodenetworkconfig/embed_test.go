package nodenetworkconfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_nodenetworkconfigs.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, NodeNetworkConfigsYAML)
}

func TestGetNodeNetworkConfigs(t *testing.T) {
	_, err := GetNodeNetworkConfigs()
	assert.NoError(t, err)
}
