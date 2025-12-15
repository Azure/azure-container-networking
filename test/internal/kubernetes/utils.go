package kubernetes

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
	k8sRetry "k8s.io/client-go/util/retry"
)

const (
	DelegatedSubnetIDLabel = "kubernetes.azure.com/podnetwork-delegationguid"
	SubnetNameLabel        = "kubernetes.azure.com/podnetwork-subnet"

	// RetryAttempts is the number of times to retry a test.
	RetryAttempts           = 90
	RetryDelay              = 10 * time.Second
	DeleteRetryAttempts     = 12
	DeleteRetryDelay        = 5 * time.Second
	ShortRetryAttempts      = 8
	ShortRetryDelay         = 250 * time.Millisecond
	PrivilegedDaemonSetPath = "../manifests/load/privileged-daemonset-windows.yaml"
	PrivilegedLabelSelector = "app=privileged-daemonset"
	PrivilegedNamespace     = "kube-system"
)

var Kubeconfig = flag.String("test-kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "(optional) absolute path to the kubeconfig file")

func GetKubeconfig() *string {
	return Kubeconfig
}

func MustGetClientset() *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", *Kubeconfig)
	if err != nil {
		panic(errors.Wrap(err, "failed to build config from flags"))
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(errors.Wrap(err, "failed to get clientset"))
	}
	return clientset
}

func MustGetRestConfig() *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", *Kubeconfig)
	if err != nil {
		panic(err)
	}
	return config
}

func GetRESTClientForMultitenantCRDFromConfig(config *rest.Config) (*rest.RESTClient, error) {
	schemeLocal := runtime.NewScheme()
	err := v1alpha1.AddToScheme(schemeLocal)
	if err != nil {
		return nil, errors.Wrap(err, "failed to AddToScheme")
	}
	config.ContentConfig.GroupVersion = &v1alpha1.GroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(schemeLocal)
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	client, err := rest.UnversionedRESTClientFor(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to UnversionedRESTClientFor config")
	}
	return client, nil
}

func GetRESTClientForMultitenantCRD(kubeconfig string) (*rest.RESTClient, error) {
	schemeLocal := runtime.NewScheme()
	err := v1alpha1.AddToScheme(schemeLocal)
	if err != nil {
		return nil, errors.Wrap(err, "failed to AddToScheme")
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get RESTConfigFromKubeConfig")
	}

	restConfig.ContentConfig.GroupVersion = &v1alpha1.GroupVersion
	restConfig.APIPath = "/apis"
	restConfig.NegotiatedSerializer = serializer.NewCodecFactory(schemeLocal)
	restConfig.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.UnversionedRESTClientFor(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to UnversionedRESTClientFor config")
	}
	return client, nil
}

func mustParseResource(path string, out interface{}) {
	f, err := os.Open(path)
	if err != nil {
		panic(errors.Wrap(err, "failed to open path"))
	}
	defer func() { _ = f.Close() }()

	if err := yaml.NewYAMLOrJSONDecoder(f, 0).Decode(out); err != nil {
		panic(errors.Wrap(err, "failed to decode"))
	}
}

func MustLabelSwiftNodes(ctx context.Context, clientset *kubernetes.Clientset, delegatedSubnetID, delegatedSubnetName string) {
	swiftNodeLabels := map[string]string{
		DelegatedSubnetIDLabel: delegatedSubnetID,
		SubnetNameLabel:        delegatedSubnetName,
	}

	res, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(errors.Wrap(err, "could not list nodes"))
	}
	for index := range res.Items {
		node := res.Items[index]
		_, err := AddNodeLabels(ctx, clientset.CoreV1().Nodes(), node.Name, swiftNodeLabels)
		if err != nil {
			panic(errors.Wrap(err, "could not add labels to node"))
		}
	}
}

