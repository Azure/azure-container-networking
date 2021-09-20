package kubecontroller

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/singletenantcontroller"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	nodeNameEnvVar = "NODENAME"
	k8sNamespace   = "kube-system"
	crdTypeName    = "nodenetworkconfigs"
	allNamespaces  = ""
)

// Config has crdRequestController options
type Config struct {
	// InitializeFromCNI whether or not to initialize CNS state from k8s/CRDs
	InitializeFromCNI  bool
	KubeConfig         *rest.Config
	MetricsBindAddress string
	Service            *restserver.HTTPRestService
}

var _ singletenantcontroller.RequestController = (*requestController)(nil)

type cnsrestservice interface {
	ReconcileNCState(*cns.CreateNetworkContainerRequest, map[string]cns.PodInfo, v1alpha.Scaler, v1alpha.NodeNetworkConfigSpec) types.ResponseCode
	CreateOrUpdateNetworkContainerInternal(*cns.CreateNetworkContainerRequest) types.ResponseCode
}

// requestController
// - watches CRD status changes
// - updates CRD spec
type requestController struct {
	cfg             Config
	mgr             manager.Manager // Manager starts the reconcile loop which watches for crd status changes
	KubeClient      KubeClient      // KubeClient is a cached client which interacts with API server
	directAPIClient DirectAPIClient // Direct client to interact with API server
	directCRDClient DirectCRDClient // Direct client to interact with CRDs on API server
	CNSRestService  cnsrestservice
	nodeName        string // name of node running this program
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

// New builds a requestController struct given a reference to CNS's HTTPRestService state.
func New(cfg Config) (*requestController, error) {
	// Check that logger package has been intialized
	if logger.Log == nil {
		return nil, errors.New("Must initialize logger before calling")
	}

	// Check that NODENAME environment variable is set. NODENAME is name of node running this program
	nodeName := os.Getenv(nodeNameEnvVar)
	if nodeName == "" {
		return nil, errors.New("Must declare " + nodeNameEnvVar + " environment variable.")
	}

	// Add client-go scheme to runtime sheme so manager can recognize it
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, errors.New("Error adding client-go scheme to runtime scheme")
	}

	// Add CRD scheme to runtime sheme so manager can recognize it
	if err := v1alpha.AddToScheme(scheme); err != nil {
		return nil, errors.New("Error adding NodeNetworkConfig scheme to runtime scheme")
	}

	// Create a direct client to the API server which we use to list pods when initializing cns state before reconcile loop
	directAPIClient, err := NewAPIDirectClient(cfg.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("Error creating direct API Client: %v", err)
	}

	// Create a direct client to the API server configured to get nodenetconfigs to get nnc for same reason above
	directCRDClient, err := NewCRDDirectClient(cfg.KubeConfig, &v1alpha.GroupVersion)
	if err != nil {
		return nil, fmt.Errorf("Error creating direct CRD client: %v", err)
	}

	// Create manager for CrdRequestController
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(cfg.KubeConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: cfg.MetricsBindAddress,
		Namespace:          k8sNamespace,
	})
	if err != nil {
		logger.Errorf("[cns-rc] Error creating new request controller manager: %v", err)
		return nil, err
	}

	// Create reconciler
	crdreconciler := &CrdReconciler{
		KubeClient:     mgr.GetClient(),
		NodeName:       nodeName,
		CNSRestService: cfg.Service,
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
		CNSRestService:  cfg.Service,
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
		if crd.IsNotDefined(err) {
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
	// Get nnc using direct client
	nnc, err := rc.getNodeNetConfigDirect(ctx, rc.nodeName, k8sNamespace)
	if err != nil {
		// If the CRD is not defined, exit
		if crd.IsNotDefined(err) {
			logger.Errorf("CRD is not defined on cluster: %v", err)
			os.Exit(1)
		}

		if nnc == nil {
			logger.Errorf("NodeNetworkConfig is not present on cluster")
			return nil
		}

		// If instance of crd is not found, pass nil to CNSRestService
		if client.IgnoreNotFound(err) == nil {
			//nolint:wrapcheck
			return restserver.ResponseCodeToError(rc.CNSRestService.ReconcileNCState(nil, nil, nnc.Status.Scaler, nnc.Spec))
		}

		// If it's any other error, log it and return
		logger.Errorf("Error when getting nodeNetConfig using direct client when initializing cns state: %v", err)
		return err
	}

	// If there are no NCs, pass nil to CNSRestService
	if len(nnc.Status.NetworkContainers) == 0 {
		//nolint:wrapcheck
		return restserver.ResponseCodeToError(rc.CNSRestService.ReconcileNCState(nil, nil, nnc.Status.Scaler, nnc.Spec))
	}

	// Convert to CreateNetworkContainerRequest
	ncRequest, err := CRDStatusToNCRequest(nnc.Status)
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
		podInfoByIPProvider = cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
			return rc.kubePodsToPodInfoByIP(pods.Items)
		})
	}

	podInfoByIP, err := podInfoByIPProvider.PodInfoByIP()
	if err != nil {
		return errors.Wrap(err, "err in CNS initialization")
	}

	// errors.Wrap provides additional context, and return nil if the err input arg is nil
	// Call CNSRestService init cns passing those two things.
	return errors.Wrap(restserver.ResponseCodeToError(rc.CNSRestService.ReconcileNCState(&ncRequest, podInfoByIP, nnc.Status.Scaler, nnc.Spec)), "err in CNS reconciliation")
}

