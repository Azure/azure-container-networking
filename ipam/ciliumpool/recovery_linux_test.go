package ciliumpool

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAPIPolicyDriftBlocksAdmissionButNotDelete(t *testing.T) {
	a, api := testAllocator(t)
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	ip, err := a.Add(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	a.PreAllocate++
	if _, addErr := a.Add(context.Background(), owner); !errors.Is(addErr, ErrConfig) {
		t.Fatalf("ADD accepted policy drift: %v", addErr)
	}
	if reconcileErr := a.Reconcile(context.Background()); !errors.Is(reconcileErr, ErrConfig) {
		t.Fatalf("reconcile accepted policy drift: %v", reconcileErr)
	}
	if deleteErr := a.Delete(context.Background(), owner, ip); deleteErr != nil {
		t.Fatalf("DEL blocked by policy drift: %v", deleteErr)
	}
	state, err := readState(api.node, a.Binding)
	if err != nil || len(state.Reservations) != 0 {
		t.Fatalf("reservation retained: %+v, %v", state, err)
	}
}

func TestAPIExhaustionPreservesUsageAndOwnerOnlyDeleteReleases(t *testing.T) {
	a, _ := testAllocator(t)
	ctx := context.Background()
	owners := []Owner{
		{Network: testNetwork, Sandbox: "a", Interface: testIfName},
		{Network: testNetwork, Sandbox: "b", Interface: testIfName},
		{Network: testNetwork, Sandbox: "pending", Interface: testIfName},
	}
	for _, owner := range owners[:2] {
		if _, err := a.Add(ctx, owner); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := a.Add(ctx, owners[2]); !errors.Is(err, ErrNoSupply) {
			t.Fatalf("exhausted Add = %v", err)
		}
	}
	node, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := readState(node, a.Binding)
	if err != nil || len(state.Reservations) != 2 {
		t.Fatalf("usage state = %+v, %v", state, err)
	}
	if callErr := a.Delete(ctx, owners[0], ""); callErr != nil {
		t.Fatal(callErr)
	}
	if _, callErr := a.Add(ctx, owners[2]); callErr != nil {
		t.Fatal(callErr)
	}
	if callErr := a.Delete(ctx, owners[0], testIP); callErr != nil {
		t.Fatal(callErr)
	}
	node, err = a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err = readState(node, a.Binding)
	if err != nil || state.Reservations[testIP].Owner != owners[2] {
		t.Fatalf("release/reuse changed ownership: %+v, %v", state, err)
	}
}

func TestAPIReconcileRetiresPreviousBootWithoutNewAllocation(t *testing.T) {
	a, _ := testAllocator(t)
	ctx := context.Background()
	if _, err := a.Add(ctx, Owner{Network: testNetwork, Sandbox: testOld, Interface: testIfName}); err != nil {
		t.Fatal(err)
	}
	a.BootID = "22222222-3333-4444-8555-666666666666"
	if err := a.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	node, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := readState(node, a.Binding)
	pool, _, poolErr := unstructured.NestedMap(node.Object, specField, ipamField, poolField)
	if err != nil || poolErr != nil || len(state.Reservations) != 0 || len(pool) != 2 {
		t.Fatalf("reboot reset changed pool or kept ownership: %+v, %v, %v", state, pool, err)
	}
}

func TestAPIWithdrawnReservationDoesNotGetAnotherIP(t *testing.T) {
	a, api := testAllocator(t)
	ctx := context.Background()
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	if _, err := a.Add(ctx, owner); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	if err := unstructured.SetNestedSlice(api.node.Object, []any{map[string]any{
		"id": testNIC, "ip": testPrimary, macField: testMAC, gatewayField: "10.0.0.1",
		subnetField: map[string]any{"id": subnetField, cidrField: testSubnet},
		"addresses": []any{map[string]any{"ip": testOtherIP, stateField: stateSucceeded}},
	}}, statusField, testNetwork, "interfaces"); err != nil {
		t.Fatal(err)
	}
	api.mu.Unlock()
	if _, err := a.Add(ctx, owner); err == nil {
		t.Fatal("withdrawn owner received a new IP")
	}
	node, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := readState(node, a.Binding)
	if err != nil || len(state.Reservations) != 1 {
		t.Fatalf("withdrawn claim changed usage: %+v, %v", state, err)
	}
	a.Supply = nil
	if err := a.Delete(ctx, owner, ""); err != nil {
		t.Fatalf("DEL required eligible supply: %v", err)
	}
}
