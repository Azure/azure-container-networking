package nicnetworkconfig

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager registers a noop NICNetworkConfig reconciler. Its only
// purpose is to start and keep the controller-runtime informer running so the
// manager cache stays warm for NICNetworkConfig objects, which the
// K8sSWIFTv2Middleware reads via the cached client.
func SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NICNetworkConfig{}).
		Complete(reconcile.Func(func(context.Context, ctrl.Request) (ctrl.Result, error) { return ctrl.Result{}, nil }))
	return errors.Wrap(err, "failed to set up nicnetworkconfig reconciler")
}
