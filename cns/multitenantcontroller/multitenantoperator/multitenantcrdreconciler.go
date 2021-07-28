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
	// NCStateInitialized indicates the NC has been initialized by DNC.
	NCStateInitialized = "Initialized"
	// NCStateSucceeded indicates the NC has been persisted by CNS.
	NCStateSucceeded = "Succeeded"
	// NCStateTerminated indicates the NC has been terminated by CNS.
	NCStateTerminated = "Terminated"
)

// multiTenantCrdReconciler reconciles multi-tenant network containers.
type multiTenantCrdReconciler struct {
	KubeClient client.Client
	NodeName   string
	CNSClient  cnsclient.APIClient
}

// Reconcile is called on multi-tenant CRD status changes.
func (r *multiTenantCrdReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger.Printf("Reconcling MultiTenantNetworkContainer %v", request.NamespacedName.String())

	var nc ncapi.MultiTenantNetworkContainer
	if err := r.KubeClient.Get(ctx, request.NamespacedName, &nc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("MultiTenantNetworkContainer %s not found, skip reconciling", request.NamespacedName.String())
			return ctrl.Result{}, nil
		}

		logger.Errorf("Failed to fetch network container %s: %v", request.NamespacedName.String(), err)
		return ctrl.Result{}, err
	}

	if !nc.ObjectMeta.DeletionTimestamp.IsZero() {
		// Do nothing if the NC has already in Terminated state.
		if nc.Status.State == NCStateTerminated {
			logger.Printf("MultiTenantNetworkContainer %s already terminated, skip reconciling", request.NamespacedName.String())
			return ctrl.Result{}, nil
		}

		// Remove the deleted network container from CNS.
		err := r.CNSClient.DeleteNC(cns.DeleteNetworkContainerRequest{
			NetworkContainerid: nc.Spec.UUID,
		})
		if err != nil {
			logger.Errorf("Failed to delete NC %s (UUID: %s) from CNS: %v", request.NamespacedName.String(), nc.Spec.UUID, err)
			return ctrl.Result{}, err
		}

		// Update NC state to Terminated.
		nc.Status.State = NCStateTerminated
		if err := r.KubeClient.Status().Update(ctx, &nc); err != nil {
			logger.Errorf("Failed to update network container state for %s (UUID: %s): %v", request.NamespacedName.String(), nc.Spec.UUID, err)
			return ctrl.Result{}, err
		}

		logger.Printf("NC has been terminated for %s (UUID: %s)", request.NamespacedName.String(), nc.Spec.UUID)
		return ctrl.Result{}, nil
	}

	// Do nothing if the network container hasn't been initialized yet from control plane.
	if nc.Status.State != NCStateInitialized {
		logger.Printf("MultiTenantNetworkContainer %s hasn't initialized yet, skip reconciling", request.NamespacedName.String())
		return ctrl.Result{}, nil
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

	// Check CNS NC states.
	_, err = r.CNSClient.GetNC(cns.GetNetworkContainerRequest{
		NetworkContainerid:  nc.Spec.UUID,
		OrchestratorContext: orchestratorContext,
	})
	if err == nil {
		logger.Printf("NC %s (UUID: %s) has already been created in CNS", request.NamespacedName.String(), nc.Spec.UUID)
		return ctrl.Result{}, nil
	} else if err.Error() != "NotFound" {
		logger.Errorf("Failed to fetch NC %s (UUID: %s) from CNS: %v", request.NamespacedName.String(), nc.Spec.UUID, err)
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
		NetworkContainerid:   nc.Spec.UUID,
		OrchestratorContext:  orchestratorContext,
		NetworkContainerType: cns.Kubernetes,
		Version:              "0",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    nc.Status.IP,
				PrefixLength: uint8(prefixLength),
			},
			GatewayIPAddress: nc.Status.Gateway,
		},
		PrimaryInterfaceIdentifier: nc.Status.PrimaryInterfaceIdentifier,
	}
	logger.Printf("CreateOrUpdateNC with networkContainerRequest: %#v", networkContainerRequest)
	if err = r.CNSClient.CreateOrUpdateNC(networkContainerRequest); err != nil {
		logger.Errorf("Failed to persist state for NC %s (UUID: %s) to CNS: %v", request.NamespacedName.String(), nc.Spec.UUID, err)
		return ctrl.Result{}, err
	}

	// Update NC state to Succeeded.
	nc.Status.State = NCStateSucceeded
	if err := r.KubeClient.Status().Update(ctx, &nc); err != nil {
		logger.Errorf("Failed to update network container state for %s (UUID: %s): %v", request.NamespacedName.String(), nc.Spec.UUID, err)
		return ctrl.Result{}, err
	}

	logger.Printf("Reconciled NC %s (UUID: %s)", request.NamespacedName.String(), nc.Spec.UUID)
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
