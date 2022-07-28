package clustersubnetstate

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type cssClient interface {
	Get(context.Context, types.NamespacedName) (*v1alpha1.ClusterSubnetState, error)
}

type Reconciler struct {
	Cli  cssClient
	Sink chan<- v1alpha1.ClusterSubnetState
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	css, err := r.Cli.Get(ctx, req.NamespacedName)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.Sink <- *css
	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ClusterSubnetState{}).
		Complete(r)
}
