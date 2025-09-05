package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// PodReconciler is a dummy reconciler just to allow us to register the watcher.
type PodReconciler struct {
	client.Client
}

func (p *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// We don’t do reconciliation here — just return.
	pod := &corev1.Pod{}
	if err := p.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fmt.Println("got pod:", pod.Name, "in namespace:", pod.Namespace)

	return ctrl.Result{}, nil
}

func (p *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	p.Client = mgr.GetClient()
	err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithOptions(controller.Options{}).
		Named("pod_Reconciler").
		Complete(p)
	if err != nil {
		return errors.Wrap(err, "error setting up pod reconciler with manager")
	}

	return nil
}

func main() {
	// Set up logging
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up manager (auto-loads ~/.kube/config or in-cluster config)
	ctrlRuntimeMgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: nil,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to start manager: %v\n", err)
		os.Exit(1)
	}

	podReconciler := &PodReconciler{}
	if err := podReconciler.SetupWithManager(ctrlRuntimeMgr); err != nil {
		fmt.Fprintf(os.Stderr, "problem running manager: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🔍 Starting Pod watcher with controller-runtime...")
	if err := ctrlRuntimeMgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "problem running manager: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Pod watcher started successfully.")
}