func MustSetUpClusterRBAC(ctx context.Context, clientset *kubernetes.Clientset, clusterRolePath, clusterRoleBindingPath, serviceAccountPath string) func() {
	var (
		clusterRole        v1.ClusterRole
		clusterRoleBinding v1.ClusterRoleBinding
		serviceAccount     corev1.ServiceAccount
	)

	clusterRole = mustParseClusterRole(clusterRolePath)
	clusterRoleBinding = mustParseClusterRoleBinding(clusterRoleBindingPath)
	serviceAccount = mustParseServiceAccount(serviceAccountPath)

	clusterRoles := clientset.RbacV1().ClusterRoles()
	clusterRoleBindings := clientset.RbacV1().ClusterRoleBindings()
	serviceAccounts := clientset.CoreV1().ServiceAccounts(serviceAccount.Namespace)

	cleanupFunc := func() {
		log.Printf("cleaning up rbac")

		if err := serviceAccounts.Delete(ctx, serviceAccount.Name, metav1.DeleteOptions{}); err != nil {
			log.Print(err)
		}
		if err := clusterRoleBindings.Delete(ctx, clusterRoleBinding.Name, metav1.DeleteOptions{}); err != nil {
			log.Print(err)
		}
		if err := clusterRoles.Delete(ctx, clusterRole.Name, metav1.DeleteOptions{}); err != nil {
			log.Print(err)
		}

		log.Print("rbac cleaned up")
	}

	mustCreateServiceAccount(ctx, serviceAccounts, serviceAccount)
	mustCreateClusterRole(ctx, clusterRoles, clusterRole)
	mustCreateClusterRoleBinding(ctx, clusterRoleBindings, clusterRoleBinding)

	return cleanupFunc
}

func MustSetUpRBAC(ctx context.Context, clientset *kubernetes.Clientset, rolePath, roleBindingPath string) {
	var (
		role        v1.Role
		roleBinding v1.RoleBinding
	)

	role = mustParseRole(rolePath)
	roleBinding = mustParseRoleBinding(roleBindingPath)

	roles := clientset.RbacV1().Roles(role.Namespace)
	roleBindings := clientset.RbacV1().RoleBindings(roleBinding.Namespace)

	mustCreateRole(ctx, roles, role)
	mustCreateRoleBinding(ctx, roleBindings, roleBinding)
}

func MustSetupConfigMap(ctx context.Context, clientset *kubernetes.Clientset, configMapPath string) (corev1.ConfigMap, func()) { // nolint
	cm := mustParseConfigMap(configMapPath)
	configmaps := clientset.CoreV1().ConfigMaps(cm.Namespace)
	mustCreateConfigMap(ctx, configmaps, cm)
	return cm, func() {
		MustDeleteConfigMap(ctx, configmaps, cm)
	}
}

// MustSetupDaemonset is a convenience function to directly apply the daemonset at dsPath to the cluster,
// returning the parsed daemonset struct and a cleanup function in the process
func MustSetupDaemonset(ctx context.Context, clientset *kubernetes.Clientset, dsPath string) (appsv1.DaemonSet, func()) { // nolint
	ds := MustParseDaemonSet(dsPath)
	dsClient := clientset.AppsV1().DaemonSets(ds.Namespace)
	MustCreateDaemonset(ctx, dsClient, ds)
	return ds, func() {
		MustDeleteDaemonset(ctx, dsClient, ds)
	}
}

func MustSetupDeployment(ctx context.Context, clientset *kubernetes.Clientset, depPath string) (appsv1.Deployment, func()) { // nolint
	dep := MustParseDeployment(depPath)
	depClient := clientset.AppsV1().Deployments(dep.Namespace)
	MustCreateDeployment(ctx, depClient, dep)
	return dep, func() {
		MustDeleteDeployment(ctx, depClient, dep)
	}
}

func MustSetupServiceAccount(ctx context.Context, clientset *kubernetes.Clientset, serviceAccountPath string) (corev1.ServiceAccount, func()) { // nolint
	sa := mustParseServiceAccount(serviceAccountPath)
	saClient := clientset.CoreV1().ServiceAccounts(sa.Namespace)
	mustCreateServiceAccount(ctx, saClient, sa)
	return sa, func() {
		MustDeleteServiceAccount(ctx, saClient, sa)
	}
}

func MustSetupService(ctx context.Context, clientset *kubernetes.Clientset, servicePath string) (corev1.Service, func()) { // nolint
	svc := mustParseService(servicePath)
	svcClient := clientset.CoreV1().Services(svc.Namespace)
	mustCreateService(ctx, svcClient, svc)
	return svc, func() {
		MustDeleteService(ctx, svcClient, svc)
	}
}

func MustSetupLRP(ctx context.Context, clientset *cilium.Clientset, lrpPath string) (ciliumv2.CiliumLocalRedirectPolicy, func()) { // nolint
	lrp := mustParseLRP(lrpPath)
	lrpClient := clientset.CiliumV2().CiliumLocalRedirectPolicies(lrp.Namespace)
	mustCreateCiliumLocalRedirectPolicy(ctx, lrpClient, lrp)
	return lrp, func() {
		MustDeleteCiliumLocalRedirectPolicy(ctx, lrpClient, lrp)
	}
}