// kubePodsToPodInfoByIP maps kubernetes pods to cns.PodInfos by IP
func (rc *requestController) kubePodsToPodInfoByIP(pods []corev1.Pod) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			if _, ok := podInfoByIP[pod.Status.PodIP]; ok {
				return nil, errors.Wrap(cns.ErrDuplicateIP, pod.Status.PodIP)
			}
			podInfoByIP[pod.Status.PodIP] = cns.NewPodInfo("", "", pod.Name, pod.Namespace)
		}
	}
	return podInfoByIP, nil
}

// UpdateCRDSpec updates the CRD spec
func (rc *requestController) UpdateCRDSpec(ctx context.Context, nnc v1alpha.NodeNetworkConfigSpec) error {
	nodeNetworkConfig, err := rc.getNodeNetConfig(ctx, rc.nodeName, k8sNamespace)
	if err != nil {
		logger.Errorf("[cns-rc] Error getting CRD when updating spec %v", err)
		return err
	}

	logger.Printf("[cns-rc] Received update for IP count %+v", nnc)

	// Update the CRD spec
	nnc.DeepCopyInto(&nodeNetworkConfig.Spec)

	logger.Printf("[cns-rc] After deep copy %+v", nodeNetworkConfig.Spec)

	// Send update to API server
	if err := rc.updateNodeNetConfig(ctx, nodeNetworkConfig); err != nil {
		logger.Errorf("[cns-rc] Error updating CRD spec %v", err)
		return err
	}

	// record IP metrics
	requestedIPs.Set(float64(nnc.RequestedIPCount))
	unusedIPs.Set(float64(len(nnc.IPsNotInUse)))
	return nil
}

// getNodeNetConfig gets the nodeNetworkConfig CRD given the name and namespace of the CRD object
func (rc *requestController) getNodeNetConfig(ctx context.Context, name, namespace string) (*v1alpha.NodeNetworkConfig, error) {
	nodeNetworkConfig := &v1alpha.NodeNetworkConfig{}

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
func (rc *requestController) getNodeNetConfigDirect(ctx context.Context, name, namespace string) (*v1alpha.NodeNetworkConfig, error) {
	//nolint:wrapcheck
	return rc.directCRDClient.Get(ctx, name, namespace, crdTypeName)
}

// updateNodeNetConfig updates the nodeNetConfig object in the API server with the given nodeNetworkConfig object
func (rc *requestController) updateNodeNetConfig(ctx context.Context, nnc *v1alpha.NodeNetworkConfig) error {
	//nolint:wrapcheck
	return rc.KubeClient.Update(ctx, nnc)
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
