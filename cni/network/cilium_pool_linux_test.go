package network

import (
	"net/netip"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCiliumPoolNetworkStateCannotBeResetWithReservations(t *testing.T) {
	cfg := &common.CiliumPoolConfig{StateDir: t.TempDir()}
	kvs, err := store.NewDurableJSONFileStore(cfg.StatePath("azure-vnet-ipam"), nil, zap.NewNop())
	require.NoError(t, err)
	network, err := store.NewDurableJSONFileStore(filepath.Join(cfg.StateDir, "azure-vnet.json"), nil, zap.NewNop())
	require.NoError(t, err)
	config := &common.PluginConfig{CiliumPool: cfg, Store: network}
	id := ipam.CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", VMID: "uuid", InstanceID: "vm",
		NICID: "nic", Interface: eth0IfName, Subnet: "10.20.16.0/20",
	}
	supply := []ipam.CiliumPoolAddress{{Address: netip.MustParseAddr("10.20.16.10"), Resource: "nic"}}
	_, _, err = ipam.CiliumPoolTransaction(kvs, id, supply, true, true)
	require.NoError(t, err)
	require.NoError(t, validateCiliumPoolNetworkState(config), "fresh network bootstrap with no ownership")
	am, err := ipam.NewAddressManager()
	require.NoError(t, err)
	require.NoError(t, am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}))
	_, err = am.RequestAddress("local", id.Subnet, "", map[string]string{ipam.OptAddressID: "pod"})
	require.NoError(t, err)
	require.ErrorContains(t, validateCiliumPoolNetworkState(config), "network state unavailable")
	require.NoError(t, network.Write("Network", map[string]interface{}{}))
	require.ErrorContains(t, validateCiliumPoolNetworkState(config), "no interfaces")
	require.NoError(t, network.Write("Network", map[string]interface{}{"ExternalInterfaces": map[string]interface{}{eth0IfName: map[string]interface{}{}}}))
	require.NoError(t, validateCiliumPoolNetworkState(config))
}
