package kubecontroller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// APIDirectClient implements DirectAPIClient interface
var _ DirectAPIClient = &APIDirectClient{}

// APIDirectClient is a direct client to a kubernetes API server
type APIDirectClient struct {
	clientset *kubernetes.Clientset
}

// ListPods lists all pods in the given namespace and node
func (apiClient *APIDirectClient) ListPods(ctx context.Context, namespace, node string) (*corev1.PodList, error) {
	var (
		pods *corev1.PodList
		err  error
	)

	pods, err = apiClient.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node,
	})

	if err != nil {
		return nil, err
	}

	return pods, nil
}

// NewAPIDirectClient creates a new APIDirectClient
func NewAPIDirectClient(kubeconfig *rest.Config) (*APIDirectClient, error) {
	var (
		clientset *kubernetes.Clientset
		err       error
	)

	if clientset, err = kubernetes.NewForConfig(kubeconfig); err != nil {
		return nil, err
	}

	return &APIDirectClient{
		clientset: clientset,
	}, nil
}
