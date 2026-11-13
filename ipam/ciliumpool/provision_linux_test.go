package ciliumpool

import (
	"context"
	"net"
	"net/netip"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestProvisionIsExplicitAndNeverOverwritesExistingUsage(t *testing.T) {
	_, cfg, err := ParseConfig([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	g := Guest{
		VMID: "vm-id", BootID: testBoot, InstanceID: "/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm",
		Primary: netip.MustParseAddr(testPrimary), MAC: net.HardwareAddr{0, 17, 34, 51, 68, 85}, Supply: map[string]bool{},
	}
	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName, UID: testNodeUID},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testPrimary}}},
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(node)
	if err != nil {
		t.Fatal(err)
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: raw})
	client.PrependReactor("create", "ciliumnodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		object := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured)
		object.SetUID("object")
		object.SetResourceVersion("1")
		return false, nil, nil
	})
	ctx := context.Background()
	a, err := NewAllocator(ctx, cfg, g, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, addErr := a.Add(ctx, Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}); addErr == nil {
		t.Fatal("ADD initialized a missing CiliumNode")
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "create" {
			t.Fatal("ADD attempted registration")
		}
	}
	if callErr := Provision(ctx, cfg, g, client); callErr != nil {
		t.Fatal(callErr)
	}
	cn, err := client.Resource(CiliumNodes).Get(ctx, testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerPrefix + "vm-id/" + testBoot + "/azure/sandbox/eth0"
	if setErr := unstructured.SetNestedMap(cn.Object, map[string]any{testIP: map[string]any{ownerField: owner, resourceField: cfg.NICID}}, statusField, ipamField, "used"); setErr != nil {
		t.Fatal(setErr)
	}
	if _, updateErr := client.Resource(CiliumNodes).UpdateStatus(ctx, cn, metav1.UpdateOptions{}); updateErr != nil {
		t.Fatal(updateErr)
	}
	if callErr := Provision(ctx, cfg, g, client); callErr != nil {
		t.Fatal(callErr)
	}
	cn, err = client.Resource(CiliumNodes).Get(ctx, testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	used, _, err := unstructured.NestedMap(cn.Object, statusField, ipamField, "used")
	if err != nil || len(used) != 1 {
		t.Fatal("registration rewrote existing usage")
	}
	if err := unstructured.SetNestedField(cn.Object, "corrupt", statusField, ipamField, "used"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(CiliumNodes).Update(ctx, cn, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Provision(ctx, cfg, g, client); err == nil {
		t.Fatal("registration silently repaired corrupt usage")
	}
}
