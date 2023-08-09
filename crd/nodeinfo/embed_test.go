package nodeinfo

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const filename = "manifests/acn.azure.com_nodeinfo.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, b, NodeInfoYAML)
}

func TestGetNodeInfo(t *testing.T) {
	_, err := GetNodeInfo()
	assert.NoError(t, err)
}
