package clustersubnetstate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_clustersubnetstates.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, ClusterSubnetStatesYAML)
}

func TestGetClusterSubnetStates(t *testing.T) {
	_, err := GetClusterSubnetStates()
	assert.NoError(t, err)
}
