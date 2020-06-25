package kubernetes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NodeNetworkConfigReconciler (aka controller) watches API server for any creation/deletion/updates of NodeNetworkConfig objects
type NodeNetworkConfigReconciler struct {
	K8sClient   K8sClient
	RestService *restserver.HTTPRestService
	HostName    string
}

// Reconcile relays status changes in NodeNetworkConfig to CNS
// Returning non-nil error causes a requeue
// Returning ctrl.Result{}, nil causes the queue to "forget" the item
// Other return values are possible, see kubebuilder docs for details
func (n *NodeNetworkConfigReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := n.K8sClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	//TODO: Translate CRD status into HTTPRestService state

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager
func (n *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		WithEventFilter(NodeNetworkConfigFilter{hostname: n.HostName}).
		Complete(n)
}
