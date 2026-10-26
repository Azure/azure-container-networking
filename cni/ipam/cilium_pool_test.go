package ipam

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/stretchr/testify/require"
)

func TestCiliumPoolConfigurationCannotSelectIndependentSource(t *testing.T) {
	config := &common.PluginConfig{CiliumPool: &common.CiliumPoolConfig{}}
	plugin, err := NewPlugin("azure-vnet-ipam", config)
	require.NoError(t, err)
	require.NoError(t, plugin.Initialize(config))
	nw, err := plugin.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam"}}`))
	require.NoError(t, err)
	require.Equal(t, common.OptEnvironmentCiliumPool, nw.IPAM.Environment)
	_, err = plugin.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam","environment":"azure"}}`))
	require.ErrorContains(t, err, "conflicts")
	plain, err := NewPlugin("azure-vnet-ipam", &common.PluginConfig{})
	require.NoError(t, err)
	_, err = plain.Configure([]byte(`{"cniVersion":"0.3.0","name":"azure","ipam":{"type":"azure-vnet-ipam","environment":"cilium-pool"}}`))
	require.ErrorContains(t, err, "host configuration")
}
