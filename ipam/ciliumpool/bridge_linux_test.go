package ciliumpool

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
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

func fixture(t *testing.T) (*Bridge, *unstructured.Unstructured, *corev1.Node, Guest) {
	t.Helper()
	data, err := os.ReadFile("testdata/ciliumnode-v1.20.1.json")
	require.NoError(t, err)
	cn := &unstructured.Unstructured{}
	require.NoError(t, cn.UnmarshalJSON(data))
	nic, _, err := unstructured.NestedString(cn.Object, "spec", "ipam", "pool", "10.20.16.10", "resource")
	require.NoError(t, err)
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
	require.NoError(t, err)
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{CiliumNodes: "CiliumNodeList", Nodes: "NodeList"},
		cn, &unstructured.Unstructured{Object: rawNode})
	mac, err := net.ParseMAC("00:11:22:33:44:55")
	require.NoError(t, err)
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
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.StateDir, "locks"), 0o700))
	return b, cn, node, g
}

func reserve(t *testing.T, b *Bridge, owner, address string) {
	t.Helper()
	lock, err := processlock.NewFileLock(b.Config.LockPath("azure-vnet-ipam"))
	require.NoError(t, err)
	require.NoError(t, lock.Lock())
	defer func() { require.NoError(t, lock.Unlock()) }()
	kvs, err := store.NewDurableJSONFileStore(b.Config.StatePath("azure-vnet-ipam"), lock, zap.NewNop())
	require.NoError(t, err)
	am, err := ipam.NewAddressManager()
	require.NoError(t, err)
	require.NoError(t, am.Initialize(&common.PluginConfig{Store: kvs}, false,
		map[string]interface{}{common.OptEnvironment: common.OptEnvironmentCiliumPool}))
	_, err = am.RequestAddress("local", b.Config.Subnet, address, map[string]string{ipam.OptAddressID: owner})
	require.NoError(t, err)
}

func TestPoolBridgeUsageConflictWithdrawalAndRestart(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	require.NoError(t, err)
	supply, _, err := eligible(cn, id, g)
	require.NoError(t, err)
	_, _, err = b.transaction(t.Context(), id, supply, true, false)
	require.NoError(t, err)
	require.NoError(t, b.Reconcile(t.Context()))
	require.NoError(t, os.WriteFile(b.Config.StatePath("azure-vnet"), []byte(`{"Network":{"Version":"test"}}`), 0o600))
	reserve(t, b, "pod-a", "10.20.16.10")
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
	require.ErrorContains(t, b.Reconcile(t.Context()), "publishing usage")
	require.NoError(t, b.Reconcile(t.Context()))
	updated, err := client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	require.NoError(t, err)
	used, _, err := unstructured.NestedMap(updated.Object, "status", "ipam", "used")
	require.NoError(t, err)
	require.Len(t, used, 2)
	operatorError, _, err := unstructured.NestedString(updated.Object, "status", "ipam", "operator-status", "error")
	require.NoError(t, err)
	require.Equal(t, "retained operator diagnostic", operatorError)
	azureBefore, _, _ := unstructured.NestedMap(cn.Object, "status", "azure")
	azureAfter, _, _ := unstructured.NestedMap(updated.Object, "status", "azure")
	require.Equal(t, azureBefore, azureAfter)
	resource, _, _ := unstructured.NestedString(updated.Object, "status", "ipam", "used", "10.20.16.10", "resource")
	require.Equal(t, b.Config.NICID, resource, "preserve the operator's original resource spelling")

	unstructured.RemoveNestedField(updated.Object, "spec", "ipam", "pool", "10.20.16.10")
	_, err = client.Resource(CiliumNodes).Update(t.Context(), updated, metav1.UpdateOptions{})
	require.NoError(t, err)
	// A fresh bridge instance must reload usage, not reuse a cached AddressManager.
	restarted := *b
	require.NoError(t, restarted.Reconcile(t.Context()))
	usedLocal, quarantined, err := b.transaction(t.Context(), id, supply[1:], false, false)
	require.NoError(t, err)
	require.Len(t, usedLocal, 2)
	require.Equal(t, 1, quarantined)
	updated, err = client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedStringMap(updated.Object, map[string]string{"10.20.16.11": "marked-for-release"},
		"status", "ipam", "release-ips"))
	_, err = client.Resource(CiliumNodes).Update(t.Context(), updated, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, b.Reconcile(t.Context()), "cloud release is unsupported")
	_, _, err = b.transaction(t.Context(), id, supply, false, true)
	require.ErrorContains(t, err, "fenced")
}

