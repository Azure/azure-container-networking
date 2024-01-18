package manifests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCiliumManifest(t *testing.T) {
	for _, dir := range CiliumV14Directories {
		files, err := CiliumManifests.ReadDir(dir)
		require.NoError(t, err)

		for _, file := range files {
			b, err := CiliumManifests.ReadFile(fmt.Sprintf("%s/%s", dir, file.Name()))
			require.NoError(t, err)
			require.NotEmpty(t, b)
			require.False(t, file.IsDir())
		}
	}
}
