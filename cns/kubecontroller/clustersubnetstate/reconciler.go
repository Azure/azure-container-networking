package clustersubnetstate

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/clustersubnetstate"
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
	cli  cssClient
	sink chan<- v1alpha1.ClusterSubnetState
}

func New(sink chan<- v1alpha1.ClusterSubnetState) *Reconciler {
	return &Reconciler{
		sink: sink,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	css, err := r.cli.Get(ctx, req.NamespacedName)
	if err != nil {
		cssReconcilerErrorCount.With(prometheus.Labels{cssReconcilerCRDWatcherStateLabel: "failed"}).Inc()
		return reconcile.Result{}, errors.Wrapf(err, "failed to get css %s", req.String())
	}
	cssReconcilerErrorCount.With(prometheus.Labels{cssReconcilerCRDWatcherStateLabel: "succeeded"}).Inc()
	r.sink <- *css
	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.cli = clustersubnetstate.NewClient(mgr.GetClient())
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ClusterSubnetState{}).
		Complete(r)
	return errors.Wrap(err, "failed to setup clustersubnetstate reconciler with manager")
}
