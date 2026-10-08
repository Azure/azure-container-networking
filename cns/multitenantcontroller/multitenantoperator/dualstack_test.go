//nolint:goconst // Repeated literals keep the address and error test cases readable.
package multitenantoperator

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/mockclients"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	ncapi "github.com/Azure/azure-container-networking/crd/multitenantnetworkcontainer/api/v1alpha1"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestIPv6Configuration(t *testing.T) {
	valid := ncapi.MultiTenantNetworkContainerStatus{
		IPv6:       "fd00:1234::4",
		IPv6Prefix: "fd00:1234::4/128",
		IPSubnetV6: "fd00:1234::/64",
		GatewayV6:  "fe80::1",
	}
	want := cns.IPConfiguration{
		IPSubnet:         cns.IPSubnet{IPAddress: valid.IPv6, PrefixLength: 64},
		GatewayIPAddress: valid.GatewayV6,
	}
	tests := []struct {
		name   string
		change func(*ncapi.MultiTenantNetworkContainerStatus)
		err    string
	}{
		{"valid", func(_ *ncapi.MultiTenantNetworkContainerStatus) {}, ""},
		{"without allocation prefix", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "" }, ""},
		{"allocation range", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "fd00:1234::/120" }, ""},
		{"normalized address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "FD00:1234:0:0:0:0:0:4" }, ""},
		{"missing address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "" }, "ipv6 address"},
		{"malformed address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "not-an-ip" }, "ipv6 address"},
		{"IPv4 address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "10.0.0.4" }, "ipv6 address"},
		{"mapped address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "::ffff:10.0.0.4" }, "ipv6 address"},
		{"zoned address", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6 = "fd00:1234::4%eth0" }, "ipv6 address"},
		{"missing subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "" }, "ipSubnetV6"},
		{"malformed subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "fd00::/129" }, "ipSubnetV6"},
		{"IPv4 subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "10.0.0.0/24" }, "ipSubnetV6"},
		{"mapped subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "::ffff:10.0.0.0/120" }, "ipSubnetV6"},
		{"zoned subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "fd00:1234::%eth0/64" }, "ipSubnetV6"},
		{"outside subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPSubnetV6 = "fd00:5678::/64" }, "outside ipSubnetV6"},
		{"malformed allocation", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "fd00::/129" }, "ipv6Prefix"},
		{"IPv4 allocation", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "10.0.0.4/32" }, "ipv6Prefix"},
		{"mapped allocation", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "::ffff:10.0.0.4/128" }, "ipv6Prefix"},
		{"zoned allocation", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "fd00:1234::4%eth0/128" }, "ipv6Prefix"},
		{"outside allocation", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "fd00:1234::5/128" }, "ipv6Prefix"},
		{"allocation exceeds subnet", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.IPv6Prefix = "fd00:1234::/48" }, "ipv6Prefix"},
		{"missing gateway", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.GatewayV6 = "" }, "gatewayV6"},
		{"malformed gateway", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.GatewayV6 = "invalid" }, "gatewayV6"},
		{"IPv4 gateway", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.GatewayV6 = "10.0.0.1" }, "gatewayV6"},
		{"mapped gateway", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.GatewayV6 = "::ffff:10.0.0.1" }, "gatewayV6"},
		{"zoned gateway", func(s *ncapi.MultiTenantNetworkContainerStatus) { s.GatewayV6 = "fe80::1%eth0" }, "gatewayV6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := valid
			tt.change(&status)
			got, err := ipv6Configuration(status.IPv6, status.IPv6Prefix, status.IPSubnetV6, status.GatewayV6)
			if tt.err != "" {
				require.ErrorIs(t, err, errInvalidIPv6Configuration)
				require.ErrorContains(t, err, tt.err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
	t.Run("IPv4 only", func(t *testing.T) {
		got, err := ipv6Configuration("", "", "", "")
		require.NoError(t, err)
		require.Empty(t, got)
	})
	t.Run("allocation only", func(t *testing.T) {
		got, err := ipv6Configuration("", valid.IPv6Prefix, "", "")
		require.ErrorIs(t, err, errInvalidIPv6Configuration)
		require.Empty(t, got)
	})
	t.Run("host subnet", func(t *testing.T) {
		got, err := ipv6Configuration(valid.IPv6, valid.IPv6Prefix, valid.IPv6Prefix, valid.GatewayV6)
		require.NoError(t, err)
		want.IPSubnet.PrefixLength = 128
		require.Equal(t, want, got)
	})
}

func TestDualStackReconcile(t *testing.T) {
	logger.InitLogger("dualstack-reconcile", 0, 0, t.TempDir()) //nolint:staticcheck // The reconciler still uses the global logger.
	for _, state := range []string{NCStateInitialized, NCStateSucceeded} {
		for _, scenario := range []struct {
			name   string
			status ncapi.MultiTenantNetworkContainerStatus
			err    string
		}{
			{name: "IPv4 only"},
			{
				name: "dual stack",
				status: ncapi.MultiTenantNetworkContainerStatus{
					IPv6: "fd00:1234::4", IPv6Prefix: "fd00:1234::4/128",
					IPSubnetV6: "fd00:1234::/64", GatewayV6: "fe80::1",
				},
			},
			{
				name: "without allocation prefix",
				status: ncapi.MultiTenantNetworkContainerStatus{
					IPv6: "fd00:1234::4", IPSubnetV6: "fd00:1234::/64", GatewayV6: "fe80::1",
				},
			},
			{
				name: "invalid gateway",
				status: ncapi.MultiTenantNetworkContainerStatus{
					IPv6: "fd00:1234::4", IPSubnetV6: "fd00:1234::/64", GatewayV6: "invalid",
				},
				err: "gatewayV6",
			},
			{
				name:   "partial status",
				status: ncapi.MultiTenantNetworkContainerStatus{IPv6Prefix: "fd00:1234::4/128"},
				err:    "ipv6 address",
			},
			{
				name: "outside subnet",
				status: ncapi.MultiTenantNetworkContainerStatus{
					IPv6: "fd00:5678::4", IPSubnetV6: "fd00:1234::/64", GatewayV6: "fe80::1",
				},
				err: "outside ipSubnetV6",
			},
		} {
			t.Run(state+"/"+scenario.name, func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				kubeClient := mockclients.NewMockClient(mockCtl)
				service := mockclients.NewMockcnsRESTservice(mockCtl)
				nc := ncapi.MultiTenantNetworkContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "test"},
					Spec:       ncapi.MultiTenantNetworkContainerSpec{UUID: "nc-id"},
					Status:     scenario.status,
				}
				nc.Status.State = state
				nc.Status.IP = "10.0.0.4"
				nc.Status.IPSubnet = "10.0.0.0/24"
				nc.Status.Gateway = "10.0.0.1"
				nc.Status.PrimaryInterfaceIdentifier = "primary-interface"
				nc.Status.MultiTenantInfo = ncapi.MultiTenantInfo{EncapType: "Vlan", ID: 42}
				key := types.NamespacedName{Name: nc.Name, Namespace: nc.Namespace}
				orchestratorContext, err := json.Marshal(cns.KubernetesPodInfo{PodName: nc.Name, PodNamespace: nc.Namespace})
				require.NoError(t, err)
				kubeClient.EXPECT().Get(gomock.Any(), key, gomock.Any()).SetArg(2, nc)
				service.EXPECT().GetNetworkContainerInternal(cns.GetNetworkContainerRequest{
					NetworkContainerid: nc.Spec.UUID, OrchestratorContext: orchestratorContext,
				}).Return(cns.GetNetworkContainerResponse{}, cnstypes.UnknownContainerID)
				if scenario.err == "" {
					want := &cns.CreateNetworkContainerRequest{
						NetworkContainerid: nc.Spec.UUID, NetworkContainerType: cns.Kubernetes,
						OrchestratorContext: orchestratorContext, Version: "0",
						IPConfiguration: cns.IPConfiguration{
							IPSubnet:         cns.IPSubnet{IPAddress: nc.Status.IP, PrefixLength: 24},
							GatewayIPAddress: nc.Status.Gateway,
						},
						PrimaryInterfaceIdentifier: nc.Status.PrimaryInterfaceIdentifier,
						MultiTenancyInfo:           cns.MultiTenancyInfo{EncapType: "Vlan", ID: 42},
					}
					if nc.Status.IPv6 != "" {
						want.IPv6Configuration = cns.IPConfiguration{
							IPSubnet:         cns.IPSubnet{IPAddress: nc.Status.IPv6, PrefixLength: 64},
							GatewayIPAddress: nc.Status.GatewayV6,
						}
					}
					service.EXPECT().CreateOrUpdateNetworkContainerInternal(want).Return(cnstypes.Success)
					writer := mockclients.NewMockSubResourceWriter(mockCtl)
					kubeClient.EXPECT().Status().Return(writer)
					succeeded := nc.DeepCopy()
					succeeded.Status.State = NCStateSucceeded
					writer.EXPECT().Update(gomock.Any(), succeeded).Return(nil)
				}
				r := &multiTenantCrdReconciler{KubeClient: clientWithApply{kubeClient}, CNSRestService: service}
				result, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: key})
				require.Empty(t, result)
				if scenario.err != "" {
					require.ErrorIs(t, err, errInvalidIPv6Configuration)
					require.ErrorContains(t, err, scenario.err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestDualStackReconcileExistingNC(t *testing.T) {
	logger.InitLogger("dualstack-existing-nc", 0, 0, t.TempDir()) //nolint:staticcheck // The reconciler still uses the global logger.
	for _, state := range []string{NCStateInitialized, NCStateSucceeded} {
		for _, returnCode := range []cnstypes.ResponseCode{cnstypes.Success, cnstypes.UnexpectedError} {
			t.Run(state+"/"+returnCode.String(), func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				kubeClient := mockclients.NewMockClient(mockCtl)
				service := mockclients.NewMockcnsRESTservice(mockCtl)
				nc := ncapi.MultiTenantNetworkContainer{
					ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "test"},
					Spec:       ncapi.MultiTenantNetworkContainerSpec{UUID: "nc-id"},
					Status: ncapi.MultiTenantNetworkContainerStatus{
						State: state, IPv6: "fd00:1234::4",
						IPSubnetV6: "fd00:1234::/64", GatewayV6: "fe80::1",
					},
				}
				key := types.NamespacedName{Name: nc.Name, Namespace: nc.Namespace}
				kubeClient.EXPECT().Get(gomock.Any(), key, gomock.Any()).SetArg(2, nc)
				service.EXPECT().GetNetworkContainerInternal(gomock.Any()).Return(cns.GetNetworkContainerResponse{}, returnCode)
				r := &multiTenantCrdReconciler{KubeClient: clientWithApply{kubeClient}, CNSRestService: service}
				_, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: key})
				if returnCode == cnstypes.Success {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}
