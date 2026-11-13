package ciliumpool

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLedgerRejectsMalformedPendingAndWithdrawalState(t *testing.T) {
	binding := Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	for _, tt := range []struct {
		name   string
		change func(*State)
	}{
		{"wrong pending key", func(s *State) {
			s.Pending["wrong"] = Pending{Owner: owner, BootID: testBoot, Expires: time.Now().Add(time.Minute)}
		}},
		{"missing pending expiry", func(s *State) { s.Pending[owner.key()] = Pending{Owner: owner, BootID: testBoot} }},
		{"invalid withdrawal", func(s *State) { s.Withdrawn["not-an-ip"] = true }},
		{"withdrawn retained address", func(s *State) { s.Withdrawn[testIP] = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := newState(binding.NodeUID, binding.VMID)
			tt.change(&state)
			if _, _, err := readState(testNode(t, state), binding); err == nil {
				t.Fatal("malformed remote state accepted")
			}
		})
	}
	state := newState(binding.NodeUID, binding.VMID)
	for i := range 256 {
		o := Owner{Network: testNetwork, Sandbox: strconv.Itoa(i), Interface: testIfName}
		state.Pending[o.key()] = Pending{Owner: o, BootID: testBoot, Expires: time.Now().Add(time.Minute)}
	}
	if _, _, err := readState(testNode(t, state), binding); err == nil {
		t.Fatal("unbounded pending ledger accepted")
	}
}

func TestLegacyReleaseHandshakeAndWrongPoolRequestAreRejected(t *testing.T) {
	binding := Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	for _, tt := range []struct {
		name   string
		mutate func(*unstructured.Unstructured)
	}{
		{"release handshake", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedMap(n.Object, map[string]any{testIP: "ready-for-release"}, statusField, ipamField, "release-ips")
		}},
		{"foreign pool", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(n.Object, []any{map[string]any{poolField: testForeign, "needed": map[string]any{"ipv4-addrs": int64(1)}}}, specField, ipamField, poolsField, "requested")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode(t, newState(binding.NodeUID, binding.VMID))
			tt.mutate(n)
			if _, _, err := readState(n, binding); err == nil {
				t.Fatal("mixed or malformed protocol accepted")
			}
		})
	}
}

func TestPendingMutationLimitFailsWithoutDroppingDemand(t *testing.T) {
	state := newState(testNodeUID, "vm")
	for i := range 255 {
		owner := Owner{Network: testNetwork, Sandbox: strconv.Itoa(i), Interface: testIfName}
		if err := state.Queue(owner, testBoot, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	before, err := json.Marshal(state.Pending)
	if err != nil {
		t.Fatal(err)
	}
	if callErr := state.Queue(Owner{Network: testNetwork, Sandbox: "overflow", Interface: testIfName}, testBoot, time.Now()); callErr == nil {
		t.Fatal("pending limit ignored")
	}
	after, err := json.Marshal(state.Pending)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("limit handling discarded existing demand")
	}
}
