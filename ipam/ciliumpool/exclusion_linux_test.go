package ciliumpool

import (
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWithdrawnAddressIsNotImportedOrAllocated(t *testing.T) {
	a, api := testAllocator(t)
	api.mu.Lock()
	state := newState(a.Binding.NodeUID, a.Binding.VMID)
	state.Withdrawn[testOtherIP] = true
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	annotations := api.node.GetAnnotations()
	annotations[stateAnnotation] = string(data)
	api.node.SetAnnotations(annotations)
	api.mu.Unlock()
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	if _, callErr := a.Add(context.Background(), owner); callErr != nil {
		t.Fatal(callErr)
	}
	node, err := a.Nodes.Get(context.Background(), a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, retained, err := readState(node, a.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 1 || retained[0] != testPrefix || !state.Withdrawn[testOtherIP] {
		t.Fatalf("withheld address reintroduced or forgotten: %v, %+v", retained, state)
	}
	if _, err := a.Add(context.Background(), Owner{Network: testNetwork, Sandbox: "another", Interface: testIfName}); err == nil {
		t.Fatal("withdrawn address allocated")
	}
}

func TestReadStateRejectsDuplicateOwnerAndMissingBoot(t *testing.T) {
	binding := Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	for _, invalid := range []Reservation{
		{Owner: Owner{}, BootID: testBoot, Resource: testNIC},
		{Owner: owner, Resource: testNIC},
	} {
		state := newState(binding.NodeUID, binding.VMID)
		state.Reservations[testIP] = invalid
		if _, _, err := readState(testNode(t, state), binding); err == nil {
			t.Fatalf("invalid reservation accepted: %+v", invalid)
		}
	}
}

func TestReconcileDoesNotWriteUnchangedState(t *testing.T) {
	a, _ := testAllocator(t)
	ctx := context.Background()
	if err := a.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if callErr := a.Reconcile(ctx); callErr != nil {
		t.Fatal(callErr)
	}
	after, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if before.GetResourceVersion() != after.GetResourceVersion() {
		t.Fatal("idle reconciliation rewrote unchanged API state")
	}
}
