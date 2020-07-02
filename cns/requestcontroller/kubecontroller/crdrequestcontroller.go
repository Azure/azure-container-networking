package kubecontroller

import (
	"context"
	"errors"
	"os"

	"github.com/Azure/azure-container-networking/cns/cnsclient/httpapi"
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

const nodeNameEnvVar = "NODENAME"
const k8sNamespace = "kube-system"
const prometheusAddress = "0" //0 means disabled

// crdRequestController
// - watches CRD status changes
// - updates CRD spec
type crdRequestController struct {
	mgr        manager.Manager //Manager starts the reconcile loop which watches for crd status changes
	KubeClient KubeClient      //KubeClient interacts with API server
	nodeName   string          //name of node running this program
	Reconciler *CrdReconciler
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

//NewCrdRequestController given a reference to CNS's HTTPRestService state, returns a crdRequestController struct
func NewCrdRequestController(restService *restserver.HTTPRestService, kubeconfig *rest.Config) (*crdRequestController, error) {

	//Check that logger package has been intialized
	if logger.Log == nil {
		return nil, errors.New("Must initialize logger before calling")
	}

	// Check that NODENAME environment variable is set. NODENAME is name of node running this program
	nodeName := os.Getenv(nodeNameEnvVar)
	if nodeName == "" {
		return nil, errors.New("Must declare " + nodeNameEnvVar + " environment variable.")
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

	// Create manager for CrdRequestController
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(kubeconfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: prometheusAddress,
		Namespace:          k8sNamespace,
	})
	if err != nil {
		logger.Errorf("[cns-rc] Error creating new request controller manager: %v", err)
		return nil, err
	}

	//Create httpClient
	httpClient := &httpapi.Client{
		RestService: restService,
	}

	//Create reconciler
	crdreconciler := &CrdReconciler{
		KubeClient: mgr.GetClient(),
		NodeName:   nodeName,
		CNSClient:  httpClient,
	}

	// Setup manager with reconciler
	if err := crdreconciler.SetupWithManager(mgr); err != nil {
		logger.Errorf("[cns-rc] Error creating new CrdRequestController: %v", err)
		return nil, err
	}

	// Create the requestController
	crdRequestController := crdRequestController{
		mgr:        mgr,
		KubeClient: mgr.GetClient(),
		nodeName:   nodeName,
		Reconciler: crdreconciler,
	}

	return &crdRequestController, nil
}

// StartRequestController starts the Reconciler loop which watches for CRD status updates
// Blocks until SIGINT or SIGTERM is received
// Notifies exitChan when kill signal received
func (crdRC *crdRequestController) StartRequestController(exitChan chan bool) error {
	logger.Printf("Starting manager")
	if err := crdRC.mgr.Start(SetupSignalHandler(exitChan)); err != nil {
		logger.Errorf("[cns-rc] Error starting manager: %v", err)
	}

	return nil
}

// UpdateCRDSpec updates the CRD spec
func (crdRC *crdRequestController) UpdateCRDSpec(cntxt context.Context, crdSpec *nnc.NodeNetworkConfigSpec) error {
	nodeNetworkConfig, err := crdRC.getNodeNetConfig(cntxt, crdRC.nodeName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when updating spec %v", err)
		return err
	}

	//Update the CRD spec
	crdSpec.DeepCopyInto(&nodeNetworkConfig.Spec)

	//Send update to API server
	if err := crdRC.updateNodeNetConfig(cntxt, nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD spec %v", err)
		return err
	}

	return nil
}

// getNodeNetConfig gets the nodeNetworkConfig CRD given the name and namespace of the CRD object
func (crdRC *crdRequestController) getNodeNetConfig(cntxt context.Context, name, namespace string) (*nnc.NodeNetworkConfig, error) {
	nodeNetworkConfig := &nnc.NodeNetworkConfig{}

	err := crdRC.KubeClient.Get(cntxt, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nodeNetworkConfig)

	if err != nil {
		return nil, err
	}

	return nodeNetworkConfig, nil
}

// updateNodeNetConfig updates the nodeNetConfig object in the API server with the given nodeNetworkConfig object
func (crdRC *crdRequestController) updateNodeNetConfig(cntxt context.Context, nodeNetworkConfig *nnc.NodeNetworkConfig) error {
	if err := crdRC.KubeClient.Update(cntxt, nodeNetworkConfig); err != nil {
		return err
	}

	return nil
}
