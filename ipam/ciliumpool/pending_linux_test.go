package ciliumpool

import (
	"testing"
	"time"
)

func TestPendingDemandDeduplicatesExpiresAndNeverExpiresReservations(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	owner := Owner{Network: testNetwork, Sandbox: "waiting", Interface: testIfName}
	state := newState(testNodeName, "vm")
	state.Reservations[testIP] = Reservation{
		Owner: Owner{Network: testNetwork, Sandbox: "active", Interface: testIfName}, BootID: testBoot, Resource: testNIC,
	}
	for range 2 {
		if err := state.Queue(owner, testBoot, now); err != nil {
			t.Fatal(err)
		}
	}
	if len(state.Pending) != 1 {
		t.Fatal("duplicate retry inflated pending demand")
	}
	state.ExpirePending(now.Add(4 * time.Minute))
	if len(state.Pending) != 1 {
		t.Fatal("pending request expired too early")
	}
	state.ExpirePending(now.Add(6 * time.Minute))
	if len(state.Pending) != 0 || len(state.Reservations) != 1 {
		t.Fatal("expiry failed to retire pending demand or expired an active reservation")
	}
	if err := state.Queue(owner, testBoot, now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Reserve(owner, testBoot, testNIC, []string{testOtherIP}); err != nil {
		t.Fatal(err)
	}
	if len(state.Pending) != 0 {
		t.Fatal("successful allocation left duplicate pending demand")
	}
	if err := state.Queue(Owner{Network: testNetwork, Sandbox: "old-pending", Interface: testIfName}, testOld, now); err != nil {
		t.Fatal(err)
	}
	if err := state.RetireBoot(testBoot); err != nil {
		t.Fatal(err)
	}
	if len(state.Pending) != 0 {
		t.Fatal("reboot retained old pending requests")
	}
}
