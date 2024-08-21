package mock

import (
	"context"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrPodNotFound   = errors.New("pod not found")
	ErrMTPNCNotFound = errors.New("mtpnc not found")
)

// Client implements the client.Client interface for testing. We only care about Get, the rest is nil ops.
type Client struct {
	client.Client
	mtPodCache map[string]*v1.Pod
	mtpncCache map[string]*v1alpha1.MultitenantPodNetworkConfig
}

// NewClient returns a new MockClient.
func NewClient() *Client {
	const podNetwork = "azure"

	testPod1 := v1.Pod{}
	testPod1.Labels = make(map[string]string)
	testPod1.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod2 := v1.Pod{}
	testPod2.Labels = make(map[string]string)
	testPod2.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod3 := v1.Pod{}
	testPod3.Labels = make(map[string]string)
	testPod3.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod4 := v1.Pod{}
	testPod4.Labels = make(map[string]string)
	testPod4.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			PrimaryIP:  "192.168.0.1/32",
			MacAddress: "00:00:00:00:00:00",
			GatewayIP:  "10.0.0.1",
			NCID:       "testncid",
		},
	}

	testMTPNC2 := v1alpha1.MultitenantPodNetworkConfig{}

	testMTPNC4 := v1alpha1.MultitenantPodNetworkConfig{}

	return &Client{
		mtPodCache: map[string]*v1.Pod{"testpod1namespace/testpod1": &testPod1, "testpod3namespace/testpod3": &testPod3, "testpod4namespace/testpod4": &testPod4},
		mtpncCache: map[string]*v1alpha1.MultitenantPodNetworkConfig{
			"testpod1namespace/testpod1": &testMTPNC1,
			"testpod2namespace/testpod2": &testMTPNC2,
			"testpod4namespace/testpod4": &testMTPNC4,
		},
	}
}

// Get implements client.Client.Get.
func (c *Client) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	switch o := obj.(type) {
	case *v1.Pod:
		if pod, ok := c.mtPodCache[key.String()]; ok {
			*o = *pod
		} else {
			return ErrPodNotFound
		}
	case *v1alpha1.MultitenantPodNetworkConfig:
		if mtpnc, ok := c.mtpncCache[key.String()]; ok {
			*o = *mtpnc
		} else {
			return ErrMTPNCNotFound
		}
	}
	return nil
}

func (c *Client) SetMTPNCReady() {
	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	testMTPNC1.Status.PrimaryIP = "192.168.0.1/32"
	testMTPNC1.Status.MacAddress = "00:00:00:00:00:00"
	testMTPNC1.Status.GatewayIP = "10.0.0.1"
	testMTPNC1.Status.NCID = "testncid"
	c.mtpncCache["testpod1namespace/testpod1"] = &testMTPNC1
}

func (c *Client) SetMTPNCNotReady() {
	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	c.mtpncCache["testpod1namespace/testpod1"] = &testMTPNC1
}
