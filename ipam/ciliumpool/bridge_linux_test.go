package ciliumpool

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/store"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

var errOperatorConflict = errors.New("operator wrote status")

const testPoolParentOwner = "pod-a"

func TestPoolBridgeInterfaceState(t *testing.T) {
	for _, state := range []string{"", "succeeded", "updating", "failed"} {
		t.Run(state, func(t *testing.T) {
			b, cn, node, g := fixture(t)
			id, err := identity(b.Config, node, cn, g)
			if err != nil {
				t.Fatal(err)
			}
			interfaces, _, err := unstructured.NestedSlice(cn.Object, "status", "azure", "interfaces")
			if err != nil {
				t.Fatal(err)
			}
			interfaces[0].(map[string]interface{})["state"] = state
			if err = unstructured.SetNestedSlice(cn.Object, interfaces, "status", "azure", "interfaces"); err != nil {
				t.Fatal(err)
			}
			supply, _, err := eligible(cn, id, g)
			if state == "" || state == "succeeded" {
				if err != nil {
					t.Fatal(err)
				}
				if len(supply) != 2 {
					t.Errorf("eligible supply = %d, want 2", len(supply))
				}
			} else if !errors.Is(err, errInvalidContract) {
				t.Errorf("interface state %q: got %v, want invalid contract", state, err)
			}
		})
	}
}

func fixture(t *testing.T) (*Bridge, *unstructured.Unstructured, *corev1.Node, Guest) {
	t.Helper()
	data, err := os.ReadFile("testdata/ciliumnode-v1.20.1.json")
	if err != nil {
		t.Fatal(err)
	}
	cn := &unstructured.Unstructured{}
	if err = cn.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	nic, _, err := unstructured.NestedString(cn.Object, "spec", "ipam", "pool", "10.20.16.10", "resource")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &common.CiliumPoolConfig{
		NodeName: "worker-1", StateDir: t.TempDir(),
		Interface: "eth0", NICID: nic, Subnet: "10.20.16.0/20", MinAllocate: 10, PreAllocate: 8,
	}
	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1", UID: "node-uid"},
		Spec:       corev1.NodeSpec{ProviderID: "azure:///subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/poc/providers/Microsoft.Compute/virtualMachines/worker-1"},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.20.16.4"}}},
	}
	rawNode, err := runtime.DefaultUnstructuredConverter.ToUnstructured(node)
	if err != nil {
		t.Fatal(err)
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{CiliumNodes: "CiliumNodeList", Nodes: "NodeList"},
		cn, &unstructured.Unstructured{Object: rawNode})
	mac, err := net.ParseMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	g := Guest{
		InstanceID: "/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/poc/providers/Microsoft.Compute/virtualMachines/worker-1",
		VMID:       "vm-incarnation", MAC: mac, Primary: netip.MustParseAddr("10.20.16.4"),
		Subnet:      netip.MustParsePrefix(cfg.Subnet),
		Secondaries: map[netip.Addr]bool{netip.MustParseAddr("10.20.16.10"): true, netip.MustParseAddr("10.20.16.11"): true},
	}
	b := &Bridge{
		Config: cfg, Client: client, Logger: zap.NewNop(),
		Discover: func(context.Context, *common.CiliumPoolConfig) (Guest, error) { return g, nil },
	}
	return b, cn, node, g
}

func reserve(t *testing.T, b *Bridge, owner, address string) {
	t.Helper()
	kvs, err := lockStore(t.Context(), b.Config.StatePath("azure-vnet-ipam"), time.Second, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockErr := kvs.Unlock(); unlockErr != nil {
			t.Errorf("unlocking IPAM state: %v", unlockErr)
		}
	}()
	am, err := ipam.NewAddressManager()
	if err != nil {
		t.Fatal(err)
	}
	if err = am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}); err != nil {
		t.Fatal(err)
	}
	_, err = am.RequestAddress("local", b.Config.Subnet, address, map[string]string{ipam.OptAddressID: owner})
	if err != nil {
		t.Fatal(err)
	}
}

