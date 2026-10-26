package ipam

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testCiliumNIC           = "nic"
	testCiliumAddress       = "10.20.16.10"
	testCiliumSecondAddress = "10.20.16.11"
	testCiliumStateFile     = "state.json"
	testCiliumPaused        = "admissions paused"
	testCiliumInterface     = "eth0"
	testCiliumSubnet        = "10.20.16.0/20"
	testCiliumVMID          = "uuid"
)

func TestCiliumPoolReservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipam.json")
	identity := CiliumPoolIdentity{
		NodeUID: "node", CiliumNodeUID: "cn", InstanceID: "vm",
		VMID: "incarnation", NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
	}
	supply := []CiliumPoolAddress{
		{netip.MustParseAddr(testCiliumAddress), testCiliumNIC},
		{netip.MustParseAddr(testCiliumSecondAddress), testCiliumNIC},
	}
	freshStore := func() store.KeyValueStore {
		kvs, err := store.NewDurableJSONFileStore(path, nil, zap.NewNop())
		require.NoError(t, err)
		return kvs
	}
	load := func() *addressManager {
		am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
		require.NoError(t, am.Initialize(&common.PluginConfig{Store: freshStore()}, false,
			map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}))
		return am
	}
	syncPool := func(supply []CiliumPoolAddress, initialize, admit bool) (map[string]CiliumPoolUsage, int) {
		used, quarantined, err := CiliumPoolTransaction(freshStore(), identity, supply, initialize, admit)
		require.NoError(t, err)
		return used, quarantined
	}
	_, _, err := CiliumPoolTransaction(freshStore(), identity, supply, false, false)
	require.ErrorContains(t, err, "missing cilium pool identity")
	syncPool(supply, true, false)
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressID: "pod-a"})
	require.ErrorContains(t, err, testCiliumPaused)
	syncPool(supply, false, true)

	am := load()
	first := map[string]string{OptAddressID: "pod-a"}
	address, err := am.RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first)
	require.NoError(t, err)
	require.Equal(t, "10.20.16.10/20", address)
	duplicate, err := load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", first)
	require.NoError(t, err)
	require.Equal(t, address, duplicate)
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumSecondAddress, first)
	require.ErrorIs(t, err, errAddressInUse)
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, map[string]string{OptAddressID: "pod-b"})
	require.ErrorIs(t, err, errAddressInUse)
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressType: OptAddressTypeGateway})
	require.Error(t, err)

	used, quarantined := syncPool(supply[1:], false, true)
	require.Equal(t, map[string]CiliumPoolUsage{testCiliumAddress: {Owner: "azure-vnet/pod-a", Resource: testCiliumNIC}}, used)
	require.Equal(t, 1, quarantined)
	for _, explicit := range []string{"", testCiliumAddress} {
		_, requestErr := load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, explicit, first)
		require.ErrorContains(t, requestErr, "not eligible")
	}
	info, err := load().GetPoolInfo(LocalDefaultAddressSpaceId, identity.Subnet)
	require.NoError(t, err)
	require.Equal(t, 1, info.Available)
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressID: "pod-b"})
	require.NoError(t, err)
	used, _ = syncPool(supply[1:], false, true)
	require.Len(t, used, 2, "fresh bridge state must preserve intervening CNI allocations")

	// DEL remains possible when admissions are paused or permanently fenced.
	require.NoError(t, FenceCiliumPool(freshStore(), "unexpected release"))
	require.NoError(t, load().ReleaseAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first))
	require.NoError(t, load().ReleaseAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first))
	used, _ = syncPool(supply[1:], false, false)
	require.NotContains(t, used, testCiliumAddress)
	_, _, err = CiliumPoolTransaction(freshStore(), identity, supply, false, true)
	require.ErrorContains(t, err, "fenced")
	_, err = load().RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressID: "pod-c"})
	require.ErrorContains(t, err, testCiliumPaused)

	changed := identity
	changed.CiliumNodeUID = "replacement"
	_, _, err = CiliumPoolTransaction(freshStore(), changed, supply, false, true)
	require.ErrorContains(t, err, "identity changed")
	require.NoError(t, os.Remove(path))
	_, _, err = CiliumPoolTransaction(freshStore(), identity, supply, false, true)
	require.ErrorContains(t, err, "missing cilium pool identity")
	require.NoError(t, os.WriteFile(path, []byte(`{"IPAM":`), 0o600))
	_, _, err = CiliumPoolTransaction(freshStore(), identity, supply, false, true)
	require.Error(t, err)
}

func TestCiliumPoolLeaseAndOwnerOnlyReservation(t *testing.T) {
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	as, err := am.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	require.NoError(t, err)
	am.AddrSpaces[LocalDefaultAddressSpaceId] = as
	subnet := netip.MustParsePrefix(testCiliumSubnet)
	path := filepath.Join(t.TempDir(), testCiliumStateFile)
	kvs, err := store.NewDurableJSONFileStore(path, nil, zap.NewNop())
	require.NoError(t, err)
	id := CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
		NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: subnet.String(),
	}
	supply := []CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}}
	_, _, err = CiliumPoolTransaction(kvs, id, supply, true, true)
	require.NoError(t, err)
	am.store = kvs
	require.NoError(t, am.restore(false))
	ap := am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[id.Subnet]
	ar := ap.Addresses[testCiliumAddress]
	ar.ID = "incomplete-add"
	ar.InUse = false
	require.NoError(t, am.save())
	used, quarantined, err := CiliumPoolTransaction(kvs, id, nil, false, false)
	require.NoError(t, err)
	require.Contains(t, used, testCiliumAddress)
	require.Equal(t, 1, quarantined)
	require.NoError(t, am.restore(false))
	am.CiliumPool.ReadyUntil = time.Now().Add(time.Minute)
	am.CiliumPool.BootID = "previous-boot"
	require.ErrorContains(t, am.checkCiliumPoolAdmission(), "post-reboot")
	am.CiliumPool.ReadyUntil = time.Now().Add(-time.Minute)
	require.ErrorContains(t, am.checkCiliumPoolAdmission(), testCiliumPaused)
	require.NoError(t, am.ReleaseAddress(LocalDefaultAddressSpaceId, id.Subnet, "", map[string]string{OptAddressID: "incomplete-add"}))
	reserved, err := CiliumPoolHasReservations(kvs)
	require.NoError(t, err)
	require.False(t, reserved, "authoritative DEL clears an owner-only incomplete reservation")
}

