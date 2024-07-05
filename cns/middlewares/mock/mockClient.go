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

	testPod5 := v1.Pod{}
	testPod5.Labels = make(map[string]string)
	testPod5.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod6 := v1.Pod{}
	testPod6.Labels = make(map[string]string)
	testPod6.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod7 := v1.Pod{}
	testPod7.Labels = make(map[string]string)
	testPod7.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod8 := v1.Pod{}
	testPod8.Labels = make(map[string]string)
	testPod8.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod9 := v1.Pod{}
	testPod9.Labels = make(map[string]string)
	testPod9.Labels[configuration.LabelPodSwiftV2] = podNetwork

	testPod10 := v1.Pod{}
	testPod10.Labels = make(map[string]string)
	testPod10.Labels[configuration.LabelPodNetworkInstanceSwiftV2] = podNetwork

	testInterfaceInfos1 := v1alpha1.InterfaceInfo{
		NCID:            "testncid",
		PrimaryIP:       "192.168.0.1/32",
		MacAddress:      "00:00:00:00:00:00",
		GatewayIP:       "10.0.0.1",
		DeviceType:      v1alpha1.DeviceTypeVnetNIC,
		AccelnetEnabled: false,
	}
	testInterfaceInfos3 := v1alpha1.InterfaceInfo{
		NCID:            "testncid",
		PrimaryIP:       "192.168.0.1/32",
		MacAddress:      "00:00:00:00:00:00",
		GatewayIP:       "10.0.0.1",
		DeviceType:      v1alpha1.DeviceTypeVnetNIC,
		AccelnetEnabled: false,
	}
	testInterfaceInfos5 := v1alpha1.InterfaceInfo{
		NCID:            "testncid",
		PrimaryIP:       "192.168.0.1/32",
		MacAddress:      "00:00:00:00:00:00",
		GatewayIP:       "10.0.0.1",
		DeviceType:      v1alpha1.DeviceTypeInfiniBandNIC,
		AccelnetEnabled: true,
	}

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			InterfaceInfos: []v1alpha1.InterfaceInfo{testInterfaceInfos1},
		},
	}

	testMTPNC2 := v1alpha1.MultitenantPodNetworkConfig{}

	testMTPNC3 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			InterfaceInfos: []v1alpha1.InterfaceInfo{testInterfaceInfos3},
		},
	}

	testMTPNC4 := v1alpha1.MultitenantPodNetworkConfig{}

	testMTPNC5 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			InterfaceInfos: []v1alpha1.InterfaceInfo{testInterfaceInfos5},
		},
	}

	testMTPNCMulti := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			InterfaceInfos: []v1alpha1.InterfaceInfo{testInterfaceInfos1, testInterfaceInfos3, testInterfaceInfos5},
		},
	}

	testMTPNC8 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			PrimaryIP:  "192.168.0.1/32",
			MacAddress: "00:00:00:00:00:00",
			GatewayIP:  "10.0.0.1",
			NCID:       "testncid",
			InterfaceInfos: []v1alpha1.InterfaceInfo{
				{
					PrimaryIP:  "192.168.0.1/32",
					MacAddress: "00:00:00:00:00:00",
					GatewayIP:  "10.0.0.1",
					NCID:       "testncid",
					DeviceType: v1alpha1.DeviceTypeVnetNIC,
				},
				{
					PrimaryIP:  "192.168.0.1/32",
					MacAddress: "00:00:00:00:00:00",
					GatewayIP:  "10.0.0.1",
					NCID:       "testncid",
					DeviceType: v1alpha1.DeviceTypeInfiniBandNIC,
				},
			},
		},
	}
	// Mtpnc with just Infiniband interface
	testMTPNC9 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{
			InterfaceInfos: []v1alpha1.InterfaceInfo{
				{
					PrimaryIP:  "192.168.0.1/32",
					MacAddress: "00:00:00:00:00:00",
					GatewayIP:  "10.0.0.1",
					NCID:       "testncid",
					DeviceType: v1alpha1.DeviceTypeInfiniBandNIC,
				},
			},
		},
	}

	// Mtpnc with just Infiniband interface
	testMTPNC10 := v1alpha1.MultitenantPodNetworkConfig{
		Status: v1alpha1.MultitenantPodNetworkConfigStatus{},
	}

	return &Client{
		mtPodCache: map[string]*v1.Pod{
			"testpod1namespace/testpod1":   &testPod1,
			"testpod3namespace/testpod3":   &testPod3,
			"testpod4namespace/testpod4":   &testPod4,
			"testpod5namespace/testpod5":   &testPod5,
			"testpod6namespace/testpod6":   &testPod6,
			"testpod7namespace/testpod7":   &testPod7,
			"testpod8namespace/testpod8":   &testPod8,
			"testpod9namespace/testpod9":   &testPod9,
			"testpod10namespace/testpod10": &testPod10,
		},
		mtpncCache: map[string]*v1alpha1.MultitenantPodNetworkConfig{
			"testpod1namespace/testpod1":   &testMTPNC1,
			"testpod2namespace/testpod2":   &testMTPNC2,
			"testpod4namespace/testpod4":   &testMTPNC4,
			"testpod5namespace/testpod5":   &testMTPNC3,
			"testpod6namespace/testpod6":   &testMTPNC5,
			"testpod7namespace/testpod7":   &testMTPNCMulti,
			"testpod8namespace/testpod8":   &testMTPNC8,
			"testpod9namespace/testpod9":   &testMTPNC9,
			"testpod10namespace/testpod10": &testMTPNC10,
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
	testInterfaceInfos1 := v1alpha1.InterfaceInfo{
		NCID:            "testncid",
		PrimaryIP:       "192.168.0.1/32",
		MacAddress:      "00:00:00:00:00:00",
		GatewayIP:       "10.0.0.1",
		DeviceType:      v1alpha1.DeviceTypeVnetNIC,
		AccelnetEnabled: false,
	}

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	testMTPNC1.Status.InterfaceInfos = []v1alpha1.InterfaceInfo{testInterfaceInfos1}

	c.mtpncCache["testpod1namespace/testpod1"] = &testMTPNC1
}

func (c *Client) SetMTPNCNotReady() {
	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	c.mtpncCache["testpod1namespace/testpod1"] = &testMTPNC1
}
