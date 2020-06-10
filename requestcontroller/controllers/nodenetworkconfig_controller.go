package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// NodeNetworkConfigReconciler (aka controller) watches API server for any creation/deletion/updates of NodeNetworkConfig objects
type NodeNetworkConfigReconciler struct {
	K8sClient client.Client
}

// Reconcile relays changes in NodeNetworkConfig to CNS
// Returning non-nil error causes a requeue
// Returning ctrl.Result{}, nil causes the queue to "forget" the item
// Other return values are possible, see kubebuilder docs for details
func (n *NodeNetworkConfigReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := n.K8sClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	//TODO: Pass the updates to CNS

	return ctrl.Result{}, nil
}

// SetupWithManager Sets up the controller with a new manager
func (n *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		Complete(n)
}