func checkPoolBridgePublication(t *testing.T, original, updated *unstructured.Unstructured, nicID string) {
	t.Helper()
	used, _, err := unstructured.NestedMap(updated.Object, "status", "ipam", "used")
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 2 {
		t.Errorf("published usage contains %d reservations, want 2", len(used))
	}
	operatorError, _, err := unstructured.NestedString(updated.Object, "status", "ipam", "operator-status", "error")
	if err != nil {
		t.Fatal(err)
	}
	if operatorError != "retained operator diagnostic" {
		t.Errorf("operator diagnostic = %q, want retained operator diagnostic", operatorError)
	}
	azureBefore, _, err := unstructured.NestedMap(original.Object, "status", "azure")
	if err != nil {
		t.Fatal(err)
	}
	azureAfter, _, err := unstructured.NestedMap(updated.Object, "status", "azure")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(azureBefore, azureAfter); diff != "" {
		t.Errorf("operator Azure status changed (-want +got):\n%s", diff)
	}
	resource, _, err := unstructured.NestedString(updated.Object, "status", "ipam", "used", "10.20.16.10", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if resource != nicID {
		t.Errorf("usage resource = %q, want operator's original spelling %q", resource, nicID)
	}
}

func TestPoolBridgeUsageConflictWithdrawalAndRestart(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	if err != nil {
		t.Fatal(err)
	}
	supply, _, err := eligible(cn, id, g)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.transaction(t.Context(), id, supply, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	network, err := lockStore(t.Context(), b.Config.StatePath("azure-vnet"), time.Second, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = network.Write("Network", map[string]string{"Version": "test"}); err != nil {
		t.Fatal(err)
	}
	if err = network.Unlock(); err != nil {
		t.Fatal(err)
	}
	reserve(t, b, testPoolParentOwner, "10.20.16.10")
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	firstPatch := true
	client.PrependReactor("patch", "ciliumnodes", func(_ ktesting.Action) (bool, runtime.Object, error) {
		if firstPatch {
			firstPatch = false
			// Publication happens outside both file locks. An independent DEL or
			// ADD can complete while the API call is in flight.
			reserve(t, b, "pod-b", "10.20.16.11")
			return true, nil, apierrors.NewConflict(CiliumNodes.GroupResource(), node.Name, errOperatorConflict)
		}
		return false, nil, nil
	})
	if err = b.Reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), "publishing usage") {
		t.Errorf("conflicting publication: got %v, want publishing usage error", err)
	}
	if err = b.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	updated, err := client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkPoolBridgePublication(t, cn, updated, b.Config.NICID)
	unstructured.RemoveNestedField(updated.Object, "spec", "ipam", "pool", "10.20.16.10")
	_, err = client.Resource(CiliumNodes).Update(t.Context(), updated, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// A fresh bridge instance must reload usage, not reuse a cached AddressManager.
	restarted := *b
	if err = restarted.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	usedLocal, quarantined, err := b.transaction(t.Context(), id, supply[1:], false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(usedLocal) != 2 {
		t.Errorf("restarted bridge retained %d reservations, want 2", len(usedLocal))
	}
	if quarantined != 1 {
		t.Errorf("quarantined addresses = %d, want 1", quarantined)
	}
	updated, err = client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = unstructured.SetNestedStringMap(updated.Object, map[string]string{"10.20.16.11": "marked-for-release"},
		"status", "ipam", "release-ips"); err != nil {
		t.Fatal(err)
	}
	_, err = client.Resource(CiliumNodes).Update(t.Context(), updated, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); !errors.Is(err, errUnsupportedRelease) {
		t.Errorf("release handshake: got %v, want %v", err, errUnsupportedRelease)
	}
	_, _, err = b.transaction(t.Context(), id, supply, false, true)
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Errorf("admission after release handshake: got %v, want fenced error", err)
	}
}

func TestPoolBridgeMissingStateAndReplacement(t *testing.T) {
	b, cn, node, g := fixture(t)
	if err := b.Reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), "missing cilium pool identity") {
		t.Errorf("missing local state: got %v, want missing identity error", err)
	}
	id, err := identity(b.Config, node, cn, g)
	if err != nil {
		t.Fatal(err)
	}
	supply, _, err := eligible(cn, id, g)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.transaction(t.Context(), id, supply, true, true)
	if err != nil {
		t.Fatal(err)
	}
	cn.SetUID("replacement")
	_, err = b.Client.Resource(CiliumNodes).Update(t.Context(), cn, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Errorf("replacement identity: got %v, want identity changed error", err)
	}
	_, _, err = b.transaction(t.Context(), id, supply, false, true)
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Errorf("admission after identity replacement: got %v, want fenced error", err)
	}
}

