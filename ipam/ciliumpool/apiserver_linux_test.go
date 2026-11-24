package ciliumpool

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestCiliumNodeAPIContract(t *testing.T) {
	local := false
	env := &envtest.Environment{
		UseExistingCluster: &local, ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:           []string{"testdata/ciliumnodes-v1.20.1.yaml"},
		BinaryAssetsDirectory:       t.TempDir(),
		DownloadBinaryAssets:        os.Getenv("KUBEBUILDER_ASSETS") == "",
		DownloadBinaryAssetsVersion: "1.33.0",
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stopping API server: %v", err)
		}
	})
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	nodes := client.Resource(CiliumNodes)
	ctx := t.Context()
	fixture, err := testOperatorNode()
	if err != nil {
		t.Fatal(err)
	}
	node := fixture.DeepCopy()
	node.SetUID("")
	node.SetResourceVersion("")
	delete(node.Object, statusField)
	node, err = nodes.Create(ctx, node, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	node.Object[statusField] = fixture.Object[statusField]
	node, err = nodes.UpdateStatus(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := &Allocator{
		Nodes: nodes, NodeName: testNodeName, BootID: testBoot, PreAllocate: 2,
		Binding: Binding{
			NodeUID: testNodeUID, VMID: "vm", Resource: testNIC, Subnet: testSubnet,
			NodeName: testNodeName, InstanceID: testInstanceID, Primary: testPrimary, MAC: testMAC,
		},
		Supply: map[string]bool{testIP: true, testOtherIP: true},
	}
	owner := Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}
	ip, err := a.Add(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, updateErr := nodes.UpdateStatus(ctx, node, metav1.UpdateOptions{}); !apierrors.IsConflict(updateErr) {
		t.Fatalf("stale operator update was not rejected: %v", updateErr)
	}
	node, err = nodes.Get(ctx, testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := readState(node, a.Binding)
	if err != nil || state.Reservations[ip].Owner != owner {
		t.Fatalf("committed owner did not survive the CRD: %+v, %v", state, err)
	}
	stalePatch, err := usagePatch(node, newState(), a.Binding.VMID)
	if err != nil {
		t.Fatal(err)
	}
	if setErr := unstructured.SetNestedField(node.Object, "operator diagnostic", statusField, ipamField, "operator-status", "error"); setErr != nil {
		t.Fatal(setErr)
	}
	if _, updateErr := nodes.UpdateStatus(ctx, node, metav1.UpdateOptions{}); updateErr != nil {
		t.Fatal(updateErr)
	}
	assertAPIRejectsPatch(t, nodes, stalePatch)
	other := Owner{Network: testNetwork, Sandbox: "other-sandbox", Interface: testIfName}
	otherIP, err := a.Add(ctx, other)
	if err != nil || otherIP == ip {
		t.Fatalf("second reservation = %q, %v", otherIP, err)
	}
	if deleteErr := a.Delete(ctx, owner, ip); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	node, err = nodes.Get(ctx, testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err = readState(node, a.Binding)
	if err != nil || len(state.Reservations) != 1 || state.Reservations[otherIP].Owner != other {
		t.Fatalf("DEL lost another owner: %+v, %v", state, err)
	}
	assertOperatorState(t, fixture, node)
	node.SetUID(types.UID("replaced-object"))
	stalePatch, err = usagePatch(node, newState(), a.Binding.VMID)
	if err != nil {
		t.Fatal(err)
	}
	assertAPIRejectsPatch(t, nodes, stalePatch)
	if deleteErr := a.Delete(ctx, other, otherIP); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	node, err = nodes.Get(ctx, testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state, err = readState(node, a.Binding)
	if err != nil || len(state.Reservations) != 0 {
		t.Fatalf("DEL did not persist: %+v, %v", state, err)
	}
}

func assertOperatorState(t *testing.T, fixture, node *unstructured.Unstructured) {
	t.Helper()
	if diff := cmp.Diff(fixture.Object[specField], node.Object[specField]); diff != "" {
		t.Fatalf("operator pool/spec changed (-want +got): %s", diff)
	}
	interfaces, _, err := unstructured.NestedSlice(node.Object, statusField, testNetwork, "interfaces")
	wantInterfaces, _, wantErr := unstructured.NestedSlice(fixture.Object, statusField, testNetwork, "interfaces")
	if err != nil || wantErr != nil || !cmp.Equal(wantInterfaces, interfaces) {
		t.Fatalf("operator interfaces changed: %s (%v, %v)", cmp.Diff(wantInterfaces, interfaces), err, wantErr)
	}
	diagnostic, _, err := unstructured.NestedString(node.Object, statusField, ipamField, "operator-status", "error")
	if err != nil || diagnostic != "operator diagnostic" {
		t.Fatalf("operator status lost: %q, %v", diagnostic, err)
	}
}

func assertAPIRejectsPatch(t *testing.T, nodes dynamic.ResourceInterface, patch []byte) {
	t.Helper()
	before, err := nodes.Get(t.Context(), testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, patchErr := nodes.Patch(t.Context(), testNodeName, types.JSONPatchType, patch, metav1.PatchOptions{}, statusField); !apierrors.IsInvalid(patchErr) && !apierrors.IsConflict(patchErr) {
		t.Fatalf("stale conditional patch was not rejected: %v", patchErr)
	}
	after, err := nodes.Get(t.Context(), testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(before.Object, after.Object); diff != "" {
		t.Fatalf("rejected patch changed the resource: %s", diff)
	}
}
