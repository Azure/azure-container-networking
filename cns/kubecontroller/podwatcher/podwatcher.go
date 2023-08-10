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

type PodWatcher struct {
	cli            podcli
	listOpt        client.ListOption
	ReconcileFuncs []reconcile.Func
}

func New(nodename string) *PodWatcher { //nolint:revive // private struct to force constructor
	return &PodWatcher{
		listOpt: &client.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodename})},
	}
}

func (p *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	for _, f := range p.ReconcileFuncs {
		if _, err := f(ctx, req); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to reconcile")
		}
	}
	return reconcile.Result{}, nil
}

type PodFilter func([]v1.Pod) []v1.Pod

var PodNetworkFilter PodFilter = func(pods []v1.Pod) []v1.Pod {
	var filtered []v1.Pod
	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			filtered = append(filtered, pod)
		}
	}
	return filtered
}

func (p *PodWatcher) PodNotifierFunc(f PodFilter, listeners ...podListener) reconcile.Func {
	return func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
		podList := &v1.PodList{}
		if err := p.cli.List(ctx, podList, p.listOpt); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to list pods")
		}
		pods := podList.Items
		if f != nil {
			pods = f(pods)
		}
		for _, l := range listeners {
			l.Update(pods)
		}
		return reconcile.Result{}, nil
	}
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (p *PodWatcher) SetupWithManager(mgr ctrl.Manager) error {
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
