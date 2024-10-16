package pod

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type cli interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

// watcher watches Pods on the current Node and notifies listeners of changes.
type watcher struct {
	z              *zap.Logger
	cli            cli
	reconcileFuncs []reconcile.Func
}

func New(z *zap.Logger) *watcher { //nolint:revive // force usage of new by keeping the struct private
	return &watcher{
		z: z.With(zap.String("component", "pod-watcher")),
	}
}

// With adds reconcile.Funcs to the Watcher.
func (p *watcher) With(fs ...reconcile.Func) *watcher {
	p.reconcileFuncs = append(p.reconcileFuncs, fs...)
	return p
}

func (p *watcher) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	for _, f := range p.reconcileFuncs {
		if res, err := f(ctx, req); !res.IsZero() || err != nil {
			return res, errors.Wrap(err, "failed to reconcile")
		}
	}
	return ctrl.Result{}, nil
}

type limiter interface {
	Allow() bool
}

// NotifierFunc returns a reconcile.Func that lists Pods to get the latest
// state and notifies listeners of the resulting Pods.
// listOpts are passed to the client.List call to filter the Pod list.
// limiter is an optional rate limiter which may be used to limit the
// rate at which listeners are notified of list changes. This guarantees
// that all Pod events will eventually be processed, but allows the listeners
// to react to less (but more complete) changes. If we rate limit events, we
// end up sending a version of the Pod list that is newer, without missing
// any events.
// listeners are called with the new Pod list.
func (p *watcher) NewNotifierFunc(listOpts *client.ListOptions, limiter limiter, listeners ...func([]v1.Pod)) reconcile.Func {
	p.z.Info("adding notifier for listeners", zap.Int("listeners", len(listeners)))
	return func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
		if !limiter.Allow() {
			// rate limit exceeded, requeue
			p.z.Info("rate limit exceeded")
			return ctrl.Result{Requeue: true}, nil
		}
		podList := &v1.PodList{}
		if err := p.cli.List(ctx, podList, listOpts); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to list pods")
		}
		pods := podList.Items
		for _, l := range listeners {
			l(pods)
		}
		return ctrl.Result{}, nil
	}
}

var hostNetworkIndexer = client.IndexerFunc(func(o client.Object) []string {
	pod, ok := o.(*v1.Pod)
	if !ok {
		return nil
	}
	return []string{strconv.FormatBool(pod.Spec.HostNetwork)}
})

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (p *watcher) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	p.cli = mgr.GetClient()
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v1.Pod{}, "spec.hostNetwork", hostNetworkIndexer); err != nil {
		return errors.Wrap(err, "failed to set up hostNetwork indexer")
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		WithEventFilter(predicate.Funcs{ // we only want create/delete events
			UpdateFunc: func(event.UpdateEvent) bool {
				return false
			},
			GenericFunc: func(event.GenericEvent) bool {
				return false
			},
		}).
		Complete(p); err != nil {
		return errors.Wrap(err, "failed to set up pod watcher with manager")
	}
	return nil
}
