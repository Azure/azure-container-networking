package requestcontroller

import (
	"errors"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/channels"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/reconcilers"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//requestController watches CRDs for status updates and publishes CRD spec changes
// cnsChannel acts as the communication between CNS and requestController
// mgr acts as the communication between requestController and API server
type requestController struct {
	cnsChannel chan channels.CNSChannel
	mgr        manager.Manager //mgr has method GetClient() to get k8s client
}

//NewRequestController given a CNSChannel, returns a requestController struct
func NewRequestController(cnsChannel chan channels.CNSChannel) (*requestController, error) {
	const k8sNamespace = "kube-system"

	//Check that logger package has been intialized
	if logger.Log == nil {
		return nil, errors.New("Must initialize logger before calling")
	}

	//Add CRD scheme to runtime sheme so manager can recognize it
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nnc.AddToScheme(scheme)

	// GetConfig precedence
	// * --kubeconfig flag pointing at a file at this cmd line
	// * KUBECONFIG environment variable pointing at a file
	// * In-cluster config if running in cluster
	// * $HOME/.kube/config if exists
	// We're not using GetConfigOrDie becuase we want to propogate the error if there is one
	k8sconfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Errorf("[cns-rc] Error getting kubeconfig: %v", err)
		return nil, err
	}

	// Create manager for NodeNetworkConfigReconciler
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(k8sconfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		Namespace:          k8sNamespace,
	})
	if err != nil {
		logger.Errorf("[cns-rc] Error creating new request controller manager: %v", err)
		return nil, err
	}

	// Create the requestController struct
	requestController := requestController{
		cnsChannel: cnsChannel,
		mgr:        mgr,
	}

	return &requestController, nil
}

// StartRequestController starts the reconcile loop. This loop waits for changes to CRD statuses.
// When a CRD status change is made, Reconcile from nodenetworkconfigreconciler is called.
func (rc *requestController) StartRequestController() error {
	nodenetworkconfigreconciler := &reconcilers.NodeNetworkConfigReconciler{
		K8sClient:  rc.mgr.GetClient(),
		CNSchannel: rc.cnsChannel,
	}

	// Setup manager with NodeNetworkConfigReconciler
	if err := nodenetworkconfigreconciler.SetupWithManager(rc.mgr); err != nil {
		logger.Errorf("[cns-rc] Error creating new NodeNetworkConfigReconciler: %v", err)
		return err
	}

	// Start manager and consequently, the reconciler
	// Start() blocks until SIGINT or SIGTERM is received
	go func() {
		logger.Printf("Starting manager")
		if err := rc.mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			logger.Errorf("[cns-rc] Error starting manager: %v", err)
		}
	}()

	return nil
}
