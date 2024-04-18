//go:build swiftv2

package swiftv2

import (
	"context"
	"flag"
	"testing"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/goldpinger"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
)

const (
	podLabelKey                = "app"
	podCount                   = 2
	nodepoolKey                = "mtapool"
	LinuxDeployIPV4            = "../manifests/datapath/linux-deployment.yaml"
	podNetworkYaml             = "../manifests/swiftv2/podnetwork.yaml"
	mtpodYaml                  = "../manifests/swiftv2/mtpod0.yaml"
	pniYaml                    = "../manifests/swiftv2/pni.yaml"
	maxRetryDelaySeconds       = 10
	defaultTimeoutSeconds      = 120
	defaultRetryDelaySeconds   = 1
	goldpingerRetryCount       = 24
	goldpingerDelayTimeSeconds = 5
	gpFolder                   = "../manifests/goldpinger"
	gpClusterRolePath          = gpFolder + "/cluster-role.yaml"
	gpClusterRoleBindingPath   = gpFolder + "/cluster-role-binding.yaml"
	gpServiceAccountPath       = gpFolder + "/service-account.yaml"
	gpDaemonset                = gpFolder + "/daemonset.yaml"
	gpDaemonsetIPv6            = gpFolder + "/daemonset-ipv6.yaml"
	gpDeployment               = gpFolder + "/deployment.yaml"
)

var (
	podPrefix        = flag.String("podName", "mta", "Prefix for test pods")
	podNamespace     = flag.String("namespace", "default", "Namespace for test pods")
	nodepoolSelector = flag.String("nodepoolSelector", "mtapool", "Provides nodepool as a Linux Node-Selector for pods")
	// TODO: add flag to support dual nic scenario
	isDualStack    = flag.Bool("isDualStack", false, "whether system supports dualstack scenario")
	defaultRetrier = retry.Retrier{
		Attempts: 10,
		Delay:    defaultRetryDelaySeconds * time.Second,
	}
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a Linux nodepool consisting of at least 2 Linux nodes.
*** The expected nodepool name is mtapool, if the nodepool has a different name ensure that you change nodepoolSelector with:
		-nodepoolSelector="yournodepoolname"

This test checks pod to pod, pod to node, pod to Internet check

Timeout context is controled by the -timeout flag.

*/

func setupLinuxEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()

	t.Log("Create Label Selectors")
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)
	nodeLabelSelector := kubernetes.CreateLabelSelector(nodepoolKey, nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := kubernetes.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		t.Fatalf("could not get k8s node list: %v", err)
	}

	// shchen comment out
	// t.Log("Creating Linux pods through deployment")

	// // run goldpinger ipv4 and ipv6 test cases saperately
	// var daemonset appsv1.DaemonSet
	// var deployment appsv1.Deployment

	// deployment = kubernetes.MustParseDeployment(LinuxDeployIPV4)
	// daemonset = kubernetes.MustParseDaemonSet(gpDaemonset)

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	// rbacSetupFn := kubernetes.MustSetUpClusterRBAC(ctx, clientset, gpClusterRolePath, gpClusterRoleBindingPath, gpServiceAccountPath)

	// Fields for overwritting existing deployment yaml.
	// Defaults from flags will not change anything
	// deployment.Spec.Selector.MatchLabels[podLabelKey] = *podPrefix
	// deployment.Spec.Template.ObjectMeta.Labels[podLabelKey] = *podPrefix
	// deployment.Spec.Template.Spec.NodeSelector[nodepoolKey] = *nodepoolSelector
	// deployment.Name = *podPrefix
	// deployment.Namespace = *podNamespace
	// daemonset.Namespace = *podNamespace

	// deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
	// kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)

	// daemonsetClient := clientset.AppsV1().DaemonSets(daemonset.Namespace)
	// kubernetes.MustCreateDaemonset(ctx, daemonsetClient, daemonset)

	// t.Cleanup(func() {
	// 	t.Log("cleaning up resources")
	// 	rbacSetupFn()

	// 	if err := deploymentsClient.Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
	// 		t.Log(err)
	// 	}

	// 	if err := daemonsetClient.Delete(ctx, daemonset.Name, metav1.DeleteOptions{}); err != nil {
	// 		t.Log(err)
	// 	}
	// })

	t.Log("Waiting for pods to be running state")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
	if err != nil {
		t.Fatalf("Pods are not in running state due to %+v", err)
	}

	t.Log("Successfully created customer Linux pods")

	t.Log("Checking swiftv2 multitenant pods number")
	for _, node := range nodes.Items {
		pods, err := kubernetes.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			t.Fatalf("could not get k8s clientset: %v", err)
		}
		if len(pods.Items) <= 1 {
			t.Fatalf("Less than 2 pods on node: %v", node.Name)
		}
	}

	t.Log("Linux test environment ready")
}