func TestPoolBridgeMissingStateAndReplacement(t *testing.T) {
	b, cn, node, g := fixture(t)
	require.ErrorContains(t, b.Reconcile(t.Context()), "missing cilium pool identity")
	id, err := identity(b.Config, node, cn, g)
	require.NoError(t, err)
	supply, _, err := eligible(cn, id, g)
	require.NoError(t, err)
	_, _, err = b.transaction(t.Context(), id, supply, true, true)
	require.NoError(t, err)
	cn.SetUID("replacement")
	_, err = b.Client.Resource(CiliumNodes).Update(t.Context(), cn, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, b.Reconcile(t.Context()), "identity changed")
	_, _, err = b.transaction(t.Context(), id, supply, false, true)
	require.ErrorContains(t, err, "fenced")
}

func TestPoolBridgeRegistration(t *testing.T) {
	b, cn, node, g := fixture(t)
	require.NoError(t, b.Client.Resource(CiliumNodes).Delete(t.Context(), node.Name, metav1.DeleteOptions{}))
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	client.PrependReactor("create", "ciliumnodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured).DeepCopy()
		require.Empty(t, obj.GetUID())
		obj.SetUID("new-cilium-uid")
		obj.SetResourceVersion("1")
		return true, obj, client.Tracker().Create(CiliumNodes, obj, "")
	})
	require.ErrorContains(t, b.Reconcile(t.Context()), "waiting for selected NIC metadata")
	registered, err := client.Resource(CiliumNodes).Get(t.Context(), node.Name, metav1.GetOptions{})
	require.NoError(t, err)
	id, err := identity(b.Config, node, registered, g)
	require.NoError(t, err)
	require.Equal(t, "new-cilium-uid", id.CiliumNodeUID)
	registered.Object["status"] = cn.Object["status"]
	pool, _, err := unstructured.NestedMap(cn.Object, "spec", "ipam", "pool")
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedMap(registered.Object, pool, "spec", "ipam", "pool"))
	_, err = client.Resource(CiliumNodes).Update(t.Context(), registered, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, b.Reconcile(t.Context()))
	require.NoError(t, os.Remove(b.Config.StatePath("azure-vnet-ipam")))
	require.ErrorContains(t, b.Reconcile(t.Context()), "missing cilium pool identity")
}

func TestPoolBridgeGuestAndAdmission(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		mutate func(*unstructured.Unstructured)
	}{
		{"foreign resource", func(n *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(n.Object, "other", "spec", "ipam", "pool", "10.20.16.10", "resource"))
		}},
		{"primary address", func(n *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(n.Object, map[string]interface{}{"resource": b.Config.NICID},
				"spec", "ipam", "pool", "10.20.16.4"))
		}},
		{"missing metadata", func(n *unstructured.Unstructured) { unstructured.RemoveNestedField(n.Object, "status", "azure") }},
		{"out of subnet", func(n *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(n.Object, map[string]interface{}{"resource": b.Config.NICID},
				"spec", "ipam", "pool", "10.30.0.1"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			modified := cn.DeepCopy()
			test.mutate(modified)
			_, _, admissionErr := eligible(modified, id, g)
			require.Error(t, admissionErr)
		})
	}
	delete(g.Secondaries, netip.MustParseAddr("10.20.16.11"))
	supply, total, err := eligible(cn, id, g)
	require.NoError(t, err)
	require.Len(t, supply, 1)
	require.Equal(t, 2, total)
	g.InstanceID += "/unexpected"
	_, err = identity(b.Config, node, cn, g)
	require.Error(t, err)
	xml := `<Interfaces><Interface MacAddress="001122334455" IsPrimary="true"><IPSubnet Prefix="10.20.16.0/20">` +
		`<IPAddress Address="10.20.16.4" IsPrimary="true"/><IPAddress Address="10.20.16.10" IsPrimary="false"/>` +
		`</IPSubnet></Interface></Interfaces>`
	guest, err := parseGuest([]byte(xml), g.MAC, b.Config.Subnet)
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("10.20.16.4"), guest.Primary)
	require.Len(t, guest.Secondaries, 1)
	_, err = parseGuest([]byte(xml), net.HardwareAddr{1, 2, 3, 4, 5, 6}, b.Config.Subnet)
	require.Error(t, err)
}

