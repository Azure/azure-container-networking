package installer

import (
	"os"
	"path/filepath"
	"testing"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
	"github.com/stretchr/testify/require"
)

func TestModifyConflistsPreservesCNSSocketPath(t *testing.T) {
	const socketPath = "/var/run/azure-cns/cns.sock"
	sourceDir := t.TempDir()
	destinationDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "10-azure.conflist")
	require.NoError(t, os.WriteFile(sourcePath, []byte(`{
		"cniVersion": "0.3.0",
		"name": "azure",
		"plugins": [{
			"type": "azure-vnet",
			"mode": "transparent",
			"cnsSocketPath": "`+socketPath+`",
			"ipam": {"type": "azure-cns"}
		}]
	}`), 0o600))

	config := InstallerConfig{
		DstConflistDir: destinationDir + string(os.PathSeparator),
		IPAMType:       c.AzureCNSIPAM,
		CNIMode:        c.Transparent,
		NetworkName:    "azure",
	}
	require.NoError(t, ModifyConflists(sourcePath, config, 0o600))

	networkConfig, _, _, err := LoadConf(filepath.Join(destinationDir, filepath.Base(sourcePath)))
	require.NoError(t, err)
	require.Equal(t, socketPath, networkConfig.CNSSocketPath)
}