func MustSetupCNP(ctx context.Context, clientset *cilium.Clientset, cnpPath string) (ciliumv2.CiliumNetworkPolicy, func()) { // nolint
	cnp := mustParseCNP(cnpPath)
	cnpClient := clientset.CiliumV2().CiliumNetworkPolicies(cnp.Namespace)
	mustCreateCiliumNetworkPolicy(ctx, cnpClient, cnp)
	return cnp, func() {
		MustDeleteCiliumNetworkPolicy(ctx, cnpClient, cnp)
	}
}

func Int32ToPtr(i int32) *int32 { return &i }

func WaitForPodsRunning(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector string) error {
	podsClient := clientset.CoreV1().Pods(namespace)

	checkPodIPsFn := func() error {
		podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelselector})
		if err != nil {
			return errors.Wrapf(err, "could not list pods with label selector %s", labelselector)
		}

		if len(podList.Items) == 0 {
			return errors.New("no pods scheduled")
		}

		for index := range podList.Items {
			pod := podList.Items[index]
			if pod.Status.Phase == corev1.PodPending {
				return errors.New("some pods still pending")
			}
		}

		for index := range podList.Items {
			pod := podList.Items[index]
			if pod.Status.PodIP == "" {
				return errors.Wrapf(err, "Pod %s/%s has not been allocated an IP yet with reason %s", pod.Namespace, pod.Name, pod.Status.Message)
			}
		}

		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrap(retrier.Do(ctx, checkPodIPsFn), "failed to check if pods were running")
}

func WaitForPodsDelete(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector string) error {
	podsClient := clientset.CoreV1().Pods(namespace)

	checkPodsDeleted := func() error {
		podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelselector})
		if err != nil {
			return errors.Wrapf(err, "could not list pods with label selector %s", labelselector)
		}
		if len(podList.Items) != 0 {
			return errors.Errorf("%d pods still present", len(podList.Items))
		}
		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrap(retrier.Do(ctx, checkPodsDeleted), "failed to wait for pods to delete")
}

func WaitForPodDeployment(ctx context.Context, clientset *kubernetes.Clientset, namespace, deploymentName, podLabelSelector string, replicas int) error {
	podsClient := clientset.CoreV1().Pods(namespace)
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	checkPodDeploymentFn := func() error {
		deployment, err := deploymentsClient.Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "could not get deployment %s", deploymentName)
		}

		if deployment.Status.AvailableReplicas != int32(replicas) {
			// Provide real-time deployment availability to console
			log.Printf("deployment %s has %d replicas in available status, expected %d", deploymentName, deployment.Status.AvailableReplicas, replicas)
			return errors.New("deployment does not have the expected number of available replicas")
		}

		podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: podLabelSelector})
		if err != nil {
			return errors.Wrapf(err, "could not list pods with label selector %s", podLabelSelector)
		}

		log.Printf("deployment %s has %d pods, expected %d", deploymentName, len(podList.Items), replicas)
		if len(podList.Items) != replicas {
			return errors.New("some pods of the deployment are still not ready")
		}
		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrapf(retrier.Do(ctx, checkPodDeploymentFn), "could not wait for deployment %s", deploymentName)
}

func WaitForDeploymentToDelete(ctx context.Context, deploymentsClient typedappsv1.DeploymentInterface, d appsv1.Deployment) error {
	assertDeploymentNotFound := func() error {
		_, err := deploymentsClient.Get(ctx, d.Name, metav1.GetOptions{})
		// only if the error is "isNotFound", do we say, the deployment is deleted
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Errorf("expected isNotFound error when getting deployment, but got %v", err)
	}
	retrier := retry.Retrier{Attempts: DeleteRetryAttempts, Delay: DeleteRetryDelay}
	return errors.Wrapf(retrier.Do(ctx, assertDeploymentNotFound), "could not assert deployment %s isNotFound", d.Name)
}

func WaitForLRPDelete(ctx context.Context, ciliumClientset *cilium.Clientset, lrp ciliumv2.CiliumLocalRedirectPolicy) error {
	lrpClient := ciliumClientset.CiliumV2().CiliumLocalRedirectPolicies(lrp.Namespace)

	checkLRPDeleted := func() error {
		_, err := lrpClient.Get(ctx, lrp.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return errors.Wrapf(err, "could not get LRP %s", lrp.Name)
		}
		return errors.Errorf("LRP %s still present", lrp.Name)
	}

	retrier := retry.Retrier{Attempts: DeleteRetryAttempts, Delay: DeleteRetryDelay}
	return errors.Wrap(retrier.Do(ctx, checkLRPDeleted), "failed to wait for LRP to delete")
}

