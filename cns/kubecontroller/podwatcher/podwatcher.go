package podwatcher

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type podcli interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

type podListener interface {
	Update([]v1.Pod)
}

type podWatcher struct {
	cli       podcli
	listOpt   client.ListOption
	listeners []podListener
}

func New(nodename string, listeners ...podListener) *podWatcher { //nolint:revive // private struct to force constructor
	return &podWatcher{
		listeners: listeners,
		listOpt:   &client.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodename})},
	}
}

func (p *podWatcher) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	podList := &v1.PodList{}
	if err := p.cli.List(ctx, podList, p.listOpt); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to list pods")
	}
	for _, l := range p.listeners {
		l.Update(podList.Items)
	}
	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (p *podWatcher) SetupWithManager(mgr ctrl.Manager) error {
	p.cli = mgr.GetClient()
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		WithEventFilter(predicate.Funcs{ // we only want create/delete events
			UpdateFunc: func(event.UpdateEvent) bool {
				return false
			},
			GenericFunc: func(event.GenericEvent) bool {
				return false
			},
		}).
		Complete(p)
	if err != nil {
		return errors.Wrap(err, "failed to set up pod watcher with manager")
	}
	return nil
}
