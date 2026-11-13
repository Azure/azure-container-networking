package ciliumpool

import (
	"net"
	"net/netip"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRegistrationBindsNodeVMWithoutConsumerPoolState(t *testing.T) {
	_, cfg, err := ParseConfig([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	guest := Guest{
		VMID: "vm-id", BootID: testBoot, InstanceID: "/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm",
		Primary: netip.MustParseAddr(testPrimary), MAC: net.HardwareAddr{0, 17, 34, 51, 68, 85},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: cfg.NodeName, UID: testNodeUID},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testPrimary}}},
	}
	cn, binding, err := registration(cfg, node, guest)
	if err != nil {
		t.Fatal(err)
	}
	cn.SetUID("object")
	cn.SetResourceVersion("1")
	if callErr := validateCiliumNode(cn, binding); callErr != nil {
		t.Fatal(callErr)
	}
	state, err := readState(cn, binding)
	if err != nil || len(state.Reservations) != 0 {
		t.Fatalf("initial contract = %+v, %v", state, err)
	}
	if _, found, _ := unstructured.NestedMap(cn.Object, specField, ipamField, "pools"); found {
		t.Fatal("registration created multi-pool state")
	}
	if _, found, _ := unstructured.NestedMap(cn.Object, specField, ipamField, poolField); found {
		t.Fatal("registration wrote operator inventory")
	}
	for _, tt := range []struct {
		name   string
		change func(*unstructured.Unstructured)
	}{
		{"VM incarnation", func(n *unstructured.Unstructured) {
			a := n.GetAnnotations()
			a["azure-container-networking.io/vm-id"] = testForeign
			n.SetAnnotations(a)
		}},
		{"node owner", func(n *unstructured.Unstructured) {
			o := n.GetOwnerReferences()
			o[0].UID = "replacement"
			n.SetOwnerReferences(o)
		}},
		{"instance", func(n *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(n.Object, testForeign, specField, "instance-id")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := cn.DeepCopy()
			tt.change(changed)
			if err := validateCiliumNode(changed, binding); err == nil {
				t.Fatal("foreign binding accepted")
			}
		})
	}
	node.Status.Addresses[0].Address = "10.0.0.99"
	if _, _, err := registration(cfg, node, guest); err == nil {
		t.Fatal("guest primary differs from Node but registration succeeded")
	}
}