func WaitForPodDaemonset(ctx context.Context, clientset *kubernetes.Clientset, namespace, daemonsetName, podLabelSelector string) error {
	podsClient := clientset.CoreV1().Pods(namespace)
	daemonsetClient := clientset.AppsV1().DaemonSets(namespace)
	checkPodDaemonsetFn := func() error {
		daemonset, err := daemonsetClient.Get(ctx, daemonsetName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "could not get daemonset %s", daemonsetName)
		}
		if daemonset.Status.UpdatedNumberScheduled != daemonset.Status.DesiredNumberScheduled {
			log.Printf("daemonset %s is updating, %v of %v pods updated", daemonsetName, daemonset.Status.UpdatedNumberScheduled, daemonset.Status.DesiredNumberScheduled)
			return errors.New("daemonset failed to update all pods")
		}

		if daemonset.Status.NumberReady == 0 && daemonset.Status.DesiredNumberScheduled == 0 {
			// Capture daemonset restart. Restart sets every numerical status to 0.
			log.Printf("daemonset %s is fresh, no pods should be ready or scheduled", daemonsetName)
			return errors.New("daemonset did not set any pods to be scheduled")
		}

		if daemonset.Status.NumberReady != daemonset.Status.DesiredNumberScheduled {
			// Provide real-time daemonset availability to console
			log.Printf("daemonset %s has %d pods in ready status, expected %d", daemonsetName, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)
			return errors.New("daemonset does not have the expected number of ready state pods")
		}

		podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: podLabelSelector})
		if err != nil {
			return errors.Wrapf(err, "could not list pods with label selector %s", podLabelSelector)
		}

		log.Printf("daemonset %s has %d pods in ready status | %d pods up-to-date status, expected %d",
			daemonsetName, len(podList.Items), daemonset.Status.UpdatedNumberScheduled, daemonset.Status.CurrentNumberScheduled)
		if len(podList.Items) != int(daemonset.Status.NumberReady) {
			return errors.New("some pods of the daemonset are still not ready")
		}
		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrapf(retrier.Do(ctx, checkPodDaemonsetFn), "could not wait for daemonset %s", daemonsetName)
}

func MustUpdateReplica(ctx context.Context, deploymentsClient typedappsv1.DeploymentInterface, deploymentName string, replicas int32) {
	retryErr := k8sRetry.RetryOnConflict(k8sRetry.DefaultRetry, func() error {
		// Get the latest Deployment resource.
		deployment, getErr := deploymentsClient.Get(ctx, deploymentName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get deployment: %w", getErr)
		}

		// Modify the number of replicas.
		deployment.Spec.Replicas = Int32ToPtr(replicas)

		// Attempt to update the Deployment.
		_, updateErr := deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
		if updateErr != nil {
			return fmt.Errorf("failed to update deployment: %w", updateErr)
		}

		return nil // No error, operation succeeded.
	})

	if retryErr != nil {
		panic(errors.Wrapf(retryErr, "could not update deployment %s", deploymentName))
	}
}

func ExportLogsByLabelSelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector, logDir string) error {
	podsClient := clientset.CoreV1().Pods(namespace)
	podLogOpts := corev1.PodLogOptions{}
	logExtension := ".log"
	podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelselector})
	if err != nil {
		return errors.Wrap(err, "failed to list pods")
	}

	for index := range podList.Items {
		pod := podList.Items[index]
		req := podsClient.GetLogs(pod.Name, &podLogOpts)
		podLogs, err := req.Stream(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get pod logs as stream")
		}

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		podLogs.Close()
		if err != nil {
			return errors.Wrap(err, "failed to copy pod logs")
		}
		str := buf.String()
		err = writeToFile(logDir, pod.Name+logExtension, str)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeToFile(dir, fileName, str string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// your dir does not exist
		if err := os.MkdirAll(dir, 0o666); err != nil { //nolint
			return errors.Wrap(err, "failed to make directory")
		}
	}
	// open output file
	f, err := os.Create(dir + fileName)
	if err != nil {
		return errors.Wrap(err, "failed to create output file")
	}
	// close fo on exit and check for its returned error
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	// If write went ok then err is nil
	_, err = f.WriteString(str)
	return errors.Wrap(err, "failed to write string")
}

