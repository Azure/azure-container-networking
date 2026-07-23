package nicnetworkconfig

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager registers a noop NICNetworkConfig reconciler. Its only
// purpose is to start and keep the controller-runtime informer running so the
// manager cache stays warm for NICNetworkConfig objects, which the
// K8sSWIFTv2Middleware reads via the cached client.
//
// The predicate scopes reconcile events to NICNetworkConfigs whose Spec.NodeName
// matches this node. The reconciler is a noop today, so this has no functional
// effect yet; it keeps the controller scoped to this node's objects for when
// reconcile logic is added. Note it filters only reconcile events, not what the
// informer watches/caches (the middleware still node-filters its cached reads).
func SetupWithManager(mgr ctrl.Manager, nodeName string) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NICNetworkConfig{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			nicnc, ok := object.(*v1alpha1.NICNetworkConfig)
			if !ok {
				return false
			}
			// Filter to only reconcile NICNetworkConfigs for this node.
			// TODO - If we set owner references on NICNetworkConfigs to the Node CRD, we could use that instead of filtering by node name.
			return nicnc.Spec.NodeName == nodeName
		})).
		Complete(reconcile.Func(func(context.Context, ctrl.Request) (ctrl.Result, error) { return ctrl.Result{}, nil }))
	return errors.Wrap(err, "failed to set up nicnetworkconfig reconciler")
}
