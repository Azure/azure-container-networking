package multitenantpodnetworkconfig

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager registers a noop MTPNC reconciler
func SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MultitenantPodNetworkConfig{}).
		Complete(reconcile.Func(func(context.Context, ctrl.Request) (ctrl.Result, error) { return ctrl.Result{}, nil }))
	return errors.Wrap(err, "failed to set up mtpnc reconciler")
}
