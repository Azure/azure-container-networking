package kubernetes

import (
	"context"
	"errors"
	"os"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const k8sNamespace = "kube-system"

// k8sRequestController watches CRDs for status updates and publishes CRD spec changes
// mgr acts as the communication between requestController and API server
// implements the RequestController interface
type k8sRequestController struct {
	mgr         manager.Manager             //mgr has method GetClient() to get k8s client
	Client      K8sClient                   //Relying on the K8sClient interface to more easily test
	restService *restserver.HTTPRestService //restService is given to nodeNetworkConfigReconciler (the reconcile loop)
	hostName    string                      //name of node running this program
}

//NewK8sRequestController given a reference to CNS's HTTPRestService state, returns a k8sRequestController struct
func NewK8sRequestController(restService *restserver.HTTPRestService) (*k8sRequestController, error) {

	//Check that logger package has been intialized
	if logger.Log == nil {
		return nil, errors.New("Must initialize logger before calling")
	}

	// Check that HOSTNAME environment variable is set. HOSTNAME is name of node running this program
	hostName := os.Getenv("HOSTNAME")
	if hostName == "" {
		return nil, errors.New("Must declare HOSTNAME environment variable. HOSTNAME is name of node.")
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
	k8sRequestController := k8sRequestController{
		mgr:         mgr,
		Client:      mgr.GetClient(),
		restService: restService,
		hostName:    hostName,
	}

	return &k8sRequestController, nil
}

// StartRequestController starts the reconcile loop. This loop waits for changes to CRD statuses.
// When a CRD status change is made, Reconcile from nodenetworkconfigreconciler is called.
func (k8sRC *k8sRequestController) StartRequestController() error {
	nodenetworkconfigreconciler := &NodeNetworkConfigReconciler{
		K8sClient:   k8sRC.Client,
		RestService: k8sRC.restService,
		HostName:    k8sRC.hostName,
	}

	// Setup manager with NodeNetworkConfigReconciler
	if err := nodenetworkconfigreconciler.SetupWithManager(k8sRC.mgr); err != nil {
		logger.Errorf("[cns-rc] Error creating new NodeNetworkConfigReconciler: %v", err)
		return err
	}

	// Start manager and consequently, the reconciler
	// Start() blocks until SIGINT or SIGTERM is received
	go func() {
		logger.Printf("Starting manager")
		if err := k8sRC.mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			logger.Errorf("[cns-rc] Error starting manager: %v", err)
		}
	}()

	return nil
}

// ReleaseIPsByUUIDs sends release ip request to the API server. Provide the UUIDs of the IP allocations
func (k8sRC *k8sRequestController) ReleaseIPsByUUIDs(listOfIPUUIDS []string) error {
	nodeNetworkConfig, err := k8sRC.getNodeNetConfig(k8sRC.hostName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when releasing IPs by uuid %V", err)
		return err
	}

	//Update the CRD IpsNotInUse
	nodeNetworkConfig.Spec.IPsNotInUse = append(nodeNetworkConfig.Spec.IPsNotInUse, listOfIPUUIDS...)

	//Send update to API server
	if err := k8sRC.updateNodeNetConfig(nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD when releasing IPs by uuid %v", err)
		return err
	}

	return nil
}

// UpdateIPCount sends the new requested ip count to the API server.
func (k8sRC *k8sRequestController) UpdateRequestedIPCount(newCount int64) error {
	nodeNetworkConfig, err := k8sRC.getNodeNetConfig(k8sRC.hostName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when releasing IPs by uuid %V", err)
		return err
	}

	//Update the CRD IP count
	nodeNetworkConfig.Spec.RequestedIPCount = newCount

	//Send update to API server
	if err := k8sRC.updateNodeNetConfig(nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD when releasing IPs by uuid %v", err)
		return err
	}

	return nil
}

// getNodeNetConfig gets the nodeNetworkConfig CRD given the name and namespace of the CRD object
func (k8sRC *k8sRequestController) getNodeNetConfig(name, namespace string) (*nnc.NodeNetworkConfig, error) {
	nodeNetworkConfig := &nnc.NodeNetworkConfig{}

	err := k8sRC.Client.Get(context.Background(), client.ObjectKey{
		Namespace: k8sNamespace,
		Name:      k8sRC.hostName,
	}, nodeNetworkConfig)

	if err != nil {
		return nil, err
	}

	return nodeNetworkConfig, nil
}

// updateNodeNetConfig updates the nodeNetConfig object in the API server with the given nodeNetworkConfig object
func (k8sRC *k8sRequestController) updateNodeNetConfig(nodeNetworkConfig *nnc.NodeNetworkConfig) error {
	if err := k8sRC.Client.Update(context.Background(), nodeNetworkConfig); err != nil {
		return err
	}

	return nil
}
