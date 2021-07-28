package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
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
	KubeClient      KubeClient
	NodeName        string
	CNSClient       cnsclient.APIClient
	IPAMPoolMonitor cns.IPAMPoolMonitor
}

// Reconcile is called on CRD status changes
func (r *CrdReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	var (
		nodeNetConfig nnc.NodeNetworkConfig
		ncRequest     cns.CreateNetworkContainerRequest
		err           error
	)

	//Get the CRD object
	if err = r.KubeClient.Get(ctx, request.NamespacedName, &nodeNetConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		} else {
			logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
			return reconcile.Result{}, err
		}
	}

	logger.Printf("[cns-rc] CRD Spec: %v", nodeNetConfig.Spec)

	// If there are no network containers, don't hand it off to CNS
	if len(nodeNetConfig.Status.NetworkContainers) == 0 {
		logger.Errorf("[cns-rc] Empty NetworkContainers")
		return reconcile.Result{}, nil
	}

	networkContainer := nodeNetConfig.Status.NetworkContainers[0]
	logger.Printf("[cns-rc] CRD Status: NcId: [%s], Version: [%d],  podSubnet: [%s], Subnet CIDR: [%s], "+
		"Gateway Addr: [%s], Primary IP: [%s], SecondaryIpsCount: [%d]",
		networkContainer.ID,
		networkContainer.Version,
		networkContainer.SubnetName,
		networkContainer.SubnetAddressSpace,
		networkContainer.DefaultGateway,
		networkContainer.PrimaryIP,
		len(networkContainer.IPAssignments))

	// Otherwise, create NC request and hand it off to CNS
	ncRequest, err = CRDStatusToNCRequest(nodeNetConfig.Status)
	if err != nil {
		logger.Errorf("[cns-rc] Error translating crd status to nc request %v", err)
		//requeue
		return reconcile.Result{}, err
	}

	if err = r.CNSClient.CreateOrUpdateNC(ncRequest); err != nil {
		logger.Errorf("[cns-rc] Error creating or updating NC in reconcile: %v", err)
		// requeue
		return reconcile.Result{}, err
	}

	if err = r.CNSClient.UpdateIPAMPoolMonitor(nodeNetConfig.Status.Scaler, nodeNetConfig.Spec); err != nil {
		logger.Errorf("[cns-rc] Error update IPAM pool monitor in reconcile: %v", err)
		// requeue
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, err
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter
func (r *CrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnc.NodeNetworkConfig{}).
		WithEventFilter(NodeNetworkConfigFilter{nodeName: r.NodeName}).
		Complete(r)
}
