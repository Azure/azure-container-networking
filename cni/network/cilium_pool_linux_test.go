package network

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/iptables"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/store"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"go.uber.org/zap"
)

func TestCiliumPoolAddRejectsUnsupportedDispatch(t *testing.T) {
	tests := []struct {
		name   string
		change func(*cni.NetworkConfig)
		noHost bool
	}{
		{name: "CNS", change: func(c *cni.NetworkConfig) { c.IPAM.Type = acnnetwork.AzureCNS }},
		{name: "baremetal", change: func(c *cni.NetworkConfig) { c.ExecutionMode = "baremetal" }},
		{name: "multitenancy", change: func(c *cni.NetworkConfig) { c.MultiTenancy = true }},
		{name: "dual network", change: func(c *cni.NetworkConfig) { c.PodNamespaceForDualNetwork = []string{"ns"} }},
		{name: "IPv6", change: func(c *cni.NetworkConfig) { c.IPV6Mode = "nat" }},
		{name: "overlay", change: func(c *cni.NetworkConfig) { c.Mode = "overlay" }},
		{name: "conflicting environment", change: func(c *cni.NetworkConfig) { c.IPAM.Environment = common.OptEnvironmentAzure }},
		{name: "missing host configuration", noHost: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, err := cni.NewPlugin("azure-vnet", "test")
			if err != nil {
				t.Fatal(err)
			}
			plugin := &NetPlugin{Plugin: base}
			if !tt.noHost {
				plugin.ciliumPool = &common.CiliumPoolConfig{}
			}
			cfg := cni.NetworkConfig{
				Mode:               transparentTunnelMode,
				IPAM:               cni.IPAM{Type: azureIPAMPlugin, Environment: common.OptEnvironmentCiliumPool},
				DisableIPTableLock: !iptables.DisableIPTableLock,
			}
			if tt.change != nil {
				tt.change(&cfg)
			}
			err = plugin.Add(&cniSkel.CmdArgs{
				ContainerID: "pod", Netns: "ns", IfName: eth0IfName,
				Args: "K8S_POD_NAME=pod;K8S_POD_NAMESPACE=ns", StdinData: cfg.Serialize(),
			})
			if err == nil || !strings.Contains(err.Error(), "cilium pool requires") {
				t.Fatalf("got %v, want pool configuration rejection", err)
			}
			if iptables.DisableIPTableLock == cfg.DisableIPTableLock {
				t.Fatal("ADD changed global platform configuration before rejection")
			}
		})
	}
}

func TestCiliumPoolNetworkConfigAllowsSupportedAndLegacyModes(t *testing.T) {
	if err := validateCiliumPoolNetworkConfig(true, &cni.NetworkConfig{
		Mode: transparentTunnelMode, IPAM: cni.IPAM{Type: azureIPAMPlugin},
	}); err != nil {
		t.Fatalf("supported pool config: %v", err)
	}
	if err := validateCiliumPoolNetworkConfig(false, &cni.NetworkConfig{
		MultiTenancy: true, IPAM: cni.IPAM{Type: acnnetwork.AzureCNS},
	}); err != nil {
		t.Fatalf("legacy config: %v", err)
	}
}

func TestCiliumPoolNetworkStateCannotBeResetWithReservations(t *testing.T) {
	cfg := &common.CiliumPoolConfig{StateDir: t.TempDir()}
	kvs, err := store.NewBoltStore(cfg.StatePath("azure-vnet-ipam"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.NewBoltStore(cfg.StatePath("azure-vnet"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	config := &common.PluginConfig{CiliumPool: cfg, Store: network}
	if err = network.Lock(store.DefaultLockTimeoutLinux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if unlockErr := network.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
	})
	if err = kvs.Lock(store.DefaultLockTimeoutLinux); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if unlockErr := kvs.Unlock(); unlockErr != nil && !errors.Is(unlockErr, store.ErrStoreNotLocked) {
			t.Error(unlockErr)
		}
	})
	id := ipam.CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", VMID: "uuid", InstanceID: "vm",
		NICID: "nic", Interface: eth0IfName, Subnet: "10.20.16.0/20",
	}
	supply := []ipam.CiliumPoolAddress{{Address: netip.MustParseAddr("10.20.16.10"), Resource: "nic"}}
	_, _, err = ipam.CiliumPoolTransaction(kvs, id, supply, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = kvs.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err = validateCiliumPoolNetworkState(config); err != nil {
		t.Fatalf("fresh network bootstrap with no ownership: %v", err)
	}
	am, err := ipam.NewAddressManager()
	if err != nil {
		t.Fatal(err)
	}
	if err = kvs.Lock(store.DefaultLockTimeoutLinux); err != nil {
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
	if err = kvs.Unlock(); err != nil {
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
