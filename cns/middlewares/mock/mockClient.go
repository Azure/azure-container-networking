package middlewares

import (
	"context"
	"errors"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errPodNotFound   = errors.New("pod not found")
	errMTPNCNotFound = errors.New("mtpnc not found")
)

// MockClient implements the client.Client interface for testing. We only care about Get, the rest is nil ops.
type MockClient struct {
	client.Client
	mtPodCache map[string]*v1.Pod
	mtpncCache map[string]*v1alpha1.MultitenantPodNetworkConfig
}

// NewMockClient returns a new MockClient.
func NewMockClient() *MockClient {
	testPod1 := v1.Pod{}
	testPod1.Labels = make(map[string]string)
	testPod1.Labels[configuration.LabelPodSwiftV2] = "true"

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	testMTPNC1.Status.PrimaryIP = "192.168.0.1/32"
	testMTPNC1.Status.MacAddress = "00:00:00:00:00:00"
	testMTPNC1.Status.GatewayIP = "10.0.0.1"
	testMTPNC1.Status.NCID = "testncid"

	testMTPNC3 := v1alpha1.MultitenantPodNetworkConfig{}

	return &MockClient{
		mtPodCache: map[string]*v1.Pod{"testpod1namespace/testpod1": &testPod1},
		mtpncCache: map[string]*v1alpha1.MultitenantPodNetworkConfig{
			"testpod1namespace/testpod1": &testMTPNC1,
			"testpod3namespace/testpod3": &testMTPNC3,
		},
	}
}

// Get implements client.Client.Get.
func (c *MockClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	switch o := obj.(type) {
	case *v1.Pod:
		if pod, ok := c.mtPodCache[key.String()]; ok {
			*o = *pod
		} else {
			return errPodNotFound
		}
	case *v1alpha1.MultitenantPodNetworkConfig:
		if mtpnc, ok := c.mtpncCache[key.String()]; ok {
			*o = *mtpnc
		} else {
			return errMTPNCNotFound
		}
	}
	return nil
}
