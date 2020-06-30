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
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//make environment var name constant
const k8sNamespace = "kube-system"

//disable prometheus constant
//nodeNetConfigRequestController

// k8sRequestController watches CRDs for status updates and publishes CRD spec changes
// mgr acts as the communication between requestController and API server
// implements the RequestController interface
type k8sRequestController struct {
	mgr        manager.Manager //mgr has method GetClient() to get k8s client
	K8sClient  K8sClient       //Relying on the K8sClient interface to more easily test
	hostName   string          //name of node running this program
	Reconciler *NodeNetworkConfigReconciler
}

// GetKubeConfig precedence
// * --kubeconfig flag pointing at a file at this cmd line
// * KUBECONFIG environment variable pointing at a file
// * In-cluster config if running in cluster
// * $HOME/.kube/config if exists
func GetKubeConfig() (*rest.Config, error) {
	k8sconfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	return k8sconfig, nil
}

//NewK8sRequestController given a reference to CNS's HTTPRestService state, returns a k8sRequestController struct
func NewRequestController(restService *restserver.HTTPRestService, kubeconfig *rest.Config) (*k8sRequestController, error) {

	//Check that logger package has been intialized
	if logger.Log == nil {
		return nil, errors.New("Must initialize logger before calling")
	}

	// Check that HOSTNAME environment variable is set. HOSTNAME is name of node running this program
	// NODENAME
	hostName := os.Getenv("HOSTNAME")
	if hostName == "" {
		return nil, errors.New("Must declare HOSTNAME environment variable. HOSTNAME is name of node.")
	}

	//Add client-go scheme to runtime sheme so manager can recognize it
	var scheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, errors.New("Error adding client-go scheme to runtime scheme")
	}

	//Add CRD scheme to runtime sheme so manager can recognize it
	if err := nnc.AddToScheme(scheme); err != nil {
		return nil, errors.New("Error adding NodeNetworkConfig scheme to runtime scheme")
	}

	// Create manager for NodeNetworkConfigReconciler
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(kubeconfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		Namespace:          k8sNamespace,
	})
	if err != nil {
		logger.Errorf("[cns-rc] Error creating new request controller manager: %v", err)
		return nil, err
	}

	//Create k8scnsinteractor
	k8scnsinteractor := &K8sCNSInteractor{
		RestService: restService,
	}

	//Create reconciler
	nodenetworkconfigreconciler := &NodeNetworkConfigReconciler{
		K8sClient:     mgr.GetClient(),
		HostName:      hostName,
		CNSInteractor: k8scnsinteractor,
	}

	// Setup manager with reconciler
	if err := nodenetworkconfigreconciler.SetupWithManager(mgr); err != nil {
		logger.Errorf("[cns-rc] Error creating new NodeNetworkConfigReconciler: %v", err)
		return nil, err
	}

	// Create the requestController
	k8sRequestController := k8sRequestController{
		mgr:        mgr,
		K8sClient:  mgr.GetClient(),
		hostName:   hostName,
		Reconciler: nodenetworkconfigreconciler,
	}

	return &k8sRequestController, nil
}

// StartRequestController starts the reconcile loop. This loop waits for changes to CRD statuses.
// When a CRD status change is made, Reconcile from nodenetworkconfigreconciler is called.
// exitChan will be notified when requestController receives a kill signal
//This method blocks
func (k8sRC *k8sRequestController) StartRequestController(exitChan chan bool) error {
	// Start manager and consequently, the reconciler
	// Start() blocks until SIGINT or SIGTERM is received
	// When SIGINT or SIGTERm are recived, notifies exitChan before exiting
	go func() {//get rid of go routine
		logger.Printf("Starting manager")
		if err := k8sRC.mgr.Start(SetupSignalHandler(exitChan)); err != nil {
			logger.Errorf("[cns-rc] Error starting manager: %v", err)
		}
	}()

	return nil
}

// ReleaseIPsByUUIDs sends release ip request to the API server and updates requested ip count.
// Provide the UUIDs of the IP allocations and the new requested ip count
func (k8sRC *k8sRequestController) ReleaseIPsByUUIDs(cntxt context.Context, listOfIPUUIDS []string, newRequestedIPCount int) error {
	nodeNetworkConfig, err := k8sRC.getNodeNetConfig(cntxt, k8sRC.hostName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when releasing IPs by uuid %v", err)
		return err
	}

	//Update the CRD IpsNotInUse
	nodeNetworkConfig.Spec.IPsNotInUse = append(nodeNetworkConfig.Spec.IPsNotInUse, listOfIPUUIDS...)
	//Update the CRD requestedIPCount
	nodeNetworkConfig.Spec.RequestedIPCount = int64(newRequestedIPCount)

	//Send update to API server
	if err := k8sRC.updateNodeNetConfig(cntxt, nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD when releasing IPs by uuid %v", err)
		return err
	}

	return nil
}

ReconcileCNSState

// seperate pr for that

// getNodeNetConfig gets the nodeNetworkConfig CRD given the name and namespace of the CRD object
func (k8sRC *k8sRequestController) getNodeNetConfig(cntxt context.Context, name, namespace string) (*nnc.NodeNetworkConfig, error) {
	nodeNetworkConfig := &nnc.NodeNetworkConfig{}

	err := k8sRC.K8sClient.Get(cntxt, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nodeNetworkConfig)

	if err != nil {
		return nil, err
	}

	return nodeNetworkConfig, nil
}

// updateNodeNetConfig updates the nodeNetConfig object in the API server with the given nodeNetworkConfig object
func (k8sRC *k8sRequestController) updateNodeNetConfig(cntxt context.Context, nodeNetworkConfig *nnc.NodeNetworkConfig) error {
	if err := k8sRC.K8sClient.Update(cntxt, nodeNetworkConfig); err != nil {
		return err
	}

	return nil
}
