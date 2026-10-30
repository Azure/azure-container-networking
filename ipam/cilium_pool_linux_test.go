package ipam

import (
	"bytes"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/google/go-cmp/cmp"
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
	testCiliumSecondOwner   = "pod-b"
)

func ciliumPoolTestStore(t *testing.T, path string) store.KeyValueStore {
	t.Helper()
	kvs, err := store.NewJsonFileStore(path, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return kvs
}

func loadCiliumPoolTestManager(t *testing.T, path string) *addressManager {
	t.Helper()
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	if err := am.Initialize(&common.PluginConfig{Store: ciliumPoolTestStore(t, path)}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}); err != nil {
		t.Fatal(err)
	}
	return am
}

func syncCiliumPoolTest(t *testing.T, path string, identity CiliumPoolIdentity, supply []CiliumPoolAddress,
	initialize, admit bool,
) (used map[string]CiliumPoolUsage, quarantined int) {
	t.Helper()
	used, quarantined, err := CiliumPoolTransaction(ciliumPoolTestStore(t, path), identity, supply, initialize, admit)
	if err != nil {
		t.Fatal(err)
	}
	return used, quarantined
}

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
	first := map[string]string{OptAddressID: "pod-a"}
	_, _, err := CiliumPoolTransaction(ciliumPoolTestStore(t, path), identity, supply, false, false)
	if err == nil || !strings.Contains(err.Error(), "missing cilium pool identity") {
		t.Errorf("uninitialized pool: got %v, want missing identity error", err)
	}
	syncCiliumPoolTest(t, path, identity, supply, true, false)
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", first)
	if err == nil || !strings.Contains(err.Error(), testCiliumPaused) {
		t.Errorf("pool before admission: got %v, want paused error", err)
	}
	syncCiliumPoolTest(t, path, identity, supply, false, true)

	am := loadCiliumPoolTestManager(t, path)
	address, err := am.RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first)
	if err != nil {
		t.Fatal(err)
	}
	if address != "10.20.16.10/20" {
		t.Errorf("allocated address = %q, want 10.20.16.10/20", address)
	}
	duplicate, err := loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", first)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate != address {
		t.Errorf("duplicate allocation = %q, want original address %q", duplicate, address)
	}
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumSecondAddress, first)
	if !errors.Is(err, errAddressInUse) {
		t.Errorf("same owner requesting another address: got %v, want %v", err, errAddressInUse)
	}
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, map[string]string{OptAddressID: testCiliumSecondOwner})
	if !errors.Is(err, errAddressInUse) {
		t.Errorf("another owner requesting allocated address: got %v, want %v", err, errAddressInUse)
	}
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressType: OptAddressTypeGateway})
	if err == nil {
		t.Error("gateway allocation from managed pool succeeded")
	}

	used, quarantined := syncCiliumPoolTest(t, path, identity, supply[1:], false, true)
	wantUsed := map[string]CiliumPoolUsage{testCiliumAddress: {Owner: "azure-vnet/pod-a", Resource: testCiliumNIC}}
	if diff := cmp.Diff(wantUsed, used); diff != "" {
		t.Errorf("usage after withdrawal (-want +got):\n%s", diff)
	}
	if quarantined != 1 {
		t.Errorf("quarantined addresses = %d, want 1", quarantined)
	}
	for _, explicit := range []string{"", testCiliumAddress} {
		_, requestErr := loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, explicit, first)
		if requestErr == nil || !strings.Contains(requestErr.Error(), "not eligible") {
			t.Errorf("withdrawn address request %q: got %v, want ineligible error", explicit, requestErr)
		}
	}
	info, err := loadCiliumPoolTestManager(t, path).GetPoolInfo(LocalDefaultAddressSpaceId, identity.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	if info.Available != 1 {
		t.Errorf("available addresses = %d, want 1", info.Available)
	}
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressID: testCiliumSecondOwner})
	if err != nil {
		t.Fatal(err)
	}
	used, _ = syncCiliumPoolTest(t, path, identity, supply[1:], false, true)
	if len(used) != 2 {
		t.Errorf("fresh bridge state retained %d reservations, want 2 including intervening CNI allocations", len(used))
	}
}

func TestCiliumPoolFencedReleaseAndStateRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipam.json")
	identity := CiliumPoolIdentity{
		NodeUID: "node", CiliumNodeUID: "cn", InstanceID: "vm",
		VMID: "incarnation", NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
	}
	supply := []CiliumPoolAddress{
		{netip.MustParseAddr(testCiliumAddress), testCiliumNIC},
		{netip.MustParseAddr(testCiliumSecondAddress), testCiliumNIC},
	}
	first := map[string]string{OptAddressID: "pod-a"}
	syncCiliumPoolTest(t, path, identity, supply, true, true)
	if _, requestErr := loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first); requestErr != nil {
		t.Fatal(requestErr)
	}
	if _, requestErr := loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumSecondAddress,
		map[string]string{OptAddressID: testCiliumSecondOwner}); requestErr != nil {
		t.Fatal(requestErr)
	}
	syncCiliumPoolTest(t, path, identity, supply[1:], false, true)
	// DEL remains possible when admissions are paused or permanently fenced.
	if err := FenceCiliumPool(ciliumPoolTestStore(t, path), "unexpected release"); err != nil {
		t.Fatal(err)
	}
	if err := loadCiliumPoolTestManager(t, path).ReleaseAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first); err != nil {
		t.Fatalf("release while fenced: %v", err)
	}
	if err := loadCiliumPoolTestManager(t, path).ReleaseAddress(LocalDefaultAddressSpaceId, identity.Subnet, testCiliumAddress, first); err != nil {
		t.Fatalf("duplicate release while fenced: %v", err)
	}
	used, _ := syncCiliumPoolTest(t, path, identity, supply[1:], false, false)
	if _, exists := used[testCiliumAddress]; exists {
		t.Errorf("released address %s is still reported as used", testCiliumAddress)
	}
	_, _, err := CiliumPoolTransaction(ciliumPoolTestStore(t, path), identity, supply, false, true)
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Errorf("admitting fenced pool: got %v, want fenced error", err)
	}
	_, err = loadCiliumPoolTestManager(t, path).RequestAddress(LocalDefaultAddressSpaceId, identity.Subnet, "", map[string]string{OptAddressID: "pod-c"})
	if err == nil || !strings.Contains(err.Error(), testCiliumPaused) {
		t.Errorf("allocation while fenced: got %v, want paused error", err)
	}

	changed := identity
	changed.CiliumNodeUID = "replacement"
	_, _, err = CiliumPoolTransaction(ciliumPoolTestStore(t, path), changed, supply, false, true)
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Errorf("replacement identity: got %v, want identity changed error", err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, _, err = CiliumPoolTransaction(ciliumPoolTestStore(t, path), identity, supply, false, true)
	if err == nil || !strings.Contains(err.Error(), "missing cilium pool identity") {
		t.Errorf("missing persisted state: got %v, want missing identity error", err)
	}
	if err = os.WriteFile(path, []byte(`{"IPAM":`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = CiliumPoolTransaction(ciliumPoolTestStore(t, path), identity, supply, false, true)
	if err == nil {
		t.Error("synchronization with corrupt JSON state succeeded")
	}
}

func TestCiliumPoolLeaseAndOwnerOnlyReservation(t *testing.T) {
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	as, err := am.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		t.Fatal(err)
	}
	am.AddrSpaces[LocalDefaultAddressSpaceId] = as
	subnet := netip.MustParsePrefix(testCiliumSubnet)
	path := filepath.Join(t.TempDir(), testCiliumStateFile)
	kvs, err := store.NewJsonFileStore(path, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	id := CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
		NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: subnet.String(),
	}
	supply := []CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}}
	_, _, err = CiliumPoolTransaction(kvs, id, supply, true, true)
	if err != nil {
		t.Fatal(err)
	}
	am.store = kvs
	if err = am.restore(false); err != nil {
		t.Fatal(err)
	}
	ap := am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[id.Subnet]
	ar := ap.Addresses[testCiliumAddress]
	ar.ID = "incomplete-add"
	ar.InUse = false
	if err = am.save(); err != nil {
		t.Fatal(err)
	}
	used, quarantined, err := CiliumPoolTransaction(kvs, id, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := used[testCiliumAddress]; !exists {
		t.Error("owner-only reservation was omitted from usage")
	}
	if quarantined != 1 {
		t.Errorf("owner-only quarantined addresses = %d, want 1", quarantined)
	}
	if err = am.restore(false); err != nil {
		t.Fatal(err)
	}
	am.CiliumPool.ReadyUntil = time.Now().Add(time.Minute)
	am.CiliumPool.BootID = "previous-boot"
	if err = am.checkCiliumPoolAdmission(); err == nil || !strings.Contains(err.Error(), "post-reboot") {
		t.Errorf("previous boot admission: got %v, want post-reboot error", err)
	}
	am.CiliumPool.ReadyUntil = time.Now().Add(-time.Minute)
	if err = am.checkCiliumPoolAdmission(); err == nil || !strings.Contains(err.Error(), testCiliumPaused) {
		t.Errorf("expired lease admission: got %v, want paused error", err)
	}
	if err = am.ReleaseAddress(LocalDefaultAddressSpaceId, id.Subnet, "", map[string]string{OptAddressID: "incomplete-add"}); err != nil {
		t.Fatal(err)
	}
	reserved, err := CiliumPoolHasReservations(kvs)
	if err != nil {
		t.Fatal(err)
	}
	if reserved {
		t.Error("authoritative DEL did not clear the owner-only incomplete reservation")
	}
}

