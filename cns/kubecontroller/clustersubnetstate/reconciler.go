package clustersubnetstate

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
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
		cssReconcilerErrorCount.With(prometheus.Labels{cssReconcilerCRDWatcherStateLabel: "failed"}).Inc()
		return reconcile.Result{}, errors.Wrapf(err, "failed to get css %s", req.String())
	}
	cssReconcilerErrorCount.With(prometheus.Labels{cssReconcilerCRDWatcherStateLabel: "succeeded"}).Inc()
	r.Sink <- *css
	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ClusterSubnetState{}).
		Complete(r)
	return errors.Wrap(err, "failed to setup clustersubnetstate reconciler with manager")
}
