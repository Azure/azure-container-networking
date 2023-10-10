package nodenetworkconfig

import (
	"context"
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type cnsClient interface {
	CreateOrUpdateNetworkContainerInternal(*cns.CreateNetworkContainerRequest) cnstypes.ResponseCode
	MustEnsureNoStaleNCs(validNCIDs []string)
}

type nodeNetworkConfigListener interface {
	Update(*v1alpha.NodeNetworkConfig) error
}

type nncGetter interface {
	Get(context.Context, types.NamespacedName) (*v1alpha.NodeNetworkConfig, error)
}

// Reconciler watches for CRD status changes
type Reconciler struct {
	cnscli             cnsClient
	ipampoolmonitorcli nodeNetworkConfigListener
	nnccli             nncGetter
	once               sync.Once
	started            chan interface{}
	nodeIP             string
}

// NewReconciler creates a NodeNetworkConfig Reconciler which will get updates from the Kubernetes
// apiserver for NNC events.
// Provided nncListeners are passed the NNC after the Reconcile preprocesses it. Note: order matters! The
// passed Listeners are notified in the order provided.
func NewReconciler(cnscli cnsClient, ipampoolmonitorcli nodeNetworkConfigListener, nodeIP string) *Reconciler {
	return &Reconciler{
		cnscli:             cnscli,
		ipampoolmonitorcli: ipampoolmonitorcli,
		started:            make(chan interface{}),
		nodeIP:             nodeIP,
	}
}

// Reconcile is called on CRD status changes
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	listenersToNotify := []nodeNetworkConfigListener{}
	nnc, err := r.nnccli.Get(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Printf("[cns-rc] CRD not found, ignoring %v", err)
			return reconcile.Result{}, errors.Wrapf(client.IgnoreNotFound(err), "NodeNetworkConfig %v not found", req.NamespacedName)
		}
		logger.Errorf("[cns-rc] Error retrieving CRD from cache : %v", err)
		return reconcile.Result{}, errors.Wrapf(err, "failed to get NodeNetworkConfig %v", req.NamespacedName)
	}

	logger.Printf("[cns-rc] CRD Spec: %+v", nnc.Spec)

	ipAssignments := 0

	// during node upgrades, an nnc may be updated with new ncs. at any given time, only the ncs
	// that exist in the nnc are valid. any others that may have been previously created and no
	// longer exist in the nnc should be considered stale.
	validNCIDs := make([]string, len(nnc.Status.NetworkContainers))
	for i := range nnc.Status.NetworkContainers {
		validNCIDs[i] = nnc.Status.NetworkContainers[i].ID
	}
	r.cnscli.MustEnsureNoStaleNCs(validNCIDs)

	// for each NC, parse it in to a CreateNCRequest and forward it to the appropriate Listener
	for i := range nnc.Status.NetworkContainers {
		// check if this NC matches the Node IP if we have one to check against
		if r.nodeIP != "" {
			if r.nodeIP != nnc.Status.NetworkContainers[i].NodeIP {
				// skip this NC since it was created for a different node
				logger.Printf("[cns-rc] skipping network container %s found in NNC because node IP doesn't match, got %s, expected %s",
					nnc.Status.NetworkContainers[i].ID, nnc.Status.NetworkContainers[i].NodeIP, r.nodeIP)
				continue
			}
		}

		var req *cns.CreateNetworkContainerRequest
		var err error
		switch nnc.Status.NetworkContainers[i].AssignmentMode { //nolint:exhaustive // skipping dynamic case
		// For Overlay and Vnet Scale Scenarios
		case v1alpha.Static:
			req, err = CreateNCRequestFromStaticNC(nnc.Status.NetworkContainers[i])
		// For Pod Subnet scenario
		default: // For backward compatibility, default will be treated as Dynamic too.
			req, err = CreateNCRequestFromDynamicNC(nnc.Status.NetworkContainers[i])
			// in dynamic, we will also push this NNC to the IPAM Pool Monitor when we're done.
			listenersToNotify = append(listenersToNotify, r.ipampoolmonitorcli)
		}

		if err != nil {
			logger.Errorf("[cns-rc] failed to generate CreateNCRequest from NC: %v, assignmentMode %s", err,
				nnc.Status.NetworkContainers[i].AssignmentMode)
			return reconcile.Result{}, errors.Wrapf(err, "failed to generate CreateNCRequest from NC "+
				"assignmentMode %s", nnc.Status.NetworkContainers[i].AssignmentMode)
		}

		responseCode := r.cnscli.CreateOrUpdateNetworkContainerInternal(req)
		if err := restserver.ResponseCodeToError(responseCode); err != nil {
			logger.Errorf("[cns-rc] Error creating or updating NC in reconcile: %v", err)
			return reconcile.Result{}, errors.Wrap(err, "failed to create or update network container")
		}
		ipAssignments += len(req.SecondaryIPConfigs)
	}

	// record assigned IPs metric
	allocatedIPs.Set(float64(ipAssignments))

	// push the NNC to the registered NNC listeners.
	for _, l := range listenersToNotify {
		if err := l.Update(nnc); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "nnc listener return error during update")
		}
	}

	// we have received and pushed an NNC update, we are "Started"
	r.once.Do(func() {
		close(r.started)
		logger.Printf("[cns-rc] CNS NNC Reconciler Started")
	})
	return reconcile.Result{}, nil
}

// Started blocks until the Reconciler has reconciled at least once,
// then, and any time that it is called after that, it immediately returns true.
// It accepts a cancellable Context and if the context is closed
// before Start it will return false. Passing a closed Context after the
// Reconciler is started is indeterminate.
func (r *Reconciler) Started(ctx context.Context) (bool, error) {
	select {
	case <-r.started:
		return true, nil
	case <-ctx.Done():
		return false, errors.Wrap(ctx.Err(), "context closed")
	}
}

// SetupWithManager Sets up the reconciler with a new manager, filtering using NodeNetworkConfigFilter on nodeName.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, node *v1.Node) error {
	r.nnccli = nodenetworkconfig.NewClient(mgr.GetClient())
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha.NodeNetworkConfig{}).
		WithEventFilter(predicate.Funcs{
			// ignore delete events.
			DeleteFunc: func(event.DeleteEvent) bool {
				return false
			},
		}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			// match on node controller ref for all other events.
			return metav1.IsControlledBy(object, node)
		})).
		WithEventFilter(predicate.Funcs{
			// check that the generation is the same - status changes don't update generation.
			UpdateFunc: func(ue event.UpdateEvent) bool {
				if ue.ObjectOld == nil || ue.ObjectNew == nil {
					return false
				}
				return ue.ObjectOld.GetGeneration() == ue.ObjectNew.GetGeneration()
			},
		}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed to set up reconciler with manager")
	}
	return nil
}
