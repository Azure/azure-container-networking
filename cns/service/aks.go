// AKS specific initialization flows
// nolint // it's not worth it
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/ipampool"
	"github.com/Azure/azure-container-networking/cns/ipampool/metrics"
	ipampoolv2 "github.com/Azure/azure-container-networking/cns/ipampool/v2"
	cssctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/clustersubnetstate"
	mtpncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/multitenantpodnetworkconfig"
	nncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/nodenetworkconfig"
	podctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/pod"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate"
	cssv1alpha1 "github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/multitenancy"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/store"
	"github.com/avast/retry-go/v4"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
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

// initializeCRDState builds and starts the CRD controllers.
func initializeCRDState(ctx context.Context, httpRestService cns.HTTPService, cnsconfig *configuration.CNSConfig) error {
	// convert interface type to implementation type
	httpRestServiceImplementation, ok := httpRestService.(*restserver.HTTPRestService)
	if !ok {
		logger.Errorf("[Azure CNS] Failed to convert interface httpRestService to implementation: %v", httpRestService)
		return fmt.Errorf("[Azure CNS] Failed to convert interface httpRestService to implementation: %v",
			httpRestService)
	}

	// Set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.KubernetesCRD,
	}
	httpRestServiceImplementation.SetNodeOrchestrator(&orchestrator)

	// build default clientset.
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to get kubeconfig for request controller: %v", err)
		return errors.Wrap(err, "failed to get kubeconfig")
	}
	kubeConfig.UserAgent = fmt.Sprintf("azure-cns-%s", version)

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build clientset")
	}

	// get nodename for scoping kube requests to node.
	nodeName, err := configuration.NodeName()
	if err != nil {
		return errors.Wrap(err, "failed to get NodeName")
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get node %s", nodeName)
	}

	// check the Node labels for Swift V2
	if _, ok := node.Labels[configuration.LabelNodeSwiftV2]; ok {
		cnsconfig.EnableSwiftV2 = true
		cnsconfig.WatchPods = true
		if nodeInfoErr := createOrUpdateNodeInfoCRD(ctx, kubeConfig, node); nodeInfoErr != nil {
			return errors.Wrap(nodeInfoErr, "error creating or updating nodeinfo crd")
		}
	}

	// perform state migration from CNI in case CNS is set to manage the endpoint state and has emty state
	if cnsconfig.EnableStateMigration && !httpRestServiceImplementation.EndpointStateStore.Exists() {
		if err = populateCNSEndpointState(httpRestServiceImplementation.EndpointStateStore); err != nil {
			return errors.Wrap(err, "failed to create CNS EndpointState From CNI")
		}
		// endpoint state needs tobe loaded in memory so the subsequent Delete calls remove the state and release the IPs.
		if err = httpRestServiceImplementation.EndpointStateStore.Read(restserver.EndpointStoreKey, &httpRestServiceImplementation.EndpointState); err != nil {
			return errors.Wrap(err, "failed to restore endpoint state")
		}
	}

	podInfoByIPProvider, err := getPodInfoByIPProvider(ctx, cnsconfig, httpRestServiceImplementation, clientset, nodeName)
	if err != nil {
		return errors.Wrap(err, "failed to initialize ip state")
	}

	// create scoped kube clients.
	directcli, err := client.New(kubeConfig, client.Options{Scheme: nodenetworkconfig.Scheme})
	if err != nil {
		return errors.Wrap(err, "failed to create ctrl client")
	}
	directnnccli := nodenetworkconfig.NewClient(directcli)
	if err != nil {
		return errors.Wrap(err, "failed to create NNC client")
	}
	// TODO(rbtr): nodename and namespace should be in the cns config
	directscopedcli := nncctrl.NewScopedClient(directnnccli, types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	logger.Printf("Reconciling initial CNS state")
	// apiserver nnc might not be registered or api server might be down and crashloop backof puts us outside of 5-10 minutes we have for
	// aks addons to come up so retry a bit more aggresively here.
	// will retry 10 times maxing out at a minute taking about 8 minutes before it gives up.
	attempt := 0
	err = retry.Do(func() error {
		attempt++
		logger.Printf("reconciling initial CNS state attempt: %d", attempt)
		err = reconcileInitialCNSState(ctx, directscopedcli, httpRestServiceImplementation, podInfoByIPProvider)
		if err != nil {
			logger.Errorf("failed to reconcile initial CNS state, attempt: %d err: %v", attempt, err)
		}
		return errors.Wrap(err, "failed to initialize CNS state")
	}, retry.Context(ctx), retry.Delay(initCNSInitalDelay), retry.MaxDelay(time.Minute))
	if err != nil {
		return err
	}
	logger.Printf("reconciled initial CNS state after %d attempts", attempt)

	scheme := kuberuntime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil { //nolint:govet // intentional shadow
		return errors.Wrap(err, "failed to add corev1 to scheme")
	}
	if err = v1alpha.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add nodenetworkconfig/v1alpha to scheme")
	}
	if err = cssv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add clustersubnetstate/v1alpha1 to scheme")
	}
	if err = mtv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add multitenantpodnetworkconfig/v1alpha1 to scheme")
	}

	// Set Selector options on the Manager cache which are used
	// to perform *server-side* filtering of the cached objects. This is very important
	// for high node/pod count clusters, as it keeps us from watching objects at the
	// whole cluster scope when we are only interested in the Node's scope.
	cacheOpts := cache.Options{
		Scheme: scheme,
		ByObject: map[client.Object]cache.ByObject{
			&v1alpha.NodeNetworkConfig{}: {
				Namespaces: map[string]cache.Config{
					"kube-system": {FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": nodeName})},
				},
			},
		},
	}

	if cnsconfig.WatchPods {
		cacheOpts.ByObject[&corev1.Pod{}] = cache.ByObject{
			Field: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
		}
	}

	if cnsconfig.EnableSubnetScarcity {
		cacheOpts.ByObject[&cssv1alpha1.ClusterSubnetState{}] = cache.ByObject{
			Namespaces: map[string]cache.Config{
				"kube-system": {},
			},
		}
	}

	managerOpts := ctrlmgr.Options{
		Scheme:  scheme,
		Metrics: ctrlmetrics.Options{BindAddress: "0"},
		Cache:   cacheOpts,
		Logger:  ctrlzap.New(),
	}

	manager, err := ctrl.NewManager(kubeConfig, managerOpts)
	if err != nil {
		return errors.Wrap(err, "failed to create manager")
	}

	// this cachedscopedclient is built using the Manager's cached client, which is
	// NOT SAFE TO USE UNTIL THE MANAGER IS STARTED!
	// This is okay because it is only used to build the IPAMPoolMonitor, which does not
	// attempt to use the client until it has received a NodeNetworkConfig to update, and
	// that can only happen once the Manager has started and the NodeNetworkConfig
	// reconciler has pushed the Monitor a NodeNetworkConfig.
	cachedscopedcli := nncctrl.NewScopedClient(nodenetworkconfig.NewClient(manager.GetClient()), types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	// Build the IPAM Pool monitor
	var poolMonitor cns.IPAMPoolMonitor
	cssCh := make(chan cssv1alpha1.ClusterSubnetState)
	ipDemandCh := make(chan int)
	if cnsconfig.EnableIPAMv2 {
		cssSrc := func(context.Context) ([]cssv1alpha1.ClusterSubnetState, error) { return nil, nil }
		if cnsconfig.EnableSubnetScarcity {
			cssSrc = clustersubnetstate.NewClient(manager.GetClient()).List
		}
		nncCh := make(chan v1alpha.NodeNetworkConfig)
		pmv2 := ipampoolv2.NewMonitor(z, httpRestServiceImplementation, cachedscopedcli, ipDemandCh, nncCh, cssCh)
		obs := metrics.NewLegacyMetricsObserver(httpRestService.GetPodIPConfigState, cachedscopedcli.Get, cssSrc)
		pmv2.WithLegacyMetricsObserver(obs)
		poolMonitor = pmv2.AsV1(nncCh)
	} else {
		poolOpts := ipampool.Options{
			RefreshDelay: poolIPAMRefreshRateInMilliseconds * time.Millisecond,
		}
		poolMonitor = ipampool.NewMonitor(httpRestServiceImplementation, cachedscopedcli, cssCh, &poolOpts)
	}

	// Start building the NNC Reconciler

	// get CNS Node IP to compare NC Node IP with this Node IP to ensure NCs were created for this node
	nodeIP := configuration.NodeIP()
	nncReconciler := nncctrl.NewReconciler(httpRestServiceImplementation, poolMonitor, nodeIP)
	// pass Node to the Reconciler for Controller xref
	// IPAMv1 - reconcile only status changes (where generation doesn't change).
	// IPAMv2 - reconcile all updates.
	filterGenerationChange := !cnsconfig.EnableIPAMv2
	if err := nncReconciler.SetupWithManager(manager, node, filterGenerationChange); err != nil { //nolint:govet // intentional shadow
		return errors.Wrapf(err, "failed to setup nnc reconciler with manager")
	}

	if cnsconfig.EnableSubnetScarcity {
		// ClusterSubnetState reconciler
		cssReconciler := cssctrl.New(cssCh)
		if err := cssReconciler.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup css reconciler with manager")
		}
	}

	// TODO: add pod listeners based on Swift V1 vs MT/V2 configuration
	if cnsconfig.WatchPods {
		pw := podctrl.New(z)
		if cnsconfig.EnableIPAMv2 {
			hostNetworkListOpt := &client.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"spec.hostNetwork": "false"})} // filter only podsubnet pods
			// don't relist pods more than every 500ms
			limit := rate.NewLimiter(rate.Every(500*time.Millisecond), 1) //nolint:gomnd // clearly 500ms
			pw.With(pw.NewNotifierFunc(hostNetworkListOpt, limit, ipampoolv2.PodIPDemandListener(ipDemandCh)))
		}
		if err := pw.SetupWithManager(ctx, manager); err != nil {
			return errors.Wrapf(err, "failed to setup pod watcher with manager")
		}
	}

	if cnsconfig.EnableSwiftV2 {
		if err := mtpncctrl.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup mtpnc reconciler with manager")
		}
		// if SWIFT v2 is enabled on CNS, attach multitenant middleware to rest service
		// switch here for AKS(K8s) swiftv2 middleware to process IP configs requests
		swiftV2Middleware := &middlewares.K8sSWIFTv2Middleware{Cli: manager.GetClient()}
		httpRestService.AttachIPConfigsHandlerMiddleware(swiftV2Middleware)
	}

	// start the pool Monitor before the Reconciler, since it needs to be ready to receive an
	// NodeNetworkConfig update by the time the Reconciler tries to send it.
	go func() {
		logger.Printf("Starting IPAM Pool Monitor")
		if e := poolMonitor.Start(ctx); e != nil {
			logger.Errorf("[Azure CNS] Failed to start pool monitor with err: %v", e)
		}
	}()
	logger.Printf("initialized and started IPAM pool monitor")

	// Start the Manager which starts the reconcile loop.
	// The Reconciler will send an initial NodeNetworkConfig update to the PoolMonitor, starting the
	// Monitor's internal loop.
	go func() {
		logger.Printf("Starting controller-manager.")
		for {
			if err := manager.Start(ctx); err != nil {
				logger.Errorf("Failed to start controller-manager: %v", err)
				// retry to start the request controller
				// inc the managerStartFailures metric for failure tracking
				managerStartFailures.Inc()
			} else {
				logger.Printf("Stopped controller-manager.")
				return
			}
			time.Sleep(time.Second) // TODO(rbtr): make this exponential backoff
		}
	}()
	logger.Printf("Initialized controller-manager.")
	for {
		logger.Printf("Waiting for NodeNetworkConfig reconciler to start.")
		// wait for the Reconciler to run once on a NNC that was made for this Node.
		// the nncReadyCtx has a timeout of 15 minutes, after which we will consider
		// this false and the NNC Reconciler stuck/failed, log and retry.
		nncReadyCtx, cancel := context.WithTimeout(ctx, 15*time.Minute) // nolint // it will time out and not leak
		if started, err := nncReconciler.Started(nncReadyCtx); !started {
			logger.Errorf("NNC reconciler has not started, does the NNC exist? err: %v", err)
			nncReconcilerStartFailures.Inc()
			continue
		}
		logger.Printf("NodeNetworkConfig reconciler has started.")
		cancel()
		break
	}

	go func() {
		logger.Printf("Starting SyncHostNCVersion loop.")
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				timedCtx, cancel := context.WithTimeout(ctx, time.Duration(cnsconfig.SyncHostNCVersionIntervalMs)*time.Millisecond)
				httpRestServiceImplementation.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode)
				cancel()
			case <-ctx.Done():
				logger.Printf("Stopping SyncHostNCVersion loop.")
				return
			}
		}
	}()
	logger.Printf("Initialized SyncHostNCVersion loop.")
	return nil
}

