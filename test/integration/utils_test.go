// +build integration

package k8s

import (
	"context"
	//crd "dnc/requestcontroller/kubernetes"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	typedrbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DelegatedSubnetIDLabel = "kubernetes.azure.com/podnetwork-delegationguid"
	SubnetNameLabel        = "kubernetes.azure.com/podnetwork-subnet"
)

func mustGetClientset(t *testing.T) *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return clientset
}

func mustGetRestConfig(t *testing.T) *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func mustParseResource(t *testing.T, path string, out interface{}) {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if err := yaml.NewYAMLOrJSONDecoder(f, 0).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func mustParseDeployment(t *testing.T, path string) appsv1.Deployment {
	var depl appsv1.Deployment
	mustParseResource(t, path, &depl)
	return depl
}

func mustParseServiceAccount(t *testing.T, path string) corev1.ServiceAccount {
	var svcAcct corev1.ServiceAccount
	mustParseResource(t, path, &svcAcct)
	return svcAcct
}

func mustParseClusterRole(t *testing.T, path string) rbacv1.ClusterRole {
	var cr rbacv1.ClusterRole
	mustParseResource(t, path, &cr)
	return cr
}

func mustParseClusterRoleBinding(t *testing.T, path string) rbacv1.ClusterRoleBinding {
	var crb rbacv1.ClusterRoleBinding
	mustParseResource(t, path, &crb)
	return crb
}

func mustCreateDeployment(t *testing.T, ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) {
	if err := deployments.Delete(ctx, d.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}

	if _, err := deployments.Create(ctx, &d, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func mustCreateServiceAccount(t *testing.T, ctx context.Context, svcAccounts typedcorev1.ServiceAccountInterface, s corev1.ServiceAccount) {
	if err := svcAccounts.Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}
	if _, err := svcAccounts.Create(ctx, &s, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func mustCreateClusterRole(t *testing.T, ctx context.Context, clusterRoles typedrbacv1.ClusterRoleInterface, cr rbacv1.ClusterRole) {
	if err := clusterRoles.Delete(ctx, cr.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}
	if _, err := clusterRoles.Create(ctx, &cr, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func mustCreateClusterRoleBinding(t *testing.T, ctx context.Context, crBindings typedrbacv1.ClusterRoleBindingInterface, crb rbacv1.ClusterRoleBinding) {
	if err := crBindings.Delete(ctx, crb.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
	}
	if _, err := crBindings.Create(ctx, &crb, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
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

func mustSetUpRBAC(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) (cleanUpFunc func(t *testing.T)) {
	clusterRole := mustParseClusterRole(t, "testdata/goldpinger/cluster-role.yaml")
	clusterRoleBinding := mustParseClusterRoleBinding(t, "testdata/goldpinger/cluster-role-binding.yaml")
	serviceAccount := mustParseServiceAccount(t, "testdata/goldpinger/service-account.yaml")

	clusterRoles := clientset.RbacV1().ClusterRoles()
	clusterRoleBindings := clientset.RbacV1().ClusterRoleBindings()
	serviceAccounts := clientset.CoreV1().ServiceAccounts(serviceAccount.Namespace)

	mustCreateServiceAccount(t, ctx, serviceAccounts, serviceAccount)
	mustCreateClusterRole(t, ctx, clusterRoles, clusterRole)
	mustCreateClusterRoleBinding(t, ctx, clusterRoleBindings, clusterRoleBinding)

	t.Log("rbac set up")

	return func(t *testing.T) {
		t.Log("cleaning up rbac")

		if err := serviceAccounts.Delete(ctx, serviceAccount.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}
		if err := clusterRoleBindings.Delete(ctx, clusterRoleBinding.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}
		if err := clusterRoles.Delete(ctx, clusterRole.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}

		t.Log("rbac cleaned up")
	}
}

func int32ptr(i int32) *int32 { return &i }
