package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type cnsclient interface {
	ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) error
	CreateOrUpdateNC(ncRequest cns.CreateNetworkContainerRequest) error
	UpdateIPAMPoolMonitor(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec)
}

type nncgetter interface {
	Get(ctx context.Context, key types.NamespacedName) (*v1alpha.NodeNetworkConfig, error)
}

// Reconciler watches for CRD status changes
type Reconciler struct {
	cnscli cnsclient
	nnccli nncgetter
}

func New(nnccli nncgetter, cnscli cnsclient) *Reconciler {
	return &Reconciler{
		cnscli: cnscli,
		nnccli: nnccli,
	}
}

// Reconcile is called on CRD status changes
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	nnc, err := r.nnccli.Get(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, errors.Wrapf(client.IgnoreNotFound(err), "NodeNetworkConfig %v not found", req.NamespacedName)
		}
		logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
		return reconcile.Result{}, errors.Wrapf(err, "failed to get NodeNetworkConfig %v", req.NamespacedName)
	}

	logger.Printf("[cns-rc] CRD Spec: %v", nnc.Spec)

	// If there are no network containers, don't hand it off to CNS
	if len(nnc.Status.NetworkContainers) == 0 {
		logger.Errorf("[cns-rc] Empty NetworkContainers")
		return reconcile.Result{}, nil
	}

	networkContainer := nnc.Status.NetworkContainers[0]
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
	ncRequest, err := CRDStatusToNCRequest(&nnc.Status)
	if err != nil {
		logger.Errorf("[cns-rc] Error translating crd status to nc request %v", err)
		// requeue
		return reconcile.Result{}, errors.Wrap(err, "failed to convert NNC status to network container request")
	}

	responseCode := r.cnscli.CreateOrUpdateNetworkContainerInternal(&ncRequest)
	err = restserver.ResponseCodeToError(responseCode)
	if err != nil {
		logger.Errorf("[cns-rc] Error creating or updating NC in reconcile: %v", err)
		// requeue
		return reconcile.Result{}, errors.Wrap(err, "failed to create or update network container")
	}

	r.cnscli.UpdateIPAMPoolMonitor(nnc.Status.Scaler, nnc.Spec)
	// record assigned IPs metric
	assignedIPs.Set(float64(len(nnc.Status.NetworkContainers[0].IPAssignments)))

	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, nodeName string) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha.NodeNetworkConfig{}).
		WithEventFilter(predicate.Funcs{
			// ignore delete events.
			DeleteFunc: func(event.DeleteEvent) bool {
				return false
			},
		}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// match on node name for all other events.
			return nodeName == object.GetName()
		})).
		WithEventFilter(predicate.Funcs{
			// check that the generation is the same - status changes don't update generation.
			UpdateFunc: func(ue event.UpdateEvent) bool {
				return ue.ObjectOld.GetGeneration() == ue.ObjectNew.GetGeneration()
			},
		}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed to set up reconciler with manager")
	}
	return nil
}