func TestPoolBridgeProcessLock(t *testing.T) {
	if dir := os.Getenv("ACN_POOL_LOCK_CHILD"); dir != "" {
		root, err := os.OpenRoot(dir)
		require.NoError(t, err)
		defer root.Close()
		path := filepath.Join(dir, "ipam.lock")
		lock, err := processlock.NewFileLock(path)
		require.NoError(t, err)
		require.NoError(t, root.WriteFile("ipam.lock.started", nil, 0o600))
		require.NoError(t, lock.Lock())
		defer func() { require.NoError(t, lock.Unlock()) }()
		require.NoError(t, root.WriteFile("ipam.lock.acquired", nil, 0o600))
		return
	}
	path := filepath.Join(t.TempDir(), "ipam.lock")
	lock, err := acquire(t.Context(), path, false)
	require.NoError(t, err)
	defer lock.Close()
	_, err = acquire(t.Context(), path, false)
	require.Error(t, err, "single-publisher lifetime locks must be exclusive")
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestPoolBridgeProcessLock$")
	cmd.Env = append(os.Environ(), "ACN_POOL_LOCK_CHILD="+filepath.Dir(path))
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(path + ".started")
		return statErr == nil
	}, 5*time.Second, 10*time.Millisecond)
	require.NoFileExists(t, path+".acquired")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = acquire(ctx, path, true)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, lock.Close())
	require.NoError(t, cmd.Wait())
	require.FileExists(t, path+".acquired")
}

func TestPoolBridgePatchRetainsUnownedFields(t *testing.T) {
	_, cn, _, _ := fixture(t)
	used := map[string]ipam.CiliumPoolUsage{"10.20.16.10": {Owner: "pod", Resource: "nic"}}
	data, err := usagePatch(cn, used)
	require.NoError(t, err)
	var ops []map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &ops))
	require.Len(t, ops, 3)
	require.Equal(t, "/metadata/resourceVersion", ops[0]["path"])
	require.Equal(t, "/metadata/uid", ops[1]["path"])
	require.Equal(t, "/status/ipam/used", ops[2]["path"])
}

func TestPoolBridgeWatchRecoveryAndSinglePublisher(t *testing.T) {
	b, cn, node, g := fixture(t)
	id, err := identity(b.Config, node, cn, g)
	require.NoError(t, err)
	_, _, err = b.transaction(t.Context(), id, nil, true, false)
	require.NoError(t, err)
	client := b.Client.(*dynamicfake.FakeDynamicClient)
	var watches atomic.Int32
	first := watch.NewRaceFreeFake()
	second := watch.NewRaceFreeFake()
	client.PrependWatchReactor("ciliumnodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		require.Equal(t, "metadata.name=worker-1", action.(ktesting.WatchAction).GetWatchRestrictions().Fields.String())
		if watches.Add(1) == 1 {
			return true, first, nil
		}
		return true, second, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	require.Eventually(t, func() bool { return watches.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	other := &Bridge{Config: b.Config, Client: b.Client, Discover: b.Discover, Logger: b.Logger}
	require.ErrorContains(t, other.Run(t.Context()), "lifetime lock")
	first.Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
	require.Eventually(t, func() bool { return watches.Load() >= 2 }, 8*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not stop after cancellation")
	}
	for _, action := range client.Actions() {
		require.NotEqual(t, "patch", action.GetVerb(), "unchanged empty usage must not create a self-watch write loop")
	}
}
