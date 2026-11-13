package ciliumpool

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAPIRejectsOperatorMetadataDriftBeforeReservation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		path  []string
		value any
	}{
		{macField, []string{macField}, "00:aa:bb:cc:dd:ee"},
		{"primary", []string{"ip"}, "10.0.0.99"},
		{subnetField, []string{subnetField, cidrField}, "10.1.0.0/24"},
		{gatewayField, []string{gatewayField}, "0.0.0.0"},
		{"provisioning", []string{stateField}, "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, api := testAllocator(t)
			interfaces, _, err := unstructured.NestedSlice(api.node.Object, statusField, testNetwork, "interfaces")
			if err != nil {
				t.Fatal(err)
			}
			iface := interfaces[0].(map[string]any)
			if err := unstructured.SetNestedField(iface, tt.value, tt.path...); err != nil {
				t.Fatal(err)
			}
			if err := unstructured.SetNestedSlice(api.node.Object, interfaces, statusField, testNetwork, "interfaces"); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Add(context.Background(), Owner{Network: testNetwork, Sandbox: testSandbox, Interface: testIfName}); err == nil {
				t.Fatal("foreign or incomplete operator metadata admitted")
			}
		})
	}
}
