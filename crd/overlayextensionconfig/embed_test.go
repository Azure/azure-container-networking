package overlayextensionconfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filename = "manifests/acn.azure.com_overlayextensionconfigs.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, b, OverlayExtensionConfigsYAML)
}

func TestGetOverlayExtensionConfigs(t *testing.T) {
	_, err := GetOverlayExtensionConfigs()
	require.NoError(t, err)
}
