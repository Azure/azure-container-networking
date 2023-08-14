package k8sutils

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	// crd "dnc/requestcontroller/kubernetes"

	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

const (
	DelegatedSubnetIDLabel = "kubernetes.azure.com/podnetwork-delegationguid"
	SubnetNameLabel        = "kubernetes.azure.com/podnetwork-subnet"

	// RetryAttempts is the number of times to retry a test.
	RetryAttempts = 90
	RetryDelay    = 10 * time.Second
)

var Kubeconfig = flag.String("test-kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "(optional) absolute path to the kubeconfig file")

func MustGetClientset() (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *Kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func MustGetRestConfig(t *testing.T) *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", *Kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func mustParseResource(path string, out interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if err := yaml.NewYAMLOrJSONDecoder(f, 0).Decode(out); err != nil {
		return err
	}
	return err
}

func MustLabelSwiftNodes(ctx context.Context, t *testing.T, clientset *kubernetes.Clientset, delegatedSubnetID, delegatedSubnetName string) {
	swiftNodeLabels := map[string]string{
		DelegatedSubnetIDLabel: delegatedSubnetID,
		SubnetNameLabel:        delegatedSubnetName,
	}

	res, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("could not list nodes: %v", err)
	}
	for _, node := range res.Items {
		_, err := AddNodeLabels(ctx, clientset.CoreV1().Nodes(), node.Name, swiftNodeLabels)
		if err != nil {
			t.Fatalf("could not add labels to node: %v", err)
		}
		t.Logf("labels added to node %s", node.Name)
	}
}

func MustSetUpClusterRBAC(ctx context.Context, clientset *kubernetes.Clientset, clusterRolePath, clusterRoleBindingPath, serviceAccountPath string) (func(), error) {
	var (
		err                error
		clusterRole        v1.ClusterRole
		clusterRoleBinding v1.ClusterRoleBinding
		serviceAccount     corev1.ServiceAccount
	)

	if clusterRole, err = mustParseClusterRole(clusterRolePath); err != nil {
		return nil, err
	}

	if clusterRoleBinding, err = mustParseClusterRoleBinding(clusterRoleBindingPath); err != nil {
		return nil, err
	}

	if serviceAccount, err = mustParseServiceAccount(serviceAccountPath); err != nil {
		return nil, err
	}

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

	if err = mustCreateServiceAccount(ctx, serviceAccounts, serviceAccount); err != nil {
		return cleanupFunc, err
	}

	if err = mustCreateClusterRole(ctx, clusterRoles, clusterRole); err != nil {
		return cleanupFunc, err
	}

	if err = mustCreateClusterRoleBinding(ctx, clusterRoleBindings, clusterRoleBinding); err != nil {
		return cleanupFunc, err
	}

	return cleanupFunc, nil
}

func MustSetUpRBAC(ctx context.Context, clientset *kubernetes.Clientset, rolePath, roleBindingPath string) error {
	var (
		err         error
		role        v1.Role
		roleBinding v1.RoleBinding
	)

	if role, err = mustParseRole(rolePath); err != nil {
		return err
	}

	if roleBinding, err = mustParseRoleBinding(roleBindingPath); err != nil {
		return err
	}

	roles := clientset.RbacV1().Roles(role.Namespace)
	roleBindings := clientset.RbacV1().RoleBindings(roleBinding.Namespace)

	if err = mustCreateRole(ctx, roles, role); err != nil {
		return err
	}

	if err = mustCreateRoleBinding(ctx, roleBindings, roleBinding); err != nil {
		return err
	}

	return nil
}

func MustSetupConfigMap(ctx context.Context, clientset *kubernetes.Clientset, configMapPath string) error {
	var (
		err error
		cm  corev1.ConfigMap
	)

	if cm, err = mustParseConfigMap(configMapPath); err != nil {
		return err
	}

	configmaps := clientset.CoreV1().ConfigMaps(cm.Namespace)

	return mustCreateConfigMap(ctx, configmaps, cm)
}

func Int32ToPtr(i int32) *int32 { return &i }

func ParseImageString(s string) (image, version string) {
	sl := strings.Split(s, ":")
	return sl[0], sl[1]
}

func GetImageString(image, version string) string {
	return image + ":" + version
}

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

		for _, pod := range podList.Items {
			if pod.Status.Phase == corev1.PodPending {
				return errors.New("some pods still pending")
			}
		}

		for _, pod := range podList.Items {
			if pod.Status.PodIP == "" {
				return errors.Wrapf(err, "Pod %s/%s has not been allocated an IP yet with reason %s", pod.Namespace, pod.Name, pod.Status.Message)
			}
		}

		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return retrier.Do(ctx, checkPodIPsFn)
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

func MustUpdateReplica(ctx context.Context, deploymentsClient typedappsv1.DeploymentInterface, deploymentName string, replicas int32) error {
	deployment, err := deploymentsClient.Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not get deployment %s", deploymentName)
	}

	deployment.Spec.Replicas = Int32ToPtr(replicas)
	_, err = deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
	return errors.Wrapf(err, "could not update deployment %s", deploymentName)
}

func ExportLogsByLabelSelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector, logDir string) error {
	podsClient := clientset.CoreV1().Pods(namespace)
	podLogOpts := corev1.PodLogOptions{}
	logExtension := ".log"
	podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelselector})
	if err != nil {
		return err
	}

	for _, pod := range podList.Items {
		req := podsClient.GetLogs(pod.Name, &podLogOpts)
		podLogs, err := req.Stream(ctx)
		if err != nil {
			return err
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return err
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
		os.MkdirAll(dir, 0o666)
	}
	// open output file
	f, err := os.Create(dir + fileName)
	if err != nil {
		return err
	}
	// close fo on exit and check for its returned error
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	// If write went ok then err is nil
	_, err = f.WriteString(str)
	return err
}

func ExecCmdOnPod(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string, cmd []string, config *rest.Config) ([]byte, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return []byte{}, errors.Wrapf(err, "error in creating executor for req %s", req.URL())
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return []byte{}, errors.Wrapf(err, "error in executing command %s", cmd)
	}

	return stdout.Bytes(), nil
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

// return a label selector
func CreateLabelSelector(key string, selector *string) string {
	return fmt.Sprintf("%s=%s", key, *selector)
}