// func TestPodToPodSourceIP(config ergonomic.Config,
// 	kubeconfig string,
// 	settings *PodToPodSettings) {
// 	ctx := config.Ctx()
// 	kubeClient := clientgen.Default(kubeconfig)
// 	k8sClient := client.MustCreateK8SClientFromKubeConfig(kubeconfig)

// 	// Create namespace.
// 	logger := config.Logger("k8sPodToVMSourceIP")
// 	logger.LogKV("step", "create namespace in the cluster")
// 	nsCreated := clientgen.EnsureNamespaceExists(
// 		ctx,
// 		kubeClient,
// 		logger,
// 		"pod-pod-src-ip-"+e2enaming.GenerateRandomName(5),
// 	)
// 	logger.Logf("namespace %q created successfully", nsCreated.Name)

// 	// Create pod network instance if labels exist
// 	if settings.PodLabels != nil {
// 		if podNetwork := settings.PodLabels["kubernetes.azure.com/pod-network"]; podNetwork != "" {
// 			if podNetworkInstance := settings.PodLabels["kubernetes.azure.com/pod-network-instance"]; podNetworkInstance != "" {
// 				CreatePodNetworkInstance(ctx, config.RunCtx(), kubeconfig, nsCreated.Name, podNetworkInstance, podNetwork, settings.NodeCountLinux)
// 			}
// 		}
// 	}

// 	// Create hostNetwork deployment for the agnhost image from k/k E2E test framework.
// 	// AgnHost's netexec command runs an HTTP server with an endpoint /clientip that echos the client's source IP.
// 	deploymentPods := CreateAgnHostDeployment(
// 		config.RunCtx(),
// 		ctx,
// 		k8sClient,
// 		kubeClient,
// 		logger,
// 		nsCreated.Name,
// 		"linux",
// 		settings.DestPort,
// 		settings.HostNetwork,
// 		1,
// 		false,
// 	)

// 	if settings.NodeCountWindows > 0 {
// 		deploymentPods = append(deploymentPods,
// 			CreateAgnHostDeployment(
// 				config.RunCtx(),
// 				ctx,
// 				k8sClient,
// 				kubeClient,
// 				logger,
// 				nsCreated.Name,
// 				"windows",
// 				settings.DestPort,
// 				settings.HostNetwork,
// 				1,
// 				false,
// 			)...)
// 	}

// 	// Create daemonset of curl pods, one on each node.
// 	curlPods := CreateCurlDaemonset(
// 		config.RunCtx(),
// 		ctx,
// 		k8sClient,
// 		kubeClient,
// 		logger,
// 		nsCreated.Name,
// 		settings.NodeCountLinux,
// 		settings.NodeCountWindows,
// 		settings.PodLabels,
// 	)

// 	cURLs := []string{}
// 	ipFamily := settings.IPFamily
// 	for _, deploymentPod := range deploymentPods {
// 		// From each curl pod, request /clientip from each agnhost deployment pod.
// 		// This will echo back the source IP as seen by the server, which we expect to be equal to the curl pod's IP unless SNAT is expected

// 		clientIpURL := getClientIPEndpoint(getPodIPForFamily(deploymentPod.Status.PodIPs, ipFamily), ipFamily, deploymentPod.Spec.Containers[0].Ports[0].ContainerPort)
// 		m.Expect(clientIpURL).NotTo(m.BeEmpty(),
// 			"no valid IP in family %s found for pod %s",
// 			ipFamily,
// 			deploymentPod.Name)
// 		cURLs = append(cURLs, clientIpURL)

// 		if settings.VerifyHostPort && !settings.HostNetwork {
// 			cURLs = append(cURLs, fmt.Sprintf("http://%s:%d", deploymentPod.Status.HostIP, deploymentPod.Spec.Containers[0].Ports[0].HostPort))
// 		}
// 	}

// 	logger.LogKV("step", "curl from each node")
// 	for _, curlPod := range curlPods {
// 		for _, url := range cURLs {
// 			var expectedPodIP string
// 			if settings.SNATExpected {
// 				node, err := k8sClient.
// 					Clientset.
// 					CoreV1().
// 					Nodes().
// 					Get(ctx, curlPod.Spec.NodeName, k8smetav1.GetOptions{})
// 				m.Expect(err).NotTo(m.HaveOccurred())

