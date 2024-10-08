package nodesubnet_test

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/ipam"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/store"
)

func getMockStore() store.KeyValueStore {
	mockStore := store.NewMockStore("")
	endpointState := map[string]*restserver.EndpointInfo{
		"12e65d89e58cb23c784e97840cf76866bfc9902089bdc8e87e9f64032e312b0b": {
			PodName:      "coredns-54b69f46b8-ldmwr",
			PodNamespace: "kube-system",
			IfnameToIPMap: map[string]*restserver.IPInfo{
				"eth0": {
					IPv4: []net.IPNet{
						{
							IP:   net.IPv4(10, 10, 0, 52),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		},
		"1fc5176913a3a1a7facfb823dde3b4ded404041134fef4f4a0c8bba140fc0413": {
			PodName:      "load-test-7f7d49687d-wxc9p",
			PodNamespace: "load-test",
			IfnameToIPMap: map[string]*restserver.IPInfo{
				"eth0": {
					IPv4: []net.IPNet{
						{
							IP:   net.IPv4(10, 10, 0, 63),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		},
	}

	err := mockStore.Write(restserver.EndpointStoreKey, endpointState)
	if err != nil {
		return nil
	}
	return mockStore
}

type MockIpamStateReconciler struct{}

func (m *MockIpamStateReconciler) ReconcileIPAMState(ncRequests []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, _ *v1alpha.NodeNetworkConfig) types.ResponseCode {
	if len(ncRequests) == 1 && len(ncRequests[0].SecondaryIPConfigs) == len(podInfoByIP) {
		return types.Success
	}

	return types.UnexpectedError
}

func TestNewCNSPodInfoProvider(t *testing.T) {
	tests := []struct {
		name       string
		store      store.KeyValueStore
		wantErr    bool
		reconciler ipam.StateReconciler
		exp        int
	}{
		{
			name:       "happy_path",
			store:      getMockStore(),
			wantErr:    false,
			reconciler: &MockIpamStateReconciler{},
			exp:        2,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := testContext(t)
			defer cancel()

			podInfoByIPProvider, err := cnireconciler.NewCNSPodInfoProvider(tt.store)
			checkErr(t, err, false)

			got, err := nodesubnet.ReconcileInitialCNSState(ctx, tt.reconciler, podInfoByIPProvider)
			checkErr(t, err, tt.wantErr)
			if got != tt.exp {
				t.Errorf("got %d IPs reconciled, expected %d", got, tt.exp)
			}
		})
	}
}
