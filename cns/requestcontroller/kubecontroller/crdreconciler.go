package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CrdReconciler watches for CRD status changes
type CrdReconciler struct {
	KubeClient KubeClient
	NodeName   string
	CNSClient  cnsclient.APIClient
}

// Reconcile is called on CRD status changes
func (r *CrdReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var nodeNetConfig nnc.NodeNetworkConfig

	//Get the CRD object
	if err := r.KubeClient.Get(context.TODO(), request.NamespacedName, &nodeNetConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		} else {
			logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
			return reconcile.Result{}, err
		}
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