// 				// get the ipfamily of the curl pod, then get the node ip of pod (host)
// 				expectedPodIP = getInternalNodeIPForFamily(*node, ipFamily).String()
// 				m.Expect(expectedPodIP).NotTo(m.BeNil(), "no host IP found for pod %s", curlPod.Name)
// 				if strings.Contains(url, curlPod.Status.HostIP) || checkIPsInNode(*node, url) {
// 					continue
// 				}
// 			} else {
// 				expectedPodIP = getPodIPForFamily(curlPod.Status.PodIPs, ipFamily).String()
// 			}

// 			m.Expect(expectedPodIP).NotTo(m.BeEmpty(),
// 				"no valid IP in family %s found for pod %s",
// 				ipFamily,
// 				curlPod.Name)

// 			// windows pod to its own host port times out
// 			if (settings.VerifyHostPort || settings.HostNetwork) && strings.Contains(curlPod.Name, "windows") && strings.Contains(url, curlPod.Status.HostIP) {
// 				continue
// 			}

// 			logger.Logf("curl from pod %q to %q", curlPod.Name, url)
// 			result, err := retry.DoFixedRetryWithMaxCount(
// 				func() retry.Result {
// 					stdout, stderr, err := clientgen.PodExecWithError(
// 						logger,
// 						kubeconfig,
// 						curlPod.Name,
// 						curlPod.Namespace,
// 						[]string{"curl", "-g", "--max-time", "30", url},
// 					)
// 					if err != nil {
// 						logger.Logf("curl %q request failed: error: %s, stdout: %s, stderr: %s", url, err, stdout, stderr)
// 						return retry.Result{
// 							Status: retry.NeedRetry,
// 							Body:   "curl request failed",
// 						}
// 					}

// 					if strings.Contains(url, "/clientip") {

// 						logger.Logf("checking response from agnhost endpoint contains expected IP. "+
// 							"endpoint=%s\nexpected IP=%s\nstdout=%s\nstderr=%s\n", url, expectedPodIP, stdout, stderr)
// 						if strings.Contains(stdout, expectedPodIP) {
// 							logger.Logf("found expected IP %s in stdout %s", expectedPodIP, stdout)
// 						} else {
// 							logger.Logf("stdout %s does not contain expected IP %s", stdout, expectedPodIP)
// 							return retry.Result{Status: retry.NeedRetry, Body: stdout}
// 						}

// 					}

// 					return retry.Result{
// 						Status: retry.Success,
// 						Body:   stdout,
// 					}
// 				},
// 				podExecRetryInterval,
// 				podExecRetryTimeout,
// 				podExecRetryMaxAttempts)

// 			m.Expect(err).NotTo(m.HaveOccurred(), "err: %s", err)
// 			m.Expect(result.Status).To(m.Equal(retry.Success))
// 		}
// 	}

// 	// Cleanup by deleting the namespace.
// 	logger.LogKV("step", "delete namespace in the cluster")
// 	clientgen.EnsureNamespaceDeleted(ctx, kubeClient, logger, nsCreated.Name)
// 	logger.Logf("deleted namespace %q", nsCreated.Name)
// }

// func GetMultitenantPodNetworkConfig(ctx context.Context, runCtx e2ev2.RunContext, kubeconfig, namespace, name string) v1alpha1.MultitenantPodNetworkConfig {
// 	crdClient, err := GetRESTClientForMultitenantCRD(kubeconfig)
// 	m.Expect(err).NotTo(m.HaveOccurred(), "failed to get multitenant crd rest client: %s", err)
// 	logger := runCtx.Logger("getMultitenantPodNetworkConfig")
// 	var mtpnc v1alpha1.MultitenantPodNetworkConfig
// 	retryResult := retry.RetryWithMaxCountWithContext(
// 		context.Background(),
// 		func() (*retry.Result, *cgerror.CategorizedError) {
// 			err = crdClient.Get().Namespace(namespace).Resource("multitenantpodnetworkconfigs").Name(name).Do(ctx).Into(&mtpnc)
// 			if err != nil {
// 				logger.Logf("failed to retrieve multitenantpodnetworkconfig: error: %s", err)
// 				retriable := true
// 				return &retry.Result{
// 						Status: retry.NeedRetry,
// 						Body:   err,
// 					}, &cgerror.CategorizedError{
// 						Retriable: &retriable,
// 					}
// 			}
// 			if mtpnc.Status.MacAddress == "" || mtpnc.Status.PrimaryIP == "" {
// 				retriable := true
// 				return &retry.Result{
// 						Status: retry.NeedRetry,
// 						Body:   "waiting for mtpnc to be ready",
// 					}, &cgerror.CategorizedError{
// 						Retriable: &retriable,
// 					}
// 			}
// 			return &retry.Result{
// 				Status: retry.Success,
// 				Body:   err,
// 			}, nil
// 		},
// 		podExecRetryInterval,
// 		podExecRetryTimeout,
// 		podExecRetryMaxAttempts,
// 		retry.FixedType,
// 	)
// 	m.Expect(retryResult.Status).To(m.Equal(retry.Success))
// 	return mtpnc
// }

