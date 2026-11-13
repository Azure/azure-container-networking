package ciliumpool

import (
	"encoding/json"
	"testing"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConsumerPatchIsAtomicAndPreservesUnownedFields(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cilium.io/v2", "kind": "CiliumNode",
		"metadata": map[string]any{"uid": "object", "resourceVersion": "7", "annotations": map[string]any{"unrelated": testKeep}},
		specField: map[string]any{ipamField: map[string]any{
			"pre-allocate": int64(2), poolsField: map[string]any{"other": testKeep},
		}},
		statusField: map[string]any{testNetwork: map[string]any{"diagnostic": testKeep}, ipamField: map[string]any{"used": map[string]any{}}},
	}}
	state := State{Reservations: map[string]Reservation{testIP: {Owner: Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}, BootID: testBoot, Resource: testNIC}}}
	data, err := consumerPatch(node, state, []string{testPrefix, "10.0.0.11/32"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := jsonpatch.DecodePatch(data)
	if err != nil {
		t.Fatal(err)
	}
	before, err := node.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	after, err := patch.Apply(before)
	if err != nil {
		t.Fatal(err)
	}
	var updated unstructured.Unstructured
	if callErr := updated.UnmarshalJSON(after); callErr != nil {
		t.Fatal(callErr)
	}
	if updated.GetAnnotations()["unrelated"] != testKeep {
		t.Fatal("patch replaced unrelated annotation")
	}
	raw := updated.GetAnnotations()["azure-container-networking.io/ipam-state"]
	var got State
	if callErr := json.Unmarshal([]byte(raw), &got); callErr != nil || len(got.Reservations) != 1 {
		t.Fatalf("ledger did not commit: %s (%v)", raw, callErr)
	}
	requests, _, err := unstructured.NestedSlice(updated.Object, specField, ipamField, poolsField, "requested")
	if err != nil || len(requests) != 1 {
		t.Fatalf("request did not commit: %v, %v", requests, err)
	}
	request := requests[0].(map[string]any)
	needed := request["needed"].(map[string]any)
	if needed["ipv4-addrs"] != int64(3) {
		t.Fatalf("demand = %v, want one reservation + two spares", needed)
	}
	for _, path := range [][]string{{statusField, testNetwork, "diagnostic"}, {specField, ipamField, poolsField, "other"}} {
		value, _, callErr := unstructured.NestedString(updated.Object, path...)
		if callErr != nil || value != testKeep {
			t.Fatalf("unowned field %v was changed", path)
		}
	}
	used, _, err := unstructured.NestedMap(updated.Object, statusField, ipamField, "used")
	if err != nil || len(used) != 0 {
		t.Fatal("multi-pool patch populated legacy usage")
	}
	node.SetResourceVersion("8")
	newer, _ := node.MarshalJSON()
	if _, err := patch.Apply(newer); err == nil {
		t.Fatal("stale writer bypassed resourceVersion test")
	}
	node.SetResourceVersion("7")
	node.SetUID("replacement")
	replaced, _ := node.MarshalJSON()
	if _, err := patch.Apply(replaced); err == nil {
		t.Fatal("stale writer bypassed UID test")
	}
}
