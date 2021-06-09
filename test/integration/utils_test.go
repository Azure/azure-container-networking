// +build integration

package k8s

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	//crd "dnc/requestcontroller/kubernetes"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/test/integration/retry"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DelegatedSubnetIDLabel = "kubernetes.azure.com/podnetwork-delegationguid"
	SubnetNameLabel        = "kubernetes.azure.com/podnetwork-subnet"
)

func mustGetClientset() (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func mustGetRestConfig(t *testing.T) *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
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

func mustLabelSwiftNodes(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, delegatedSubnetID, delegatedSubnetName string) {
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

func mustSetUpClusterRBAC(ctx context.Context, clientset *kubernetes.Clientset, clusterRolePath, clusterRoleBindingPath, serviceAccountPath string) (func(), error) {
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

func mustSetUpRBAC(ctx context.Context, clientset *kubernetes.Clientset, rolePath, roleBindingPath string) error {
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

func mustSetupConfigMap(ctx context.Context, clientset *kubernetes.Clientset, configMapPath string) error {
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

func int32ptr(i int32) *int32 { return &i }

func parseImageString(s string) (image, version string) {
	sl := strings.Split(s, ":")
	return sl[0], sl[1]
}

func getImageString(image, version string) string {
	return image + ":" + version
}

func waitForPodsRunning(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector string) error {
	podsClient := clientset.CoreV1().Pods(namespace)

	checkPodIPsFn := func() error {
		podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelselector})
		if err != nil {
			return err
		}

		if len(podList.Items) == 0 {
			return errors.New("no pods scheduled")
		}

		for _, pod := range podList.Items {
			if pod.Status.Phase == apiv1.PodPending {
				return errors.New("some pods still pending")
			}
		}

		for _, pod := range podList.Items {
			if pod.Status.PodIP == "" {
				return errors.New("a pod has not been allocated an IP")
			}
		}

		return nil
	}

	retrier := retry.Retrier{Attempts: 10, Delay: 6 * time.Second}
	return retrier.Do(ctx, checkPodIPsFn)
}


func exportLogsByLabelSelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelselector, logDir string) error {
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
		err = writeToFile(logDir, pod.Name + logExtension, str)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeToFile(dir, fileName, str string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) { 
		// your dir does not exist
		os.MkdirAll(dir, 0666)
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
