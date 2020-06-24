package kubernetes

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sClient is an interface used by nodeNetworkConfigReconciler and k8sRequestController
// They rely on this interface in order to be able to make unit tests that overload these methods
type K8sClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error
	Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error
}

//TODO: make a mock client for testing