// func GetRESTClientForMultitenantCRD(kubeconfig string) (*rest.RESTClient, error) {
// 	scheme := runtime.NewScheme()
// 	err := acnv1alpha1.AddToScheme(scheme)
// 	if err != nil {
// 		return nil, err
// 	}

// 	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
// 	if err != nil {
// 		return nil, err
// 	}

// 	restConfig.ContentConfig.GroupVersion = &acnv1alpha1.GroupVersion
// 	restConfig.APIPath = "/apis"
// 	restConfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
// 	restConfig.UserAgent = rest.DefaultKubernetesUserAgent()

// 	return rest.UnversionedRESTClientFor(restConfig)
// }

// // EnsureAllPodsAreRunning expects all pods returned from getPods are running.
// func EnsureAllPodsAreRunning(
// 	ctx context.Context,
// 	k Interface,
// 	runCtxLogger e2ev2.RunContextLogger,
// 	getPods func(k Interface) ([]k8scorev1.Pod, error),
// 	checkOpt *CheckPodOption,
// ) []k8scorev1.Pod {
// 	return k.Failer().MustPodList(ExpectAllPodsAreRunning(ctx, k, runCtxLogger, getPods, checkOpt))
// }

// // PodExecWithError executes a command in a pod by using its first container.
// // It returns the stdout, stderr and error.
// func PodExecWithError(
// 	logger e2ev2.RunContextLogger,
// 	kubeconfig string,
// 	podName string, namespace string,
// 	command []string,
// ) (string, string, error) {
// 	clientcmdConfig, err := k8sclientcmd.Load([]byte(kubeconfig))
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to load kube config: %w", err)
// 	}

// 	directClientcmdConfig := k8sclientcmd.NewNonInteractiveClientConfig(
// 		*clientcmdConfig,
// 		"", // default context
// 		&k8sclientcmd.ConfigOverrides{},
// 		nil, // config access
// 	)

// 	clientRestConfig, err := directClientcmdConfig.ClientConfig()
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to create kube client config: %w", err)
// 	}
// 	WrapClientRestConfigWithRetry(clientRestConfig)

// 	clientRestConfig.Timeout = 10 * time.Minute

// 	client, err := k8sclientset.NewForConfig(clientRestConfig)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to create kube clientset: %w", err)
// 	}

// 	pod, err := client.CoreV1().Pods(namespace).Get(context.TODO(), podName, k8smetav1.GetOptions{})
// 	if err != nil {
// 		return "", "", fmt.Errorf("get pod: %w", err)
// 	}
// 	containerName := pod.Spec.Containers[0].Name
// 	req := client.CoreV1().RESTClient().Post().
// 		Resource("pods").
// 		Name(podName).
// 		Namespace(namespace).
// 		SubResource("exec").
// 		Param("container", containerName)

// 	req.VersionedParams(&k8scorev1.PodExecOptions{
// 		Command:   command,
// 		Stdin:     false,
// 		Stdout:    true,
// 		Stderr:    true,
// 		TTY:       false,
// 		Container: containerName,
// 	}, k8sscheme.ParameterCodec)

// 	var stdout, stderr bytes.Buffer
// 	executor, err := remotecommand.NewSPDYExecutor(clientRestConfig, "POST", req.URL())
// 	if err != nil {
// 		return "", "", fmt.Errorf("NewSPDYExecutor: %w", err)
// 	}

