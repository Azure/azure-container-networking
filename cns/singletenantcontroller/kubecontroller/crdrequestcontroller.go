package kubecontroller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/cnsclient/httpapi"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/singletenantcontroller"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	nodeNameEnvVar    = "NODENAME"
	k8sNamespace      = "kube-system"
	crdTypeName       = "nodenetworkconfigs"
	allNamespaces     = ""
	prometheusAddress = "0" //0 means disabled
)

// Config has crdRequestController options
type Config struct {
	// InitializeFromCNI whether or not to initialize CNS state from k8s/CRDs
	InitializeFromCNI bool
	KubeConfig        *rest.Config
	Service           *restserver.HTTPRestService
}

var _ singletenantcontroller.RequestController = (*requestController)(nil)

// requestController
// - watches CRD status changes
// - updates CRD spec
type requestController struct {
	cfg             Config
	mgr             manager.Manager //Manager starts the reconcile loop which watches for crd status changes
	KubeClient      KubeClient      //KubeClient is a cached client which interacts with API server
	directAPIClient DirectAPIClient //Direct client to interact with API server
	directCRDClient DirectCRDClient //Direct client to interact with CRDs on API server
	CNSClient       cnsclient.APIClient
	nodeName        string //name of node running this program
	Reconciler      *CrdReconciler
	initialized     bool
	Started         bool
	lock            sync.Mutex
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
func New(cfg Config) (*requestController, error) {

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

	// Create a direct client to the API server which we use to list pods when initializing cns state before reconcile loop
	directAPIClient, err := NewAPIDirectClient(cfg.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Error creating direct API Client: %v", err)
	}

	// Create a direct client to the API server configured to get nodenetconfigs to get nnc for same reason above
	directCRDClient, err := NewCRDDirectClient(cfg.KubeConfig, &nnc.GroupVersion)
	if err != nil {
		return nil, fmt.Errorf("Error creating direct CRD client: %v", err)
	}

	// Create manager for CrdRequestController
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(cfg.KubeConfig, ctrl.Options{
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
		RestService: cfg.Service,
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
	rc := requestController{
		cfg:             cfg,
		mgr:             mgr,
		KubeClient:      mgr.GetClient(),
		directAPIClient: directAPIClient,
		directCRDClient: directCRDClient,
		CNSClient:       httpClient,
		nodeName:        nodeName,
		Reconciler:      crdreconciler,
	}

	return &rc, nil
}

// Init will initialize/reconcile the CNS state
func (rc *requestController) Init(ctx context.Context) error {
	logger.Printf("InitRequestController")

	rc.lock.Lock()
	defer rc.lock.Unlock()

	if err := rc.initCNS(ctx); err != nil {
		logger.Errorf("[cns-rc] Error initializing cns state: %v", err)
		return err
	}

	rc.initialized = true
	return nil
}

// Start starts the Reconciler loop which watches for CRD status updates
func (rc *requestController) Start(ctx context.Context) error {
	logger.Printf("StartRequestController")

	rc.lock.Lock()
	if !rc.initialized {
		rc.lock.Unlock()
		return fmt.Errorf("Failed to start requestController, state is not initialized [%v]", rc)
	}

	// Setting the started state
	rc.Started = true
	rc.lock.Unlock()

	logger.Printf("Starting reconcile loop")
	if err := rc.mgr.Start(ctx); err != nil {
		if rc.isNotDefined(err) {
			logger.Errorf("[cns-rc] CRD is not defined on cluster, starting reconcile loop failed: %v", err)
			os.Exit(1)
		}

		return err
	}

	return nil
}

// return if RequestController is started
func (rc *requestController) IsStarted() bool {
	rc.lock.Lock()
	defer rc.lock.Unlock()
	return rc.Started
}

// InitCNS initializes cns by passing pods and a createnetworkcontainerrequest
func (rc *requestController) initCNS(ctx context.Context) error {
	// Get nodeNetConfig using direct client
	nodeNetConfig, err := rc.getNodeNetConfigDirect(ctx, rc.nodeName, k8sNamespace)
	if err != nil {
		// If the CRD is not defined, exit
		if rc.isNotDefined(err) {
			logger.Errorf("CRD is not defined on cluster: %v", err)
			os.Exit(1)
		}

		if nodeNetConfig == nil {
			logger.Errorf("NodeNetworkConfig is not present on cluster")
			return nil
		}

		// If instance of crd is not found, pass nil to CNSClient
		if client.IgnoreNotFound(err) == nil {
			return rc.CNSClient.ReconcileNCState(nil, nil, nodeNetConfig.Status.Scaler, nodeNetConfig.Spec)
		}

		// If it's any other error, log it and return
		logger.Errorf("Error when getting nodeNetConfig using direct client when initializing cns state: %v", err)
		return err
	}

	// If there are no NCs, pass nil to CNSClient
	if len(nodeNetConfig.Status.NetworkContainers) == 0 {
		return rc.CNSClient.ReconcileNCState(nil, nil, nodeNetConfig.Status.Scaler, nodeNetConfig.Spec)
	}

	// Convert to CreateNetworkContainerRequest
	ncRequest, err := CRDStatusToNCRequest(nodeNetConfig.Status)
	if err != nil {
		logger.Errorf("Error when converting nodeNetConfig status into CreateNetworkContainerRequest: %v", err)
		return err
	}

	var podInfoByIPProvider cns.PodInfoByIPProvider

	if rc.cfg.InitializeFromCNI {
		// rebuild CNS state from CNI
		logger.Printf("initializing CNS from CNI")
		podInfoByIPProvider, err = cnireconciler.NewCNIPodInfoProvider()
		if err != nil {
			return err
		}
	} else {
		logger.Printf("initializing CNS from apiserver")
		// Get all pods using direct client
		pods, err := rc.getAllPods(ctx, rc.nodeName)
		if err != nil {
			logger.Errorf("error when getting all pods when initializing cns: %v", err)
			return err
		}
		podInfoByIPProvider = cns.PodInfoByIPProviderFunc(func() map[string]cns.PodInfo {
			return rc.kubePodsToPodInfoByIP(pods.Items)
		})
	}

	// Call cnsclient init cns passing those two things
	return rc.CNSClient.ReconcileNCState(&ncRequest, podInfoByIPProvider.PodInfoByIP(), nodeNetConfig.Status.Scaler, nodeNetConfig.Spec)
}

// kubePodsToPodInfoByIP maps kubernetes pods to cns.PodInfos by IP
func (rc *requestController) kubePodsToPodInfoByIP(pods []corev1.Pod) map[string]cns.PodInfo {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			podInfoByIP[pod.Status.PodIP] = cns.NewPodInfo("", "", pod.Name, pod.Namespace)
		}
	}
	return podInfoByIP
}

// UpdateCRDSpec updates the CRD spec
func (rc *requestController) UpdateCRDSpec(ctx context.Context, crdSpec nnc.NodeNetworkConfigSpec) error {
	nodeNetworkConfig, err := rc.getNodeNetConfig(ctx, rc.nodeName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when updating spec %v", err)
		return err
	}

	logger.Printf("[cns-rc] Received update for IP count %+v", crdSpec)

	//Update the CRD spec
	crdSpec.DeepCopyInto(&nodeNetworkConfig.Spec)

	logger.Printf("[cns-rc] After deep copy %+v", nodeNetworkConfig.Spec)

	//Send update to API server
	if err := rc.updateNodeNetConfig(ctx, nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD spec %v", err)
		return err
	}

	return nil
}

// getNodeNetConfig gets the nodeNetworkConfig CRD given the name and namespace of the CRD object
func (rc *requestController) getNodeNetConfig(ctx context.Context, name, namespace string) (*nnc.NodeNetworkConfig, error) {
	nodeNetworkConfig := &nnc.NodeNetworkConfig{}

	err := rc.KubeClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nodeNetworkConfig)

	if err != nil {
		return nil, err
	}

	return nodeNetworkConfig, nil
}

