package ciliumpool

import (
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReleaseByOwnerIsIdempotentAndDoesNotReleaseReplacement(t *testing.T) {
	old := Owner{Network: testNetwork, Sandbox: testOld, Interface: testIfName}
	replacement := Owner{Network: testNetwork, Sandbox: "replacement", Interface: testIfName}
	state := State{Reservations: map[string]Reservation{testIP: {Owner: old, BootID: testBoot, Resource: testNIC}}}
	if err := state.Release(old, ""); err != nil {
		t.Fatal(err)
	}
	if len(state.Reservations) != 0 {
		t.Fatal("owner-only DEL did not release its claim")
	}
	state.Reservations[testIP] = Reservation{Owner: replacement, BootID: testBoot, Resource: testNIC}
	before := maps.Clone(state.Reservations)
	for _, address := range []string{"", testIP} {
		if err := state.Release(old, address); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(before, state.Reservations); diff != "" {
			t.Fatalf("old DEL changed replacement: %s", diff)
		}
	}
}

func TestReleaseRejectsWrongAddressBeforeMutation(t *testing.T) {
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	state := State{Reservations: map[string]Reservation{testIP: {Owner: owner, BootID: testBoot, Resource: testNIC}}}
	before := maps.Clone(state.Reservations)
	if err := state.Release(owner, testOtherIP); err == nil {
		t.Fatal("release accepted another address")
	}
	if diff := cmp.Diff(before, state.Reservations); diff != "" {
		t.Fatalf("rejected release mutated reservations: %s", diff)
	}
}

func TestRebootRetiresOnlyPreviousBootReservations(t *testing.T) {
	state := State{Reservations: map[string]Reservation{
		testIP:      {Owner: Owner{Network: testNetwork, Sandbox: testOld, Interface: testIfName}, BootID: testOld, Resource: testNIC},
		testOtherIP: {Owner: Owner{Network: testNetwork, Sandbox: "current", Interface: testIfName}, BootID: "current", Resource: testNIC},
	}}
	want := map[string]Reservation{testOtherIP: state.Reservations[testOtherIP]}
	for range 2 {
		if err := state.RetireBoot("current"); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, state.Reservations); diff != "" {
			t.Fatalf("reboot changed current ownership: %s", diff)
		}
	}
	if err := state.RetireBoot(""); err == nil {
		t.Fatal("missing boot ID accepted as a reset")
	}
}
