package ciliumpool

import (
	"net/url"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testNode(t *testing.T, state State) *unstructured.Unstructured {
	t.Helper()
	used := map[string]any{}
	for ip, entry := range state.Reservations {
		parts := []string{"vm", entry.BootID, entry.Owner.Network, entry.Owner.Sandbox, entry.Owner.Interface}
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		used[ip] = map[string]any{ownerField: ownerPrefix + strings.Join(parts, "/"), resourceField: entry.Resource}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		apiVersionField: ciliumAPIVersion, kindField: ciliumNodeKind,
		metadataField: map[string]any{"name": testNodeName, uidField: testObjectUID, resourceVersionField: "1", annotationsField: map[string]any{}},
		specField: map[string]any{ipamField: map[string]any{poolField: map[string]any{
			testIP: map[string]any{resourceField: testNIC}, testOtherIP: map[string]any{resourceField: testNIC},
		}}},
		statusField: map[string]any{ipamField: map[string]any{"used": used}},
	}}
}

func TestStableStateRejectsForeignOrMalformedUsage(t *testing.T) {
	binding := Binding{VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	validOwner := ownerPrefix + "vm/" + testBoot + "/azure/sandbox/eth0"
	for _, tt := range []struct{ name, owner, resource, ip string }{
		{"unknown owner", "other", testNIC, testIP},
		{"foreign VM", strings.Replace(validOwner, "/vm/", "/foreign-vm/", 1), testNIC, testIP},
		{"foreign resource", validOwner, testForeign, testIP},
		{"foreign subnet", validOwner, testNIC, "192.0.2.10"},
		{"malformed boot", strings.Replace(validOwner, testBoot, "malformed", 1), testNIC, testIP},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode(t, newState())
			if err := unstructured.SetNestedMap(n.Object, map[string]any{tt.ip: map[string]any{ownerField: tt.owner, resourceField: tt.resource}}, statusField, ipamField, "used"); err != nil {
				t.Fatal(err)
			}
			if _, err := readState(n, binding); err == nil {
				t.Fatal("invalid usage accepted")
			}
		})
	}
}

func TestStableStateRejectsActiveMultiPoolContract(t *testing.T) {
	binding := Binding{VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	for _, annotation := range []bool{false, true} {
		n := testNode(t, newState())
		if annotation {
			n.SetAnnotations(map[string]string{"azure-container-networking.io/ipam-state": "{}"})
		} else if err := unstructured.SetNestedMap(n.Object, map[string]any{"requested": []any{}}, specField, ipamField, "pools"); err != nil {
			t.Fatal(err)
		}
		if _, err := readState(n, binding); err == nil {
			t.Fatal("mixed reservation authority accepted")
		}
	}
}

func TestStableOwnerEscapingAndWithdrawnClaimRelease(t *testing.T) {
	binding := Binding{VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	owner := Owner{Network: "azure/network", Sandbox: "sandbox/with%escape", Interface: testIfName}
	state := newState()
	state.Reservations[testIP] = Reservation{Owner: owner, BootID: testBoot, Resource: testNIC}
	n := testNode(t, state)
	if err := unstructured.SetNestedMap(n.Object, map[string]any{}, specField, ipamField, poolField); err != nil {
		t.Fatal(err)
	}
	restored, err := readState(n, binding)
	if err != nil || restored.Reservations[testIP].Owner != owner {
		t.Fatalf("owner round trip = %+v, %v", restored, err)
	}
	if err := restored.Release(owner, ""); err != nil {
		t.Fatal(err)
	}
	if len(restored.Reservations) != 0 {
		t.Fatal("DEL required the withdrawn address to remain in the pool")
	}
}
