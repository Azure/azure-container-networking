package kubecontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeClient is an interface that talks to the API server
type KubeClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

// DirectCRDClient is an interface to get CRDs directly, without cache
type DirectCRDClient interface {
	Get(ctx context.Context, name, namespace, typeName string) (*v1alpha.NodeNetworkConfig, error)
}

// DirectAPIClient is an interface to talk directly with API Server without cache
type DirectAPIClient interface {
	ListPods(ctx context.Context, namespace, node string) (*corev1.PodList, error)
}
