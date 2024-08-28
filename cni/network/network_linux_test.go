//go:build linux
// +build linux

package network

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetNetworkOptions(t *testing.T) {
	tests := []struct {
		name             string
		cnsNwConfig      cns.GetNetworkContainerResponse
		nwInfo           network.NetworkInfo
		expectedVlanID   string
		expectedSnatBrIP string
	}{
		{
			name: "set network options multitenancy",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "169.254.0.4",
						PrefixLength: 17,
					},
					GatewayIPAddress: "169.254.0.1",
				},
			},
			nwInfo: network.NetworkInfo{
				Options: make(map[string]interface{}),
			},
			expectedVlanID:   "1",
			expectedSnatBrIP: "169.254.0.1/17",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setNetworkOptions(&tt.cnsNwConfig, &tt.nwInfo)
			require.Condition(t, assert.Comparison(func() bool {
				optMap := tt.nwInfo.Options[dockerNetworkOption]
				vlanID, ok := optMap.(map[string]interface{})[network.VlanIDKey]
				if !ok {
					return false
				}
				snatBridgeIP, ok := optMap.(map[string]interface{})[network.SnatBridgeIPKey]
				return ok && vlanID == tt.expectedVlanID && snatBridgeIP == tt.expectedSnatBrIP
			}))
		})
	}
}

func TestSetEndpointOptions(t *testing.T) {
	tests := []struct {
		name        string
		cnsNwConfig cns.GetNetworkContainerResponse
		epInfo      network.EndpointInfo
		vethName    string
	}{
		{
			name: "set endpoint options multitenancy",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "169.254.0.4",
						PrefixLength: 17,
					},
					GatewayIPAddress: "169.254.0.1",
				},
				AllowHostToNCCommunication: true,
				AllowNCToHostCommunication: false,
				NetworkContainerID:         "abcd",
			},
			epInfo: network.EndpointInfo{
				Data: make(map[string]interface{}),
			},
			vethName: "azv1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setEndpointOptions(&tt.cnsNwConfig, &tt.epInfo, tt.vethName)
			require.Condition(t, assert.Comparison(func() bool {
				vlanID := tt.epInfo.Data[network.VlanIDKey]
				localIP := tt.epInfo.Data[network.LocalIPKey]
				snatBrIP := tt.epInfo.Data[network.SnatBridgeIPKey]

				return tt.epInfo.AllowInboundFromHostToNC == true &&
					tt.epInfo.AllowInboundFromNCToHost == false &&
					tt.epInfo.NetworkContainerID == "abcd" &&
					vlanID == 1 &&
					localIP == "169.254.0.4/17" &&
					snatBrIP == "169.254.0.1/17"
			}))
		})
	}
}

func TestAddDefaultRoute(t *testing.T) {
	tests := []struct {
		name   string
		gwIP   string
		epInfo network.EndpointInfo
		result network.InterfaceInfo
	}{
		{
			name: "add default route multitenancy",
			gwIP: "192.168.0.1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			addDefaultRoute(tt.gwIP, &tt.epInfo, &tt.result)
			require.Condition(t, assert.Comparison(func() bool {
				return len(tt.epInfo.Routes) == 1 &&
					len(tt.result.Routes) == 1 &&
					tt.epInfo.Routes[0].DevName == snatInterface &&
					tt.epInfo.Routes[0].Gw.String() == "192.168.0.1"
			}))
		})
	}
}

func TestAddSnatForDns(t *testing.T) {
	tests := []struct {
		name   string
		gwIP   string
		epInfo network.EndpointInfo
		result network.InterfaceInfo
	}{
		{
			name: "add snat for dns",
			gwIP: "192.168.0.1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			addSnatForDNS(tt.gwIP, &tt.epInfo, &tt.result)
			require.Condition(t, assert.Comparison(func() bool {
				return len(tt.epInfo.Routes) == 1 &&
					len(tt.result.Routes) == 1 &&
					tt.epInfo.Routes[0].DevName == snatInterface &&
					tt.epInfo.Routes[0].Gw.String() == "192.168.0.1" &&
					tt.epInfo.Routes[0].Dst.String() == "168.63.129.16/32"
			}))
		})
	}
}

// Test Linux Multitenancy Add
func TestPluginMultitenancyLinuxAdd(t *testing.T) {
	plugin, _ := cni.NewPlugin("test", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		plugin     *NetPlugin
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add Happy path",
			plugin: &NetPlugin{
				Plugin:             plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				tb:                 &telemetry.TelemetryBuffer{},
				report:             &telemetry.CNIReport{},
				multitenancyClient: NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1()}),
			},

			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
		{
			name: "Add Fail",
			plugin: &NetPlugin{
				Plugin:             plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				tb:                 &telemetry.TelemetryBuffer{},
				report:             &telemetry.CNIReport{},
				multitenancyClient: NewMockMultitenancy(true, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1()}),
			},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: errMockMulAdd.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg, "Expected %v but got %+v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)

				require.Len(t, endpoints, 1)
			}
		})
	}
}

func TestPluginMultitenancyLinuxDelete(t *testing.T) {
	plugin := GetTestResources()
	plugin.multitenancyClient = NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1()})
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
		wantNumEps []int
	}{
		{
			name:    "Multitenancy delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    false,
			wantNumEps: []int{1, 0},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for idx, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(tt.args)
				} else if method == CNI_DEL {
					err = plugin.Delete(tt.args)
				}
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Len(t, endpoints, tt.wantNumEps[idx])
			}
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
