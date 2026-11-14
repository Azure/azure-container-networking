package ciliumpool

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAllocationRequiresOperatorPoolNotJustNICInventory(t *testing.T) {
	a, api := testAllocator(t)
	if err := unstructured.SetNestedMap(api.node.Object, map[string]any{
		testOtherIP: map[string]any{resourceField: testNIC},
	}, specField, ipamField, poolField); err != nil {
		t.Fatal(err)
	}
	ip, err := a.Add(context.Background(), Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName})
	if err != nil || ip != testOtherIP {
		t.Fatalf("allocated %q, %v; want only operator-published IP", ip, err)
	}
	if releaseErr := a.Delete(context.Background(), Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}, ""); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	node, err := a.Nodes.Get(context.Background(), a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _, err := unstructured.NestedMap(node.Object, specField, ipamField, poolField)
	if err != nil || len(pool) != 1 || pool[testOtherIP] == nil {
		t.Fatalf("DEL changed NIC pool: %v, %v", pool, err)
	}
	used, _, err := unstructured.NestedMap(node.Object, statusField, ipamField, "used")
	if err != nil || len(used) != 0 {
		t.Fatal("DEL did not make the IP locally reusable")
	}
}
