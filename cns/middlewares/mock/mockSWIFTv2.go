package middlewares

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"
)

var (
	errMTPNCNotReady  = errors.New("mtpnc is not ready")
	errFailedToGetPod = errors.New("failed to get pod")
)

type MockSWIFTv2Middleware struct {
	mtPodState map[string]*v1.Pod
	mtpncState map[string]*v1alpha1.MultitenantPodNetworkConfig
}

func NewMockSWIFTv2Middleware() *MockSWIFTv2Middleware {
	testPod1 := v1.Pod{}
	testPod1.Labels = make(map[string]string)
	testPod1.Labels[configuration.LabelPodSwiftV2] = "true"

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}
	testMTPNC1.Status.PrimaryIP = "192.168.0.1"
	testMTPNC1.Status.MacAddress = "00:00:00:00:00:00"
	testMTPNC1.Status.GatewayIP = "10.0.0.1"
	testMTPNC1.Status.NCID = "testncid"

	return &MockSWIFTv2Middleware{
		mtPodState: map[string]*v1.Pod{"testpod1namespace/testpod1": &testPod1},
		mtpncState: map[string]*v1alpha1.MultitenantPodNetworkConfig{"testpod1namespace/testpod1": &testMTPNC1},
	}
}

// validateMultitenantIPConfigsRequest validates if pod is multitenant
// nolint
func (m *MockSWIFTv2Middleware) ValidateIPConfigsRequest(_ context.Context, req *cns.IPConfigsRequest) (respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := fmt.Sprintf("unmarshalling pod info from ipconfigs request %v failed with error %v", req, err)
		return types.UnexpectedError, errBuf
	}
	podNamespacedName := k8types.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod, ok := m.mtPodState[podNamespacedName.String()]
	if !ok {
		errBuf := fmt.Sprintf("failed to get pod %v with error %v", podNamespacedName, err)
		return types.UnexpectedError, errBuf
	}
	// check the pod labels for Swift V2, enrich the request with the multitenant flag.
	if _, ok := pod.Labels[configuration.LabelPodSwiftV2]; ok {
		req.SecondaryInterfacesExist = true
	}
	return types.Success, ""
}

// GetSWIFTv2IPConfig(podInfo PodInfo) (*PodIpInfo, error)
// GetMultitenantIPConfig returns the IP config for a multitenant pod from the MTPNC CRD
func (m *MockSWIFTv2Middleware) GetIPConfig(_ context.Context, podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpncNamespacedName := k8types.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	mtpnc, ok := m.mtpncState[mtpncNamespacedName.String()]
	if !ok {
		return cns.PodIpInfo{}, errFailedToGetPod
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if mtpnc.Status.PrimaryIP == "" || mtpnc.Status.MacAddress == "" || mtpnc.Status.NCID == "" || mtpnc.Status.GatewayIP == "" {
		return cns.PodIpInfo{}, errMTPNCNotReady
	}
	podIPInfo := cns.PodIpInfo{}
	podIPInfo.PodIPConfig = cns.IPSubnet{
		IPAddress: mtpnc.Status.PrimaryIP,
	}
	podIPInfo.MacAddress = mtpnc.Status.MacAddress
	podIPInfo.NICType = cns.DelegatedVMNIC
	podIPInfo.SkipDefaultRoutes = false

	return podIPInfo, nil
}

func (m *MockSWIFTv2Middleware) SetRoutes(_ *cns.PodIpInfo) error {
	return nil
}
