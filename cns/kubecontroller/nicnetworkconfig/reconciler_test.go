package nicnetworkconfig

import (
	"context"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcileNICNC(t *testing.T) {
	result, err := reconcileNICNC(context.Background(), ctrl.Request{})
	if err != nil {
		t.Fatalf("reconcileNICNC() error = %v, want nil", err)
	}
	if result != (ctrl.Result{}) {
		t.Fatalf("reconcileNICNC() result = %v, want empty result", result)
	}
}
