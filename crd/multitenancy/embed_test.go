package multitenancy

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const mtpncFilename = "manifests/multitenancy.acn.azure.com_multitenantpodnetworkconfigs.yaml"

func TestEmbedMTPNC(t *testing.T) {
	b, err := os.ReadFile(mtpncFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, MultitenantPodNetworkConfigsYAML)
}

func TestGetMultitenantPodNetworkConfigs(t *testing.T) {
	_, err := GetMultitenantPodNetworkConfigs()
	assert.NoError(t, err)
}

const nodeinfoFilename = "manifests/multitenancy.acn.azure.com_nodeinfo.yaml"

func TestEmbedNodeInfo(t *testing.T) {
	b, err := os.ReadFile(nodeinfoFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, NodeInfoYAML)
}

func TestGetNodeInfo(t *testing.T) {
	_, err := GetNodeInfo()
	assert.NoError(t, err)
}

const podNetworkFilename = "manifests/multitenancy.acn.azure.com_podnetworks.yaml"

func TestEmbedPodNetwork(t *testing.T) {
	b, err := os.ReadFile(podNetworkFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkYAML)
}

func TestGetPodNetworks(t *testing.T) {
	_, err := GetPodNetworks()
	assert.NoError(t, err)
}

const podNetworkInstanceFilename = "manifests/multitenancy.acn.azure.com_podnetworkinstances.yaml"

func TestEmbedPodNetworkInstance(t *testing.T) {
	b, err := os.ReadFile(podNetworkInstanceFilename)
	assert.NoError(t, err)
	assert.Equal(t, b, PodNetworkInstanceYAML)
}

func TestGetPodNetworkInstances(t *testing.T) {
	_, err := GetPodNetworkInstances()
	assert.NoError(t, err)
}
