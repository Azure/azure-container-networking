package ciliumpool

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testNode(t *testing.T, state State) *unstructured.Unstructured {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cilium.io/v2", "kind": "CiliumNode",
		"metadata": map[string]any{
			"name": testNodeName, "uid": "object", "resourceVersion": "1",
			"annotations": map[string]any{stateAnnotation: string(data)},
		}, specField: map[string]any{ipamField: map[string]any{poolsField: map[string]any{
			"requested": []any{map[string]any{poolField: defaultPool, "needed": map[string]any{"ipv4-addrs": int64(2)}}},
			"allocated": []any{map[string]any{poolField: defaultPool, "cidrs": []any{testPrefix}}},
		}}}, statusField: map[string]any{ipamField: map[string]any{"used": map[string]any{}}},
	}}
}

func TestReadStateRejectsCorruptOrForeignOwnership(t *testing.T) {
	binding := Binding{NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet}
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	valid := newState(binding.NodeUID, binding.VMID)
	valid.Reservations[testIP] = Reservation{Owner: owner, BootID: testBoot, Resource: testNIC}
	for _, tt := range []struct {
		name   string
		change func(*unstructured.Unstructured)
	}{
		{"missing ledger", func(n *unstructured.Unstructured) { n.SetAnnotations(map[string]string{}) }},
		{"corrupt ledger", func(n *unstructured.Unstructured) { n.SetAnnotations(map[string]string{stateAnnotation: "{"}) }},
		{"null ledger", func(n *unstructured.Unstructured) { n.SetAnnotations(map[string]string{stateAnnotation: "null"}) }},
		{"foreign VM", func(n *unstructured.Unstructured) {
			n.SetAnnotations(map[string]string{stateAnnotation: strings.ReplaceAll(n.GetAnnotations()[stateAnnotation], `"vm"`, `"foreign"`)})
		}},
		{"legacy used", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedMap(n.Object, map[string]any{testIP: map[string]any{"owner": "legacy"}}, statusField, ipamField, "used")
		}},
		{"unretained reservation", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(n.Object, []any{}, specField, ipamField, poolsField, "allocated")
		}},
		{"foreign retained prefix", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedSlice(n.Object, []any{map[string]any{poolField: defaultPool, "cidrs": []any{"192.0.2.10/32"}}}, specField, ipamField, poolsField, "allocated")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode(t, valid)
			tt.change(n)
			if _, _, err := readState(n, binding); err == nil {
				t.Fatal("invalid remote state accepted")
			}
		})
	}
	if _, retained, err := readState(testNode(t, valid), binding); err != nil || len(retained) != 1 {
		t.Fatalf("valid reservation rejected: %v, %v", retained, err)
	}
}

func TestPatchRejectsOversizedAnnotations(t *testing.T) {
	n := testNode(t, newState(testNodeUID, "vm"))
	annotations := n.GetAnnotations()
	annotations["unrelated"] = strings.Repeat("x", 256*1024)
	n.SetAnnotations(annotations)
	if _, err := consumerPatch(n, newState(testNodeUID, "vm"), []string{}, 2); err == nil {
		t.Fatal("oversized annotation update was accepted")
	}
}