// createOrUpdateNodeInfoCRD polls imds to learn the VM Unique ID and then creates or updates the NodeInfo CRD
// with that vm unique ID
func createOrUpdateNodeInfoCRD(ctx context.Context, restConfig *rest.Config, node *corev1.Node) error {
	imdsCli := imds.NewClient()
	vmUniqueID, err := imdsCli.GetVMUniqueID(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting vm unique ID from imds")
	}

	directcli, err := client.New(restConfig, client.Options{Scheme: multitenancy.Scheme})
	if err != nil {
		return errors.Wrap(err, "failed to create ctrl client")
	}

	nodeInfoCli := multitenancy.NodeInfoClient{
		Cli: directcli,
	}

	nodeInfo := &mtv1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: mtv1alpha1.NodeInfoSpec{
			VMUniqueID: vmUniqueID,
		},
	}

	if err := controllerutil.SetOwnerReference(node, nodeInfo, multitenancy.Scheme); err != nil {
		return errors.Wrap(err, "failed to set nodeinfo owner reference to node")
	}

	if err := nodeInfoCli.CreateOrUpdate(ctx, nodeInfo, "azure-cns"); err != nil {
		return errors.Wrap(err, "error ensuring nodeinfo CRD exists and is up-to-date")
	}

	return nil
}

// populateCNSEndpointState initilizes CNS Endpoint State by Migrating the CNI state.
func populateCNSEndpointState(endpointStateStore store.KeyValueStore) error {
	logger.Printf("State Migration is enabled")
	endpointState, err := cnireconciler.MigrateCNISate()
	if err != nil {
		return errors.Wrap(err, "failed to create CNS Endpoint state from CNI")
	}
	err = endpointStateStore.Write(restserver.EndpointStoreKey, endpointState)
	if err != nil {
		return fmt.Errorf("failed to write endpoint state to store: %w", err)
	}
	return nil
}