// 	// NOTE: remotecommand is not a Kubernetes pod resource API used here, but a tool API.
// 	ctx := context.Background()
// 	// yes, 3 mins is a magic number
// 	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
// 	defer cancel()
// 	logger.Logf("executing command: %s", strings.Join(command, " "))
// 	readStreamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
// 		Stdin:  nil,
// 		Stdout: &stdout,
// 		Stderr: &stderr,
// 		Tty:    false,
// 	})

// 	// FIXME(hbc): Windows validation expect stdout/stderr output even seeing error
// 	//             therefore we need to return the stdout/stderr output here
// 	stdoutRead := strings.TrimSpace(stdout.String())
// 	stderrRead := strings.TrimSpace(stderr.String())
// 	return stdoutRead, stderrRead, readStreamErr
// }

// func HandleSwiftv2PodToPodTestcase() e2e.Handler {
// 	var (
// 		kubeconfig string
// 		namespace  string
// 		numNodes   int
// 		podLabels  map[string]string
// 	)

// 	fs := schemahelper.NewFlagSet()
// 	fs.RequiredStringVar(&kubeconfig, "kube_config", "First AKS cluster credentials to use")
// 	fs.StringVar(&namespace, "namespace", "", "namespace to deploy, generate if not specified")
// 	fs.IntVar(&numNodes, "num_linux_nodes", 2, "number of linux nodes in cluster")
// 	fs.StringMapStringVar(&podLabels, "pod_labels", nil, "client pod labels")

// 	return &handler{
// 		name:             Swiftv2PodToPod,
// 		parametersSchema: fs.BuildParametersSchema(),
// 		handler: func(runCtx e2e.RunContext) {
// 			fs.Parse(runCtx.Parameters())
// 			logger := runCtx.Logger("k8sPodConnectionCrossClusterWithPeeredNetwork")
// 			ctx := context.Background()

// 			if namespace == "" {
// 				namespace = generateNamespace()
// 			}

// 			// Create pod network instance
// 			m.Expect(podLabels).ToNot(m.BeNil())
// 			podNetwork := podLabels["kubernetes.azure.com/pod-network"]
// 			podNetworkInstance := podLabels["kubernetes.azure.com/pod-network-instance"]

// 			k8sClient := MustCreateK8SClientFromKubeConfig(kubeconfig)
// 			kubeClient := clientgen.Default(kubeconfig)

// 			logger.LogKV("step", "create namespace in the cluster")
// 			clientgen.EnsureNamespaceExists(ctx, kubeClient, logger, namespace)
// 			logger.LogKV("namespace", namespace, "state", "created")

// 			network.CreatePodNetworkInstance(ctx, runCtx, kubeconfig, namespace, podNetworkInstance, podNetwork, numNodes)

// 			logger.LogKV("step", "create deployment in the cluster")
// 			testcase := BusyboxTestcase{
// 				Namespace:               namespace,
// 				BusyboxImage:            dockerimage.ImageBusybox.MustGetFromRunContext(runCtx),
// 				Basename:                "mtpod-to-mtpod",
// 				Replicas:                numNodes,
// 				PodAntiAffinityHostname: true,
// 				Labels:                  podLabels,
// 			}
// 			deployment := testcase.Deployment()
// 			k8sClient.MustCreateDeployment(namespace, deployment)

// 			logger.LogKV("step", "wait until the pods in the deployment are ready")
// 			k8sClient.MustWaitDeploymentReady(namespace, k8smetav1.ListOptions{}, numNodes, nil)

// 			deploymentPods := EnsureAllPodsAreRunning(ctx,
// 				kubeClient,
// 				logger,
// 				func(k clientgen.Interface) ([]k8scorev1.Pod, error) {
// 					result := k.Pods(namespace).List(ctx, k8smetav1.ListOptions{LabelSelector: fmt.Sprintf("app in (%s)", testcase.Basename)})
// 					err := result.Err()
// 					if err != nil {
// 						logger.Logf("failed to list pods on node: %s", err)
// 						return nil, err
// 					}

// 					podList := result.OrElseThrow()

// 					numPods := len(podList.Items)
// 					if numPods != numNodes {
// 						return nil, fmt.Errorf("waiting for %d/%d pods", numPods, numNodes)
// 					}

// 					return podList.Items, nil
// 				},
// 				&clientgen.CheckPodOption{
// 					CheckInterval: podExecRetryInterval,
// 					CheckTimeout:  podExecRetryTimeout,
// 				})

