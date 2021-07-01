package multitenantoperator

import (
	"context"
	"encoding/json"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	ncapi "github.com/Azure/azure-container-networking/crds/multitenantnetworkcontainer/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	NCStateInitialized = "Initialized" // NC has been initialized by DNC.
	NCStateSucceeded   = "Succeeded"   // NC has been persisted by CNS
	NCStateTerminated  = "Terminated"  // NC has been cleaned up from CNS
)

// multiTenantCrdReconciler reconciles multi-tenant network containers.
type multiTenantCrdReconciler struct {
	KubeClient client.Client
	NodeName   string
	CNSClient  cnsclient.APIClient
}

// Reconcile is called on multi-tenant CRD status changes.
func (r *multiTenantCrdReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	var nc ncapi.MultiTenantNetworkContainer

	if err := r.KubeClient.Get(ctx, request.NamespacedName, &nc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Errorf("Failed to fetch network container: %v", err)
		return ctrl.Result{}, err
	}

	if !nc.ObjectMeta.DeletionTimestamp.IsZero() {
		// Do nothing if the NC has already in Terminated state.
		if nc.Status.State == NCStateTerminated {
			return ctrl.Result{}, nil
		}

		// Remove the deleted network container from CNS.
		err := r.CNSClient.DeleteNC(cns.DeleteNetworkContainerRequest{
			NetworkContainerid: nc.Spec.UUID,
		})
		if err != nil {
			logger.Errorf("Failed to delete NC %s from CNS: %v", nc.Spec.UUID, err)
			return ctrl.Result{}, err
		}

		// Update NC state to Terminated.
		nc.Status.State = NCStateTerminated
		if err := r.KubeClient.Status().Update(ctx, &nc); err != nil {
			logger.Errorf("Failed to update network container state for %s: %v", nc.Spec.UUID, err)
			return ctrl.Result{}, err
		}

		logger.Printf("NC has been terminated for %s", nc.Spec.UUID)
		return ctrl.Result{}, nil
	}

	// Do nothing if the network container hasn't been initialized yet from control plane.
	if nc.Status.State != NCStateInitialized {
		return ctrl.Result{}, nil
	}

	// Check CNS NC states.
	_, err := r.CNSClient.GetNC(cns.GetNetworkContainerRequest{
		NetworkContainerid: nc.Spec.UUID,
	})
	if err == nil {
		logger.Printf("NC %s has already been created in CNS", nc.Spec.UUID)
		return ctrl.Result{}, nil
	} else if err.Error() != "NotFound" {
		logger.Errorf("Failed to fetch NC from CNS: %v", err)
		return ctrl.Result{}, err
	}

	// Parse KubernetesPodInfo as orchestratorContext.
	podInfo := cns.KubernetesPodInfo{
		PodName:      nc.Name,
		PodNamespace: nc.Namespace,
	}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		logger.Errorf("Failed to marshal podInfo (%v): %v", podInfo, err)
		return ctrl.Result{}, err
	}

	// Persist NC states into CNS.
	_, ipNet, err := net.ParseCIDR(nc.Status.IPSubnet)
	if err != nil {
		logger.Errorf("Failed to parse IPSubnet %s for NC %s: %v", nc.Status.IPSubnet, nc.Spec.UUID, err)
		return ctrl.Result{}, err
	}
	prefixLength, _ := ipNet.Mask.Size()
	networkContainerRequest := cns.CreateNetworkContainerRequest{
		NetworkContainerid:  nc.Spec.UUID,
		OrchestratorContext: orchestratorContext,
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    nc.Status.IP,
				PrefixLength: uint8(prefixLength),
			},
			GatewayIPAddress: nc.Status.Gateway,
		},
	}
	if err = r.CNSClient.CreateOrUpdateNC(networkContainerRequest); err != nil {
		logger.Errorf("Failed to persist state for NC %s to CNS: %v", nc.Spec.UUID, err)
		return ctrl.Result{}, err
	}

	// Update NC state to Succeeded.
	nc.Status.State = NCStateSucceeded
	if err := r.KubeClient.Status().Update(ctx, &nc); err != nil {
		logger.Errorf("Failed to update network container state for %s: %v", nc.Spec.UUID, err)
		return ctrl.Result{}, err
	}

	logger.Printf("Reconciled NC %s", nc.Spec.UUID)
	return reconcile.Result{}, nil
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter
func (r *multiTenantCrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ncapi.MultiTenantNetworkContainer{}).
		WithEventFilter(r.predicate()).
		Complete(r)
}

func (r *multiTenantCrdReconciler) predicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.equalNode(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return r.equalNode(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return r.equalNode(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.equalNode(e.Object)
		},
	}
}

func (r *multiTenantCrdReconciler) equalNode(o runtime.Object) bool {
	nc, ok := o.(*ncapi.MultiTenantNetworkContainer)
	if ok {
		return strings.EqualFold(nc.Spec.Node, r.NodeName)
	}

	return false
}
