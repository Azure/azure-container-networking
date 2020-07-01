package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CrdReconciler watches for CRD status changes
type CrdReconciler struct {
	APIClient APIClient
	NodeName  string
	CNSClient requestcontroller.CNSClient
}

// Reconcile is called on CRD status changes
func (r *CrdReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := r.APIClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	logger.Printf("[cns-rc] CRD object: %v", nodeNetConfig)

	//TODO: Translate CRD status into NetworkContainer request
	ncRequest, err := CRDStatusToNCRequest(&nodeNetConfig.Status)
	if err != nil {
		logger.Errorf("[cns-rc] Error translating crd status to nc request %v", err)
		//requeue
		return reconcile.Result{}, err
	}

	//TODO: process the nc request on CNS side
	r.CNSClient.UpdateCNSState(ncRequest)

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter
func (r *CrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		WithEventFilter(NodeNetworkConfigFilter{nodeName: r.NodeName}).
		Complete(r)
}