// 			logger.LogKV("step", "validate swiftv2 pods datapath")
// 			ipsToPing := make([]string, 0, numNodes)
// 			for _, pod := range deploymentPods {
// 				mtpnc := network.GetMultitenantPodNetworkConfig(ctx, runCtx, kubeconfig, pod.Namespace, pod.Name)
// 				m.Expect(pod.Status.PodIPs).To(m.HaveLen(1))
// 				// remove /32 from PrimaryIP
// 				splitcidr := strings.Split(mtpnc.Status.PrimaryIP, "/")
// 				m.Expect(splitcidr).To(m.HaveLen(2))
// 				ipsToPing = append(ipsToPing, splitcidr[0])
// 			}

// 			for _, pod := range deploymentPods {
// 				for _, ip := range ipsToPing {
// 					logger.Logf("ping from pod %q to %q", pod.Name, ip)
// 					result, err := retry.DoFixedRetryWithMaxCount(
// 						func() retry.Result {
// 							stdout, stderr, err := clientgen.PodExecWithError(
// 								logger,
// 								kubeconfig,
// 								pod.Name,
// 								pod.Namespace,
// 								[]string{"ping", "-c", "3", ip},
// 							)
// 							if err != nil {
// 								logger.Logf("ping %q failed: error: %s, stdout: %s, stderr: %s", ip, err, stdout, stderr)
// 								return retry.Result{
// 									Status: retry.NeedRetry,
// 									Body:   "ping failed",
// 								}
// 							}

// 							return retry.Result{
// 								Status: retry.Success,
// 								Body:   stdout,
// 							}
// 						},
// 						podExecRetryInterval,
// 						podExecRetryTimeout,
// 						podExecRetryMaxAttempts)

// 					m.Expect(err).NotTo(m.HaveOccurred(), "err: %s", err)
// 					m.Expect(result.Status).To(m.Equal(retry.Success))
// 				}
// 			}

// 			// Cleanup by deleting the namespace.
// 			logger.LogKV("step", "delete namespace in the cluster")
// 			clientgen.EnsureNamespaceDeleted(ctx, kubeClient, logger, namespace)
// 			logger.Logf("deleted namespace %q", namespace)
// 		},
// 	}
// }

func TestDatapathLinux(t *testing.T) {
	ctx := context.Background()

	t.Log("Get REST config")
	restConfig := kubernetes.MustGetRestConfig()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()

	setupLinuxEnvironment(t)
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)

	t.Run("Linux ping tests", func(t *testing.T) {
		// Check goldpinger health
		t.Run("all pods have IPs assigned", func(t *testing.T) {
			err := kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
			if err != nil {
				t.Fatalf("Pods are not in running state due to %+v", err)
			}
			t.Log("all pods have been allocated IPs")
		})

		t.Run("all linux pods can ping each other", func(t *testing.T) {
			clusterCheckCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()

			pfOpts := k8s.PortForwardingOpts{
				Namespace:     *podNamespace,
				LabelSelector: podLabelSelector,
				LocalPort:     9090,
				DestPort:      8080,
			}

			pf, err := k8s.NewPortForwarder(restConfig, t, pfOpts)
			if err != nil {
				t.Fatal(err)
			}

			portForwardCtx, cancel := context.WithTimeout(ctx, defaultTimeoutSeconds*time.Second)
			defer cancel()

			portForwardFn := func() error {
				err := pf.Forward(portForwardCtx)
				if err != nil {
					t.Logf("unable to start port forward: %v", err)
					return err
				}
				return nil
			}

			if err := defaultRetrier.Do(portForwardCtx, portForwardFn); err != nil {
				t.Fatalf("could not start port forward within %d: %v", defaultTimeoutSeconds, err)
			}
			defer pf.Stop()

			gpClient := goldpinger.Client{Host: pf.Address()}
			clusterCheckFn := func() error {
				clusterState, err := gpClient.CheckAll(clusterCheckCtx)
				if err != nil {
					return err
				}
				stats := goldpinger.ClusterStats(clusterState)
				stats.PrintStats()
				if stats.AllPingsHealthy() {
					return nil
				}

				return errors.New("not all pings are healthy")
			}
			retrier := retry.Retrier{Attempts: goldpingerRetryCount, Delay: goldpingerDelayTimeSeconds * time.Second}
			if err := retrier.Do(clusterCheckCtx, clusterCheckFn); err != nil {
				t.Fatalf("goldpinger pods network health could not reach healthy state after %d seconds: %v", goldpingerRetryCount*goldpingerDelayTimeSeconds, err)
			}

			t.Log("all pings successful!")
		})
	})
}
