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
	if os.Getenv("ACN_CILIUM_ENVTEST") != "1" {
		t.Skip("set ACN_CILIUM_ENVTEST=1 to run the local API-server contract test")
	}
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
	fixture := testOperatorNode(t)
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
	node = readAPINode(t, nodes)
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
	node = readAPINode(t, nodes)
	state, err = readState(node, a.Binding)
	if err != nil || len(state.Reservations) != 1 || state.Reservations[otherIP].Owner != other {
		t.Fatalf("DEL lost another owner: %+v, %v", state, err)
	}
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
	node.SetUID(types.UID("replaced-object"))
	stalePatch, err = usagePatch(node, newState(), a.Binding.VMID)
	if err != nil {
		t.Fatal(err)
	}
	assertAPIRejectsPatch(t, nodes, stalePatch)
	if deleteErr := a.Delete(ctx, other, otherIP); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	state, err = readState(readAPINode(t, nodes), a.Binding)
	if err != nil || len(state.Reservations) != 0 {
		t.Fatalf("DEL did not persist: %+v, %v", state, err)
	}
}

func readAPINode(t *testing.T, nodes dynamic.ResourceInterface) *unstructured.Unstructured {
	t.Helper()
	node, err := nodes.Get(t.Context(), testNodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func assertAPIRejectsPatch(t *testing.T, nodes dynamic.ResourceInterface, patch []byte) {
	t.Helper()
	before := readAPINode(t, nodes)
	if _, err := nodes.Patch(t.Context(), testNodeName, types.JSONPatchType, patch, metav1.PatchOptions{}, statusField); !apierrors.IsInvalid(err) && !apierrors.IsConflict(err) {
		t.Fatalf("stale conditional patch was not rejected: %v", err)
	}
	if diff := cmp.Diff(before.Object, readAPINode(t, nodes).Object); diff != "" {
		t.Fatalf("rejected patch changed the resource: %s", diff)
	}
}