// ExecCmdOnPod runs the specified command on a particular pod and retries the command on failure if doRetry is set to true
// The function returns the standard output, standard error, and error (if any) in that order
func ExecCmdOnPod(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName, containerName string, cmd []string, config *rest.Config, doRetry bool) ([]byte, []byte, error) { // nolint
	var result []byte
	var errResult []byte
	execCmdOnPod := func() error {
		req := clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(namespace).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Command:   cmd,
				Container: containerName,
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
		if err != nil {
			return errors.Wrapf(err, "error in creating executor for req %s", req.URL())
		}

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: &stdout,
			Stderr: &stderr,
			Tty:    false,
		})

		result = stdout.Bytes()
		errResult = stderr.Bytes()

		if err != nil {
			log.Printf("Error: %v had error %v from command - %v", podName, err, cmd)
			return errors.Wrapf(err, "error in executing command %s", cmd)
		}
		if len(stdout.Bytes()) == 0 {
			log.Printf("Warning: %v had 0 bytes returned from command - %v", podName, cmd)
		}
		return nil
	}

	var err error
	if doRetry {
		retrier := retry.Retrier{Attempts: ShortRetryAttempts, Delay: RetryDelay}
		err = retrier.Do(ctx, execCmdOnPod)
	} else {
		err = execCmdOnPod()
	}
	return result, errResult, errors.Wrapf(err, "could not execute the cmd %s on %s", cmd, podName)
}

func NamespaceExists(ctx context.Context, clientset *kubernetes.Clientset, namespace string) (bool, error) {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "error in getting namespace %s", namespace)
	}
	return true, nil
}

func DeploymentExists(ctx context.Context, deploymentsClient typedappsv1.DeploymentInterface, deploymentName string) (bool, error) {
	_, err := deploymentsClient.Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "error in getting deployment %s", deploymentName)
	}

	return true, nil
}

// return a label selector
func CreateLabelSelector(key string, selector *string) string {
	return fmt.Sprintf("%s=%s", key, *selector)
}

func HasWindowsNodes(ctx context.Context, clientset *kubernetes.Clientset) (bool, error) {
	nodes, err := GetNodeList(ctx, clientset)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		node := nodes.Items[index]
		if node.Status.NodeInfo.OperatingSystem == string(corev1.Windows) {
			return true, nil
		}
	}
	return false, nil
}

func MustRestartDaemonset(ctx context.Context, clientset *kubernetes.Clientset, namespace, daemonsetName string) error {
	ds, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, daemonsetName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get daemonset %s", daemonsetName)
	}

	if ds.Spec.Template.ObjectMeta.Annotations == nil {
		ds.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	// gen represents the generation before triggering a restart
	gen := ds.Status.ObservedGeneration
	log.Printf("Current generation is %v", gen)
	ds.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = clientset.AppsV1().DaemonSets(namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update ds %s", daemonsetName)
	}
	checkDaemonsetGenerationFn := func() error {
		ds, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, daemonsetName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "could not get daemonset %s", daemonsetName)
		}

		if ds.Status.ObservedGeneration < gen {
			// Generation update should not reset or lower the ObservedGeneration. Only happens if a complete restart or teardown of the daemonset occurs.
			log.Printf("Warning: daemonset %s current generation (%d) is less than starting generation (%d)", daemonsetName, gen, ds.Status.ObservedGeneration)
			return errors.New("daemonset generation was less than original")
		}

		if ds.Status.ObservedGeneration == gen {
			// Check for generation update.
			log.Printf("daemonset %s has not updated generation", daemonsetName)
			return errors.New("daemonset generation did not change")
		}

		log.Printf("daemonset %s has updated generation", daemonsetName)
		return nil
	}
	retrier := retry.Retrier{Attempts: ShortRetryAttempts, Delay: ShortRetryDelay}
	return errors.Wrapf(retrier.Do(ctx, checkDaemonsetGenerationFn), "could not wait for ds %s generation update", daemonsetName)
}

// Restarts kubeproxy on windows nodes from an existing privileged daemonset
func RestartKubeProxyService(ctx context.Context, clientset *kubernetes.Clientset, privilegedNamespace, privilegedLabelSelector string, config *rest.Config) error {
	restartKubeProxyCmd := []string{"powershell", "Restart-service", "kubeproxy"}

	nodes, err := GetNodeList(ctx, clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	for index := range nodes.Items {
		node := nodes.Items[index]
		if node.Status.NodeInfo.OperatingSystem != string(corev1.Windows) {
			continue
		}
		// get the privileged pod
		pod, err := GetPodsByNode(ctx, clientset, privilegedNamespace, privilegedLabelSelector, node.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get privileged pod on node %s", node.Name)
		}

		if len(pod.Items) == 0 {
			return errors.Errorf("there are no privileged pods on node - %v", node.Name)
		}
		privilegedPod := pod.Items[0]
		// exec into the pod and restart kubeproxy
		_, _, err = ExecCmdOnPod(ctx, clientset, privilegedNamespace, privilegedPod.Name, "", restartKubeProxyCmd, config, true)
		if err != nil {
			return errors.Wrapf(err, "failed to exec into privileged pod %s on node %s", privilegedPod.Name, node.Name)
		}
	}
	return nil
}
