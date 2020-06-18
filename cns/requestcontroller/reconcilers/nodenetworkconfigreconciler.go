package reconcilers

import (
	"context"
	"errors"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/channels"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NodeNetworkConfigReconciler (aka controller) watches API server for any creation/deletion/updates of NodeNetworkConfig objects
type NodeNetworkConfigReconciler struct {
	K8sClient  client.Client
	CNSchannel chan channels.CNSChannel
	HostName   string
}

// Reconcile relays changes in NodeNetworkConfig to CNS
// Returning non-nil error causes a requeue
// Returning ctrl.Result{}, nil causes the queue to "forget" the item
// Other return values are possible, see kubebuilder docs for details
func (n *NodeNetworkConfigReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// We are only interested in requests coming for the node that this program is running on
	// Requeue if it's not for this node
	if request.Name != n.HostName {
		return reconcile.Result{}, errors.New("Requeing")
	}

	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := n.K8sClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	//TODO: Pass the updates to CNS via the CNSChannel

	logger.Printf("Sending channel")
	tempChannel := channels.CNSChannel{
		Foo: "hi",
	}

	n.CNSchannel <- tempChannel

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager
func (n *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		Complete(n)
}