// getNodeNetConfigDirect gets the nodeNetworkConfig CRD using a direct client
func (rc *requestController) getNodeNetConfigDirect(ctx context.Context, name, namespace string) (*nnc.NodeNetworkConfig, error) {
	var (
		nodeNetworkConfig *nnc.NodeNetworkConfig
		err               error
	)

	if nodeNetworkConfig, err = rc.directCRDClient.Get(ctx, name, namespace, crdTypeName); err != nil {
		return nil, err
	}

	return nodeNetworkConfig, nil
}

// updateNodeNetConfig updates the nodeNetConfig object in the API server with the given nodeNetworkConfig object
func (rc *requestController) updateNodeNetConfig(ctx context.Context, nodeNetworkConfig *nnc.NodeNetworkConfig) error {
	if err := rc.KubeClient.Update(ctx, nodeNetworkConfig); err != nil {
		return err
	}

	return nil
}

// getAllPods gets all pods running on the node using the direct API client
func (rc *requestController) getAllPods(ctx context.Context, node string) (*corev1.PodList, error) {
	var (
		pods *corev1.PodList
		err  error
	)

	if pods, err = rc.directAPIClient.ListPods(ctx, allNamespaces, node); err != nil {
		return nil, err
	}

	return pods, nil
}

// isNotDefined tells whether the given error is a CRD not defined error
func (rc *requestController) isNotDefined(err error) bool {
	var (
		statusError *apierrors.StatusError
		ok          bool
		notDefined  bool
		cause       metav1.StatusCause
	)

	if err == nil {
		return false
	}

	if statusError, ok = err.(*apierrors.StatusError); !ok {
		return false
	}

	if len(statusError.ErrStatus.Details.Causes) > 0 {
		for _, cause = range statusError.ErrStatus.Details.Causes {
			if cause.Type == metav1.CauseTypeUnexpectedServerResponse {
				if apierrors.IsNotFound(err) {
					notDefined = true
					break
				}
			}
		}
	}

	return notDefined
}
