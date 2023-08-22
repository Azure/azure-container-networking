package multitenancy

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const mtpncFilename = "manifests/acn.azure.com_multitenantpodnetworkconfigs.yaml"
const nodeinfoFilename = "manifests/acn.azure.com_nodeinfo.yaml"

func TestEmbedMTPNC(t *testing.T) {
	b, err := os.ReadFile(mtpncFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, MultitenantPodNetworkConfigsYAML)
}

func TestGetMultitenantPodNetworkConfigs(t *testing.T) {
	_, err := GetMultitenantPodNetworkConfigs()
	assert.NoError(t, err)
}

func TestEmbedNodeInfo(t *testing.T) {
	b, err := os.ReadFile(nodeinfoFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, NodeInfoYAML)
}

func TestGetNodeInfo(t *testing.T) {
	_, err := GetNodeInfo()
	assert.NoError(t, err)
}