func TestCiliumPoolRejectsForeignSupplyAndStaticSource(t *testing.T) {
	kvs, err := store.NewJsonFileStore(filepath.Join(t.TempDir(), testCiliumStateFile), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
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
		if transactionErr == nil {
			t.Errorf("foreign supply %+v was accepted", entry)
		}
	}
	_, _, err = CiliumPoolTransaction(kvs, id, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	if err = am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentAzure}); err == nil {
		t.Error("managed pool accepted an independent Azure source")
	}
}

type failedCiliumPoolStore struct{ store.KeyValueStore }

var errStorageFailure = errors.New("injected storage failure")

func (failedCiliumPoolStore) Write(string, interface{}) error {
	return errStorageFailure
}

func TestCiliumPoolCIDRRollbackAndFailedReplay(t *testing.T) {
	kvs, err := store.NewJsonFileStore(filepath.Join(t.TempDir(), testCiliumStateFile), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	id := CiliumPoolIdentity{
		NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
		NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
	}
	_, _, err = CiliumPoolTransaction(kvs, id,
		[]CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	am := &addressManager{AddrSpaces: make(map[string]*addressSpace)}
	if err = am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}); err != nil {
		t.Fatal(err)
	}
	options := map[string]string{OptAddressID: "pod"}
	cidr, err := am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = am.ReleaseAddress(LocalDefaultAddressSpaceId, id.Subnet, cidr, options); err != nil {
		t.Fatal(err)
	}
	info, err := am.GetPoolInfo(LocalDefaultAddressSpaceId, id.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	if info.Available != 1 {
		t.Errorf("available addresses after CIDR rollback = %d, want 1", info.Available)
	}
	_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	if err != nil {
		t.Fatal(err)
	}
	am.store = failedCiliumPoolStore{kvs}
	_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, "", options)
	if !errors.Is(err, errStorageFailure) {
		t.Errorf("failed replay: got %v, want %v", err, errStorageFailure)
	}
	if !am.AddrSpaces[LocalDefaultAddressSpaceId].Pools[id.Subnet].Addresses[testCiliumAddress].InUse {
		t.Error("failed duplicate ADD forgot an existing reservation")
	}
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
			kvs, err := store.NewJsonFileStore(path, nil, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			id := CiliumPoolIdentity{
				NodeUID: "n", CiliumNodeUID: "cn", InstanceID: "vm", VMID: testCiliumVMID,
				NICID: testCiliumNIC, Interface: testCiliumInterface, Subnet: testCiliumSubnet,
			}
			supply := []CiliumPoolAddress{{netip.MustParseAddr(testCiliumAddress), testCiliumNIC}, {netip.MustParseAddr(testCiliumSecondAddress), testCiliumNIC}}
			_, _, err = CiliumPoolTransaction(kvs, id, supply, true, true)
			if err != nil {
				t.Fatal(err)
			}
			am := &addressManager{store: kvs, AddrSpaces: make(map[string]*addressSpace)}
			if err = am.restore(false); err != nil {
				t.Fatal(err)
			}
			_, err = am.RequestAddress(LocalDefaultAddressSpaceId, id.Subnet, testCiliumAddress, map[string]string{OptAddressID: "pod"})
			if err != nil {
				t.Fatal(err)
			}
			test.corrupt(am)
			if err = kvs.Write(storeKey, am); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			kvs, err = store.NewJsonFileStore(path, nil, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = CiliumPoolTransaction(kvs, id, supply, false, true)
			if err == nil || !strings.Contains(err.Error(), "persisted cilium pool") {
				t.Errorf("synchronizing corrupt state: got %v, want persisted pool error", err)
			}
			_, err = CiliumPoolHasReservations(kvs)
			if err == nil || !strings.Contains(err.Error(), "persisted cilium pool") {
				t.Errorf("reading reservations from corrupt state: got %v, want persisted pool error", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("corrupt state was rewritten instead of being preserved")
			}
		})
	}
}
