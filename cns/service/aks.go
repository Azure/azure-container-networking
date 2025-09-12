// AKS specific initialization flows
// nolint // it's not worth it
package main

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	nncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type cniConflistScenario string

const (
	scenarioV4Overlay        cniConflistScenario = "v4overlay"
	scenarioDualStackOverlay cniConflistScenario = "dualStackOverlay"
	scenarioOverlay          cniConflistScenario = "overlay"
	scenarioCilium           cniConflistScenario = "cilium"
	scenarioSWIFT            cniConflistScenario = "swift"
)

type nodeNetworkConfigGetter interface {
	Get(context.Context) (*v1alpha.NodeNetworkConfig, error)
}

type ipamStateReconciler interface {
	ReconcileIPAMStateForSwift(ncRequests []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) cnstypes.ResponseCode
}

// TODO(rbtr) where should this live??
// reconcileInitialCNSState initializes cns by passing pods and a CreateNetworkContainerRequest
func reconcileInitialCNSState(ctx context.Context, cli nodeNetworkConfigGetter, ipamReconciler ipamStateReconciler, podInfoByIPProvider cns.PodInfoByIPProvider) error {
	// Get nnc using direct client
	nnc, err := cli.Get(ctx)
	if err != nil {
		if crd.IsNotDefined(err) {
			return errors.Wrap(err, "failed to init CNS state: NNC CRD is not defined")
		}
		if apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to init CNS state: NNC not found")
		}
		return errors.Wrap(err, "failed to init CNS state: failed to get NNC CRD")
	}

	logger.Printf("Retrieved NNC: %+v", nnc)
	if !nnc.DeletionTimestamp.IsZero() {
		return errors.New("failed to init CNS state: NNC is being deleted")
	}

	// If there are no NCs, we can't initialize our state and we should fail out.
	if len(nnc.Status.NetworkContainers) == 0 {
		return errors.New("failed to init CNS state: no NCs found in NNC CRD")
	}

	// Get previous PodInfo state from podInfoByIPProvider
	podInfoByIP, err := podInfoByIPProvider.PodInfoByIP()
	if err != nil {
		return errors.Wrap(err, "provider failed to provide PodInfoByIP")
	}

	ncReqs := make([]*cns.CreateNetworkContainerRequest, len(nnc.Status.NetworkContainers))

	// For each NC, we need to create a CreateNetworkContainerRequest and use it to rebuild our state.
	for i := range nnc.Status.NetworkContainers {
		var (
			ncRequest *cns.CreateNetworkContainerRequest
			err       error
		)
		switch nnc.Status.NetworkContainers[i].AssignmentMode { //nolint:exhaustive // skipping dynamic case
		case v1alpha.Static:
			ncRequest, err = nncctrl.CreateNCRequestFromStaticNC(nnc.Status.NetworkContainers[i])
		default: // For backward compatibility, default will be treated as Dynamic too.
			ncRequest, err = nncctrl.CreateNCRequestFromDynamicNC(nnc.Status.NetworkContainers[i])
		}

		if err != nil {
			return errors.Wrapf(err, "failed to convert NNC status to network container request, "+
				"assignmentMode: %s", nnc.Status.NetworkContainers[i].AssignmentMode)
		}

		ncReqs[i] = ncRequest
	}

	// Call cnsclient init cns passing those two things.
	if err := restserver.ResponseCodeToError(ipamReconciler.ReconcileIPAMStateForSwift(ncReqs, podInfoByIP, nnc)); err != nil {
		return errors.Wrap(err, "failed to reconcile CNS IPAM state")
	}

	return nil
}