func TestPoolBridgeRegistration(t *testing.T) {
	b, cn, node, g := fixture(t)
	if err := b.Client.Resource(CiliumNodes).Delete(t.Context(), node.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	client.PrependReactor("create", "ciliumnodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured).DeepCopy()
		if uid := obj.GetUID(); uid != "" {
			t.Errorf("registration supplied UID %q, want server-assigned UID", uid)
		}
		obj.SetUID("new-cilium-uid")
		obj.SetResourceVersion("1")
		return true, obj, client.Tracker().Create(CiliumNodes, obj, "")
	})
	if err := b.Reconcile(t.Context()); !errors.Is(err, errMissingNICMetadata) {
		t.Errorf("registration before pool publication: got %v, want %v", err, errMissingNICMetadata)
	}
	registered, err := client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity(b.Config, node, registered, g)
	if err != nil {
		t.Fatal(err)
	}
	if id.CiliumNodeUID != "new-cilium-uid" {
		t.Errorf("bound CiliumNode UID = %q, want new-cilium-uid", id.CiliumNodeUID)
	}
	registered.Object["status"] = cn.Object["status"]
	pool, _, err := unstructured.NestedMap(cn.Object, "spec", "ipam", "pool")
	if err != nil {
		t.Fatal(err)
	}
	if err = unstructured.SetNestedMap(registered.Object, pool, "spec", "ipam", "pool"); err != nil {
		t.Fatal(err)
	}
	_, err = client.Resource(CiliumNodes).Update(t.Context(), registered, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(b.Config.StatePath("azure-vnet-ipam")); err != nil {
		t.Fatal(err)
	}
	if err = b.Reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), "missing cilium pool identity") {
		t.Errorf("deleted IPAM state: got %v, want missing identity error", err)
	}
}

