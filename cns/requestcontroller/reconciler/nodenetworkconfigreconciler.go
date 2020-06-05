package nodenetworkconfigreconciler

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// NodeNetworkConfigReconciler reconciles a NodeNetworkConfig object
type NodeNetworkConfigReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Reconcile relays changes in NodeNetworkConfig to CNS
func (r *NodeNetworkConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("nodenetworkconfig", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager Sets up the reconcilers
func (r *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		Complete(r)
}
