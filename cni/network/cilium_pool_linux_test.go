package network

import (
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
	"go.uber.org/zap"
)

func TestCiliumPoolNetworkStateCannotBeResetWithReservations(t *testing.T) {
	cfg := &common.CiliumPoolConfig{StateDir: t.TempDir()}
	kvs, err := store.NewDurableJSONFileStore(cfg.StatePath("azure-vnet-ipam"), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.NewDurableJSONFileStore(filepath.Join(cfg.StateDir, "azure-vnet.json"), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	config := &common.PluginConfig{CiliumPool: cfg, Store: network}
	id := ipam.CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", VMID: "uuid", InstanceID: "vm",
		NICID: "nic", Interface: eth0IfName, Subnet: "10.20.16.0/20",
	}
	supply := []ipam.CiliumPoolAddress{{Address: netip.MustParseAddr("10.20.16.10"), Resource: "nic"}}
	_, _, err = ipam.CiliumPoolTransaction(kvs, id, supply, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateCiliumPoolNetworkState(config); err != nil {
		t.Fatalf("fresh network bootstrap with no ownership: %v", err)
	}
	am, err := ipam.NewAddressManager()
	if err != nil {
		t.Fatal(err)
	}
	if err = am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}); err != nil {
		t.Fatal(err)
	}
	_, err = am.RequestAddress("local", id.Subnet, "", map[string]string{ipam.OptAddressID: "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCiliumPoolNetworkState(config); err == nil || !strings.Contains(err.Error(), "network state unavailable") {
		t.Errorf("missing network state with reservations: got %v, want unavailable error", err)
	}
	if err := network.Write("Network", map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	if err := validateCiliumPoolNetworkState(config); err == nil || !strings.Contains(err.Error(), "no interfaces") {
		t.Errorf("empty network state with reservations: got %v, want no interfaces error", err)
	}
	if err := network.Write("Network", map[string]interface{}{"ExternalInterfaces": map[string]interface{}{eth0IfName: map[string]interface{}{}}}); err != nil {
		t.Fatal(err)
	}
	if err := validateCiliumPoolNetworkState(config); err != nil {
		t.Errorf("valid network state with reservations: %v", err)
	}
}
