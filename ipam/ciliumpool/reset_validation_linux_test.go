package ciliumpool

import (
	"context"
	"strings"
	"testing"
)

func TestMalformedBootCannotBeTreatedAsPreviousBoot(t *testing.T) {
	binding := Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	state := newState()
	state.Reservations[testIP] = Reservation{
		Owner:  Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName},
		BootID: "malformed", Resource: testNIC,
	}
	if _, err := readState(testNode(t, state), binding); err == nil {
		t.Fatal("unrecognized remote boot identity accepted for automatic retirement")
	}
	a, _ := testAllocator(t)
	a.BootID = "malformed"
	if err := a.Reconcile(context.Background()); err == nil {
		t.Fatal("unrecognized current boot identity accepted for reconciliation")
	}
}

func TestConfigRejectsAllocationWatermarkOverflow(t *testing.T) {
	raw := strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"minAllocate":9223372036854775807,"maxAboveWatermark":9223372036854775807`, 1)
	if _, _, err := ParseConfig([]byte(raw)); err == nil {
		t.Fatal("overflowing watermark values accepted")
	}
}
