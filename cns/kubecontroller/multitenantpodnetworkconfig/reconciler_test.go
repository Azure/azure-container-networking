package multitenantpodnetworkconfig

import (
	"context"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcileMTPNC(t *testing.T) {
	result, err := reconcileMTPNC(context.Background(), ctrl.Request{})
	if err != nil {
		t.Fatalf("reconcileMTPNC() error = %v, want nil", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("reconcileMTPNC() result = %v, want empty result", result)
	}
}
