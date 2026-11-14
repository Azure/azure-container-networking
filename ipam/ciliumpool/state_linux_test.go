package ciliumpool

import (
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReserveSelectsFreeAddressAndReplaysOwner(t *testing.T) {
	first := Owner{Network: testNetwork, Sandbox: "sandbox-a", Interface: testIfName}
	second := Owner{Network: testNetwork, Sandbox: "sandbox-b", Interface: testIfName}
	state := State{Reservations: map[string]Reservation{}}
	supply := []string{testOtherIP, testIP}
	ip, err := state.Reserve(first, testBootA, testNIC, supply)
	if err != nil || ip != testIP {
		t.Fatalf("first reservation = %q, %v; want .10", ip, err)
	}
	replayed, err := state.Reserve(first, testBootA, testNIC, supply)
	if err != nil || replayed != ip {
		t.Fatalf("replayed reservation = %q, %v; want %q", replayed, err, ip)
	}
	next, err := state.Reserve(second, testBootA, testNIC, supply)
	if err != nil || next != testOtherIP {
		t.Fatalf("second owner = %q, %v; want .11", next, err)
	}
	want := map[string]Reservation{testIP: {Owner: first, BootID: testBootA, Resource: testNIC}, testOtherIP: {Owner: second, BootID: testBootA, Resource: testNIC}}
	if diff := cmp.Diff(want, state.Reservations); diff != "" {
		t.Fatalf("reservation mismatch (-want +got):\n%s", diff)
	}
}

func TestReserveRejectsInvalidOwnershipWithoutMutation(t *testing.T) {
	owner := Owner{Network: testNetwork, Sandbox: "sandbox-a", Interface: testIfName}
	for _, tt := range []struct {
		name         string
		owner        Owner
		reservations map[string]Reservation
	}{
		{"withdrawn", owner, map[string]Reservation{"10.0.0.9": {Owner: owner, BootID: testBootA, Resource: testNIC}}},
		{"duplicate", owner, map[string]Reservation{testIP: {Owner: owner, BootID: testBootA, Resource: testNIC}, testOtherIP: {Owner: owner, BootID: testBootA, Resource: testNIC}}},
		{"missing sandbox", Owner{Network: testNetwork, Interface: testIfName}, map[string]Reservation{}},
		{"previous boot", owner, map[string]Reservation{testIP: {Owner: owner, BootID: "boot-old", Resource: testNIC}}},
		{"foreign resource", owner, map[string]Reservation{testIP: {Owner: owner, BootID: testBootA, Resource: testForeign}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := State{Reservations: tt.reservations}
			before := maps.Clone(state.Reservations)
			if ip, err := state.Reserve(tt.owner, testBootA, testNIC, []string{testIP, testOtherIP}); err == nil {
				t.Fatalf("Reserve returned %q without rejecting %s", ip, tt.name)
			}
			if diff := cmp.Diff(before, state.Reservations); diff != "" {
				t.Fatalf("rejected reservation changed ownership:\n%s", diff)
			}
		})
	}
}