func TestPoolBridgeGuestAndAdmission(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*unstructured.Unstructured) error
	}{
		{"foreign resource", func(n *unstructured.Unstructured) error {
			return unstructured.SetNestedField(n.Object, "other", "spec", "ipam", "pool", "10.20.16.10", "resource")
		}},
		{"primary address", func(n *unstructured.Unstructured) error {
			return unstructured.SetNestedField(n.Object, map[string]interface{}{"resource": b.Config.NICID},
				"spec", "ipam", "pool", "10.20.16.4")
		}},
		{"missing metadata", func(n *unstructured.Unstructured) error {
			unstructured.RemoveNestedField(n.Object, "status", "azure")
			return nil
		}},
		{"out of subnet", func(n *unstructured.Unstructured) error {
			return unstructured.SetNestedField(n.Object, map[string]interface{}{"resource": b.Config.NICID},
				"spec", "ipam", "pool", "10.30.0.1")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			modified := cn.DeepCopy()
			if mutationErr := test.mutate(modified); mutationErr != nil {
				t.Fatal(mutationErr)
			}
			_, _, admissionErr := eligible(modified, id, g)
			if admissionErr == nil {
				t.Error("invalid pool was admitted")
			}
		})
	}
	delete(g.Secondaries, netip.MustParseAddr("10.20.16.11"))
	supply, total, err := eligible(cn, id, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(supply) != 1 {
		t.Errorf("eligible supply = %d addresses, want 1", len(supply))
	}
	if total != 2 {
		t.Errorf("operator pool = %d addresses, want 2", total)
	}
	g.InstanceID += "/unexpected"
	_, err = identity(b.Config, node, cn, g)
	if err == nil {
		t.Error("invalid VM resource ID was accepted")
	}
	document := `<Interfaces><Interface MacAddress="001122334455" IsPrimary="true"><IPSubnet Prefix="10.20.16.0/20">` +
		`<IPAddress Address="10.20.16.4" IsPrimary="true"/><IPAddress Address="10.20.16.10" IsPrimary="false"/>` +
		`</IPSubnet></Interface></Interfaces>`
	var interfaces nmagent.Interfaces
	if err = xml.Unmarshal([]byte(document), &interfaces); err != nil {
		t.Fatal(err)
	}
	guest, err := parseGuest(interfaces, g.MAC, b.Config.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	if want := netip.MustParseAddr("10.20.16.4"); guest.Primary != want {
		t.Errorf("guest primary = %s, want %s", guest.Primary, want)
	}
	if len(guest.Secondaries) != 1 {
		t.Errorf("guest secondaries = %d addresses, want 1", len(guest.Secondaries))
	}
	_, err = parseGuest(interfaces, net.HardwareAddr{1, 2, 3, 4, 5, 6}, b.Config.Subnet)
	if err == nil {
		t.Error("NMAgent response was accepted for a different guest MAC")
	}
}

func updatePoolStateInChild(t *testing.T, dir string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join(dir, "ipam.db")
	lock, err := store.NewBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile("ipam.db.started", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = lock.Lock(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			t.Errorf("unlocking child IPAM state: %v", unlockErr)
		}
	}()
	if err = root.WriteFile("ipam.db.acquired", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var usage map[string]string
	if err = lock.Read("IPAM", &usage); err != nil {
		t.Fatal(err)
	}
	if usage["parent"] != testPoolParentOwner {
		t.Fatalf("child did not restore parent's reservation: %v", usage)
	}
	usage["child"] = "pod-b"
	if err = lock.Write("IPAM", usage); err != nil {
		t.Fatal(err)
	}
}

func TestPoolBridgeProcessLock(t *testing.T) {
	if dir := os.Getenv("ACN_POOL_LOCK_CHILD"); dir != "" {
		updatePoolStateInChild(t, dir)
		return
	}
	path := filepath.Join(t.TempDir(), "ipam.db")
	lock, err := lockStore(t.Context(), path, time.Millisecond, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer unlockStore(lock, zap.NewNop())
	if err = lock.Write("IPAM", map[string]string{"parent": testPoolParentOwner}); err != nil {
		t.Fatal(err)
	}
	otherLock, err := lockStore(t.Context(), path, time.Millisecond, zap.NewNop())
	if err == nil {
		if unlockErr := otherLock.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
		t.Fatal("single-publisher lifetime lock was acquired twice")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childCtx, stopChild := context.WithTimeout(t.Context(), 10*time.Second)
	defer stopChild()
	cmd := exec.CommandContext(childCtx, executable, "-test.run=^TestPoolBridgeProcessLock$")
	cmd.Env = append(os.Environ(), "ACN_POOL_LOCK_CHILD="+filepath.Dir(path))
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		_, statErr := os.Stat(path + ".started")
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			t.Fatal(statErr)
		}
		select {
		case <-poll.C:
		case <-timeout.C:
			t.Fatal("child did not start its lock attempt")
		}
	}
	if _, statErr := os.Stat(path + ".acquired"); !os.IsNotExist(statErr) {
		t.Fatalf("child acquired a held lock or marker could not be inspected: %v", statErr)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	canceledLock, err := lockStore(ctx, path, time.Second, zap.NewNop())
	if canceledLock != nil {
		if unlockErr := canceledLock.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
		t.Error("canceled lock attempt returned an open lock")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled lock attempt: got %v, want %v", err, context.Canceled)
	}
	if err = lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(path + ".acquired"); statErr != nil {
		t.Fatal(statErr)
	} else if !info.Mode().IsRegular() {
		t.Error("child lock acquisition marker is not a regular file")
	}
	if err = lock.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	var usage map[string]string
	if err = lock.Read("IPAM", &usage); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[string]string{"parent": testPoolParentOwner, "child": "pod-b"}, usage); diff != "" {
		t.Errorf("reservations after process handoff (-want +got):\n%s", diff)
	}
}

func TestPoolBridgePatchRetainsUnownedFields(t *testing.T) {
	_, cn, _, _ := fixture(t)
	used := map[string]ipam.CiliumPoolUsage{"10.20.16.10": {Owner: "pod", Resource: "nic"}}
	data, err := usagePatch(cn, used)
	if err != nil {
		t.Fatal(err)
	}
	var ops []map[string]interface{}
	if err = json.Unmarshal(data, &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 3 {
		t.Fatalf("patch contains %d operations, want 3", len(ops))
	}
	paths := []string{"/metadata/resourceVersion", "/metadata/uid", "/status/ipam/used"}
	for i, path := range paths {
		if ops[i]["path"] != path {
			t.Errorf("patch operation %d path = %v, want %s", i, ops[i]["path"], path)
		}
	}
}

func TestPoolBridgeWatchRecoveryAndSinglePublisher(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.transaction(t.Context(), id, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	var watches atomic.Int32
	watchStarted := make(chan struct{}, 2)
	first := watch.NewRaceFreeFake()
	second := watch.NewRaceFreeFake()
	client.PrependWatchReactor("ciliumnodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		if selector := action.(ktesting.WatchAction).GetWatchRestrictions().Fields.String(); selector != "metadata.name=worker-1" {
			t.Errorf("watch field selector = %q, want metadata.name=worker-1", selector)
		}
		count := watches.Add(1)
		if count <= 2 {
			watchStarted <- struct{}{}
		}
		if count == 1 {
			return true, first, nil
		}
		return true, second, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case runErr := <-done:
			if runErr != nil {
				t.Errorf("bridge stopped with error: %v", runErr)
			}
		case <-time.After(2 * time.Second):
			t.Error("bridge did not stop after cancellation")
		}
	})
	select {
	case <-watchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not start its first watch")
	}
	other := &Bridge{Config: b.Config, Client: b.Client, Discover: b.Discover, Logger: b.Logger}
	if err = other.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "lifetime lock") {
		t.Errorf("second publisher: got %v, want lifetime lock error", err)
	}
	first.Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
	select {
	case <-watchStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("bridge did not recreate its expired watch")
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" {
			t.Error("unchanged empty usage generated a patch, risking a self-watch write loop")
		}
	}
}
