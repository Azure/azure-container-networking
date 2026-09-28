//go:build unit

package pipelines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildScriptsUseAzureUbuntuMirror(t *testing.T) {
	const rewrite = `for f in /etc/apt/sources.list /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; do
    if [[ -f "$f" ]]; then
      sed -i 's#//archive\.ubuntu\.com#//azure.archive.ubuntu.com#g' "$f"
    fi
  done`

	paths, err := filepath.Glob(filepath.Join("..", "..", ".pipelines", "build", "scripts", "*.sh"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	var checked []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		script := string(data)
		update := strings.Index(script, "apt-get update")
		if update == -1 {
			continue
		}
		checked = append(checked, filepath.Base(path))
		t.Run(filepath.Base(path), func(t *testing.T) {
			debian := strings.Index(script, "if [[ -f /etc/debian_version ]];")
			require.NotEqual(t, -1, debian)
			require.Less(t, debian, update)
			beforeUpdate := strings.Join(strings.Fields(script[debian:update]), " ")
			require.Contains(t, beforeUpdate, strings.Join(strings.Fields(rewrite), " "),
				"rewrite legacy and deb822 sources to the Azure mirror before the first apt-get update")
		})
	}
	require.NotEmpty(t, checked, "expected build scripts that install Ubuntu packages")
}
