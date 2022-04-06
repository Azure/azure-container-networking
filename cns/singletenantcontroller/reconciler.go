package kubecontroller

import (
	"context"
	"sync"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type nodeNetworkConfigListener interface {
	Update(*v1alpha.NodeNetworkConfig) error
}

type nncGetter interface {
	Get(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error)
}

// Reconciler watches for CRD status changes
type Reconciler struct {
	nncListeners []nodeNetworkConfigListener
	nnccli       nncGetter
	once         sync.Once
	started      chan interface{}
}

// NewReconciler creates a NodeNetworkConfig Reconciler which will get updates from the Kubernetes
// apiserver for NNC events.
// Provided nncListeners are passed the NNC after the Reconcile preprocesses it. Note: order matters! The
// passed Listeners are notified in the order provided.
func NewReconciler(nnccli nncGetter, nncListeners ...nodeNetworkConfigListener) *Reconciler {
	return &Reconciler{
		nncListeners: nncListeners,
		nnccli:       nnccli,
		started:      make(chan interface{}),
	}
}

// Reconcile is called on CRD status changes
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	nnc, err := r.nnccli.Get(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, errors.Wrapf(client.IgnoreNotFound(err), "NodeNetworkConfig %v not found", req.NamespacedName)
		}
		logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
		return reconcile.Result{}, errors.Wrapf(err, "failed to get NodeNetworkConfig %v", req.NamespacedName)
	}

	logger.Printf("[cns-rc] CRD Spec: %v", nnc.Spec)

	// If there are no network containers, don't continue to updating Listeners
	if len(nnc.Status.NetworkContainers) == 0 {
		logger.Errorf("[cns-rc] Empty NetworkContainers")
		return reconcile.Result{}, nil
	}

	// push the NNC to the registered NNC Sinks
	for i := range r.nncListeners {
		if err := r.nncListeners[i].Update(nnc); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "nnc listener return error during update")
		}
	}

	// we have received and pushed an NNC update, we are "Started"
	r.once.Do(func() { close(r.started) })
	return reconcile.Result{}, nil
}

// Started blocks until the Reconciler has reconciled at least once,
// then, and any time that it is called after that, it immediately returns true.
// It accepts a cancellable Context and if the context is closed
// before Start it will return false. Passing a closed Context after the
// Reconciler is started is indeterminate and the response is psuedorandom.
func (r *Reconciler) Started(ctx context.Context) bool {
	select {
	case <-r.started:
		return true
	case <-ctx.Done():
		return false
	}
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, node *v1.Node) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha.NodeNetworkConfig{}).
		WithEventFilter(predicate.Funcs{
			// ignore delete events.
			DeleteFunc: func(event.DeleteEvent) bool {
				return false
			},
		}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// match on node controller ref for all other events.
			return metav1.IsControlledBy(object, node)
		})).
		WithEventFilter(predicate.Funcs{
			// check that the generation is the same - status changes don't update generation.a
			UpdateFunc: func(ue event.UpdateEvent) bool {
				return ue.ObjectOld.GetGeneration() == ue.ObjectNew.GetGeneration()
			},
		}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed to set up reconciler with manager")
	}
	return nil
}
