package ciliumpool

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStableUsagePatchPreservesOperatorPoolAndStatus(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		apiVersionField: ciliumAPIVersion, kindField: ciliumNodeKind,
		metadataField: map[string]any{uidField: testObjectUID, resourceVersionField: "7", annotationsField: map[string]any{"unrelated": testKeep}},
		specField: map[string]any{ipamField: map[string]any{poolField: map[string]any{
			testIP: map[string]any{resourceField: testNIC}, testOtherIP: map[string]any{resourceField: testNIC},
		}}}, statusField: map[string]any{"azure": map[string]any{"diagnostic": testKeep}, ipamField: map[string]any{"operator-status": map[string]any{"error": testKeep}}},
	}}
	state := State{Reservations: map[string]Reservation{
		testIP: {Owner: Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}, BootID: testBoot, Resource: testNIC},
	}}
	data, err := usagePatch(node, state, "vm")
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
	var got unstructured.Unstructured
	if decodeErr := got.UnmarshalJSON(after); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if diff := cmp.Diff(node.Object[specField], got.Object[specField]); diff != "" {
		t.Fatalf("operator spec changed: %s", diff)
	}
	if diff := cmp.Diff(node.GetAnnotations(), got.GetAnnotations()); diff != "" {
		t.Fatalf("annotation ledger introduced: %s", diff)
	}
	used, _, err := unstructured.NestedMap(got.Object, statusField, ipamField, "used")
	if err != nil {
		t.Fatal(err)
	}
	wantOwner := "azure-cilium-ipam/v1/vm/" + testBoot + "/azure/sandbox/eth0"
	if diff := cmp.Diff(map[string]any{testIP: map[string]any{ownerField: wantOwner, resourceField: testNIC}}, used); diff != "" {
		t.Fatalf("stable used-map contract differs: %s", diff)
	}
	for _, path := range [][]string{{statusField, "azure", "diagnostic"}, {statusField, ipamField, "operator-status", "error"}} {
		v, _, err := unstructured.NestedString(got.Object, path...)
		if err != nil || v != testKeep {
			t.Fatalf("operator field changed: %v", path)
		}
	}
	node.SetResourceVersion("8")
	changed, _ := node.MarshalJSON()
	if _, err := patch.Apply(changed); err == nil {
		t.Fatal("stale writer accepted")
	}
}

func TestEmptyStableUsageIsValidWithoutLocalOrRemoteLedger(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		apiVersionField: ciliumAPIVersion, kindField: ciliumNodeKind,
		metadataField: map[string]any{uidField: testObjectUID, resourceVersionField: "1"},
		specField:     map[string]any{ipamField: map[string]any{poolField: map[string]any{}}},
	}}
	state, err := readState(node, Binding{VMID: "vm", Resource: testNIC, Subnet: testSubnet})
	if err != nil || len(state.Reservations) != 0 {
		t.Fatalf("empty status failed: %+v, %v", state, err)
	}
	data, err := usagePatch(node, State{Reservations: map[string]Reservation{}}, "vm")
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		var ops []patchOperation
		if err := json.Unmarshal(data, &ops); err != nil {
			t.Fatal(err)
		}
		for _, op := range ops {
			if op.Op != "test" && op.Path != "/status" && op.Path != "/status/ipam" && op.Path != "/status/ipam/used" {
				t.Fatalf("non-status write: %+v", op)
			}
		}
	}
}
