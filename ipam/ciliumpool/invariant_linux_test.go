package ciliumpool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMutationsRejectInvalidIdentityAndReservationOverflow(t *testing.T) {
	state := newState(testNodeName, "vm")
	for i := range 255 {
		state.Reservations[fmt.Sprintf("10.0.0.%d", i)] = Reservation{Owner: Owner{Network: testNetwork, Sandbox: strconv.Itoa(i), Interface: testIfName}, BootID: testBoot, Resource: testNIC}
	}
	if _, err := state.Reserve(Owner{Network: testNetwork, Sandbox: "overflow", Interface: testIfName}, testBoot, testNIC, []string{"10.0.1.1"}); err == nil {
		t.Fatal("reservation limit exceeded")
	}
	empty := newState(testNodeName, "vm")
	if err := empty.Release(Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}, "not-an-ip"); err == nil {
		t.Fatal("malformed release address reported success")
	}
	if _, err := empty.Reserve(Owner{Network: strings.Repeat("x", 257), Sandbox: testSandbox, Interface: testIfName}, testBoot, testNIC, []string{testIP}); err == nil {
		t.Fatal("unbounded owner accepted")
	}
}

func TestAPIRejectsSparePolicyDrift(t *testing.T) {
	a, api := testAllocator(t)
	if err := unstructured.SetNestedField(api.node.Object, int64(9), specField, ipamField, "pre-allocate"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Add(context.Background(), Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}); err == nil {
		t.Fatal("consumer demand used a different spare buffer than operator")
	}
}
