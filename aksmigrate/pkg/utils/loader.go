package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/azure/aksmigrate/pkg/types"
)

var (
	scheme = runtime.NewScheme()
	codecs serializer.CodecFactory
)

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	codecs = serializer.NewCodecFactory(scheme)
}

// NewKubeClient creates a Kubernetes clientset and REST config from the given kubeconfig path.
// If kubeconfig is empty, it uses the default loading rules (KUBECONFIG env, ~/.kube/config).
func NewKubeClient(kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, config, nil
}

// LoadFromCluster fetches all relevant resources from a live Kubernetes cluster.
func LoadFromCluster(ctx context.Context, clientset *kubernetes.Clientset) (*types.ClusterResources, error) {
	resources := &types.ClusterResources{}

	// Fetch NetworkPolicies from all namespaces
	npList, err := clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies: %w", err)
	}
	resources.NetworkPolicies = npList.Items

	// Fetch Pods from all namespaces
	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	resources.Pods = podList.Items

	// Fetch Services from all namespaces
	svcList, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	resources.Services = svcList.Items

	// Fetch Nodes
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	resources.Nodes = nodeList.Items

	// Fetch Namespaces
	nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	resources.Namespaces = nsList.Items

	return resources, nil
}

// LoadFromDirectory reads Kubernetes YAML manifests from a directory.
// It scans for .yaml and .yml files and parses NetworkPolicy, Pod, Service, Node, and Namespace objects.
func LoadFromDirectory(dir string) (*types.ClusterResources, error) {
	resources := &types.ClusterResources{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Split multi-document YAML
		docs := splitYAMLDocuments(data)
		for _, doc := range docs {
			doc = []byte(strings.TrimSpace(string(doc)))
			if len(doc) == 0 {
				continue
			}
			parseObject(doc, resources)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return resources, nil
}

// splitYAMLDocuments splits a multi-document YAML byte slice on "---" separators.
func splitYAMLDocuments(data []byte) [][]byte {
	var docs [][]byte
	for _, doc := range strings.Split(string(data), "\n---") {
		trimmed := strings.TrimSpace(doc)
		if trimmed != "" {
			docs = append(docs, []byte(trimmed))
		}
	}
	return docs
}

// parseObject attempts to decode a YAML document into a known Kubernetes type
// and adds it to the appropriate slice in ClusterResources.
func parseObject(data []byte, resources *types.ClusterResources) {
	decoder := codecs.UniversalDeserializer()
	obj, gvk, err := decoder.Decode(data, nil, nil)
	if err != nil {
		// Not a recognized K8s object, skip
		return
	}

	switch gvk.Kind {
	case "NetworkPolicy":
		if np, ok := obj.(*networkingv1.NetworkPolicy); ok {
			resources.NetworkPolicies = append(resources.NetworkPolicies, *np)
		}
	case "Pod":
		if pod, ok := obj.(*corev1.Pod); ok {
			resources.Pods = append(resources.Pods, *pod)
		}
	case "Service":
		if svc, ok := obj.(*corev1.Service); ok {
			resources.Services = append(resources.Services, *svc)
		}
	case "Node":
		if node, ok := obj.(*corev1.Node); ok {
			resources.Nodes = append(resources.Nodes, *node)
		}
	case "Namespace":
		if ns, ok := obj.(*corev1.Namespace); ok {
			resources.Namespaces = append(resources.Namespaces, *ns)
		}
	}
}
