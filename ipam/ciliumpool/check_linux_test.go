package ciliumpool

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckValidatesReservationWithoutMutatingState(t *testing.T) {
	a, _ := testAllocator(t)
	ctx := context.Background()
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	if _, err := a.Add(ctx, owner); err != nil {
		t.Fatal(err)
	}
	before, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if callErr := a.Check(ctx, owner); callErr != nil {
		t.Fatal(callErr)
	}
	if callErr := a.Check(ctx, Owner{Network: testNetwork, Sandbox: "missing", Interface: testIfName}); callErr == nil {
		t.Fatal("CHECK accepted missing reservation")
	}
	after, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if before.GetResourceVersion() != after.GetResourceVersion() {
		t.Fatal("CHECK mutated remote state")
	}
}
