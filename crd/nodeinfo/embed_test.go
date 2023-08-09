package nodeinfo

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_nodesinfo.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, NodesInfoYAML)
}

func TestGetNodesInfo(t *testing.T) {
	_, err := GetNodesInfo()
	assert.NoError(t, err)
}