func TestCiliumPoolRejectsForeignSupplyAndStaticSource(t *testing.T) {
	kvs, err := store.NewDurableJSONFileStore(filepath.Join(t.TempDir(), testCiliumStateFile), nil, zap.NewNop())
	require.NoError(t, err)
	id := CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
		NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
	}
	for _, entry := range []CiliumPoolAddress{
		{netip.MustParseAddr("10.30.0.1"), testCiliumNIC},
		{netip.MustParseAddr(testCiliumAddress), "other-nic"},
		{netip.MustParseAddr("2001:db8::1"), testCiliumNIC},
	} {
		_, _, transactionErr := CiliumPoolTransaction(kvs, id, []CiliumPoolAddress{entry}, true, true)
		require.Error(t, transactionErr)
	}
	_, _, err = CiliumPoolTransaction(kvs, id, nil, true, true)
	require.NoError(t, err)
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	require.Error(t, am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentAzure}))
}

type failedCiliumPoolStore struct{ store.KeyValueStore }

var errStorageFailure = errors.New("injected storage failure")

func (failedCiliumPoolStore) Write(string, interface{}) error {
	return errStorageFailure
}

func TestCiliumPoolCIDRRollbackAndFailedReplay(t *testing.T) {
	kvs, err := store.NewDurableJSONFileStore(filepath.Join(t.TempDir(), testCiliumStateFile), nil, zap.NewNop())
	require.NoError(t, err)
	id := CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
		NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
	}
	_, _, err = CiliumPoolTransaction(kvs, id,
		[]CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}}, true, true)
	require.NoError(t, err)
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	require.NoError(t, am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}))
	options := map[string]string{OptAddressID: "pod"}
	cidr, err := am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	require.NoError(t, err)
	require.NoError(t, am.ReleaseAddress(LocalDefaultAddressSpaceId, id.Subnet, cidr, options))
	info, err := am.GetPoolInfo(LocalDefaultAddressSpaceId, id.Subnet)
	require.NoError(t, err)
	require.Equal(t, 1, info.Available, "failed endpoint creation must release the CIDR returned by ADD")
	_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	require.NoError(t, err)
	am.store = failedCiliumPoolStore{kvs}
	_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	require.ErrorContains(t, err, "storage failure")
	require.True(t, am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[id.Subnet].Addresses[testCiliumAddress].InUse,
		"failed duplicate ADD must not forget an existing reservation")
}

func TestCiliumPoolCorruptionCannotBecomeEmptyUsage(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(*addressManager)
	}{
		{"missing spaces", func(am *addressManager) { am.AddrSpaces = nil }},
		{"empty spaces", func(am *addressManager) { am.AddrSpaces = make(map[string]*addressSpace) }},
		{"missing pool", func(am *addressManager) { am.AddrSpaces[LocalDefaultAddressSpaceId].Pools = nil }},
		{"empty pools", func(am *addressManager) {
			am.AddrSpaces[LocalDefaultAddressSpaceId].Pools = make(map[string]*addressPool)
		}},
		{"nil pool", func(am *addressManager) { am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[testCiliumSubnet] = nil }},
		{"nil record", func(am *addressManager) {
			am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[testCiliumSubnet].Addresses[testCiliumAddress] = nil
		}},
		{"foreign resource", func(am *addressManager) {
			am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[testCiliumSubnet].Addresses[testCiliumAddress].Resource = "other"
		}},
		{"duplicate owner", func(am *addressManager) {
			for _, ar := range am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[testCiliumSubnet].Addresses {
				ar.ID = "same-owner"
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), testCiliumStateFile)
			kvs, err := store.NewDurableJSONFileStore(path, nil, zap.NewNop())
			require.NoError(t, err)
			id := CiliumPoolIdentity{
				NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
				NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
			}
			supply := []CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}, {netip.MustParseAddr(testCiliumSecondAddress), testCiliumNIC}}
			_, _, err = CiliumPoolTransaction(kvs, id, supply, true, true)
			require.NoError(t, err)
			am := &addressManager{store: kvs, AddrSpaces: make(map[string]*addressSpace)}
			require.NoError(t, am.restore(false))
			_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, testCiliumAddress, map[string]string{OptAddressID: "pod"})
			require.NoError(t, err)
			test.corrupt(am)
			require.NoError(t, kvs.Write(storeKey, am))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			kvs, err = store.NewDurableJSONFileStore(path, nil, zap.NewNop())
			require.NoError(t, err)
			_, _, err = CiliumPoolTransaction(kvs, id, supply, false, true)
			require.ErrorContains(t, err, "persisted cilium pool")
			_, err = CiliumPoolHasReservations(kvs)
			require.ErrorContains(t, err, "persisted cilium pool")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after, "corrupt state must never be rewritten as an empty pool")
		})
	}
}
