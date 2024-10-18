package network

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/common"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/nns"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	eth0IfName = "eth0"
)

var (
	args  *cniSkel.CmdArgs
	nwCfg cni.NetworkConfig
)

func TestMain(m *testing.M) {
	nwCfg = cni.NetworkConfig{
		Name:              "test-nwcfg",
		CNIVersion:        "0.3.0",
		Type:              "azure-vnet",
		Mode:              "bridge",
		Master:            eth0IfName,
		IPsToRouteViaHost: []string{"169.254.20.10"},
		IPAM: struct {
			Mode          string `json:"mode,omitempty"`
			Type          string `json:"type"`
			Environment   string `json:"environment,omitempty"`
			AddrSpace     string `json:"addressSpace,omitempty"`
			Subnet        string `json:"subnet,omitempty"`
			Address       string `json:"ipAddress,omitempty"`
			QueryInterval string `json:"queryInterval,omitempty"`
		}{
			Type: "azure-cns",
		},
	}

	args = &cniSkel.CmdArgs{
		ContainerID: "test-container",
		Netns:       "test-container",
	}
	args.StdinData = nwCfg.Serialize()
	podEnv := cni.K8SPodEnvArgs{
		K8S_POD_NAME:      "test-pod",
		K8S_POD_NAMESPACE: "test-pod-namespace",
	}
	args.Args = fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podEnv.K8S_POD_NAME, podEnv.K8S_POD_NAMESPACE)
	args.IfName = eth0IfName

	// Run tests.
	exitCode := m.Run()
	os.Exit(exitCode)
}

func GetTestResources() *NetPlugin {
	pluginName := "testplugin"
	isIPv6 := false
	config := &common.PluginConfig{}
	grpcClient := &nns.MockGrpcClient{}
	plugin, _ := NewPlugin(pluginName, config, grpcClient, &Multitenancy{})
	plugin.report = &telemetry.CNIReport{}
	mockNetworkManager := acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil))
	plugin.nm = mockNetworkManager
	plugin.ipamInvoker = NewMockIpamInvoker(isIPv6, false, false, false, false)
	return plugin
}

// Happy path scenario for add and delete
func TestPluginAdd(t *testing.T) {
	plugin := GetTestResources()
	tests := []struct {
		name       string
		nwCfg      cni.NetworkConfig
		args       *cniSkel.CmdArgs
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "Add Happy path",
			plugin: plugin,
			nwCfg:  nwCfg,
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.Add(tt.args)
			require.NoError(t, err)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(tt.nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }))
			}
		})
	}
}

// Happy path scenario for delete
func TestPluginDelete(t *testing.T) {
	plugin := GetTestResources()
	tests := []struct {
		name       string
		args       *cniSkel.CmdArgs
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "Add Happy path",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			err = plugin.Delete(tt.args)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			}
		})
	}
}

// Test multiple cni add calls
func TestPluginSecondAddDifferentPod(t *testing.T) {
	plugin := GetTestResources()

	tests := []struct {
		name       string
		methods    []string
		cniArgs    []cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "CNI multiple add for multiple pods",
			methods: []string{CNI_ADD, CNI_ADD},
			cniArgs: []cniSkel.CmdArgs{
				{
					ContainerID: "test1-container",
					Netns:       "test1-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
					IfName:      eth0IfName,
				},
				{
					ContainerID: "test2-container",
					Netns:       "test2-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container2", "container2-ns"),
					IfName:      eth0IfName,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for i, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(&tt.cniArgs[i])
				}
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 2 }), "Expected 2 but got %v", len(endpoints))
			}
		})
	}
}

// Check CNI returns error if required fields are missing
func TestPluginCNIFieldsMissing(t *testing.T) {
	plugin := GetTestResources()

	tests := []struct {
		name       string
		args       *cniSkel.CmdArgs
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "Interface name not specified",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
			},
			wantErr:    true,
			wantErrMsg: "Interfacename not specified in CNI Args",
		},
		{
			name:   "Container ID not specified",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData: nwCfg.Serialize(),
				Netns:     "test-container",
				Args:      fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:    eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: "Container ID not specified in CNI Args",
		},
		{
			name:   "Pod Namespace not specified",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				Netns:       "test-container",
				ContainerID: "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", ""),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: "Pod Namespace not specified in CNI Args",
		},
		{
			name:   "Pod Name not specified",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				Netns:       "test-container",
				ContainerID: "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: "Pod Name not specified in CNI Args",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test cni handles ipam CNI_ADD failures as expected
func TestIpamAddFail(t *testing.T) {
	plugin := GetTestResources()

	tests := []struct {
		name              string
		methods           []string
		cniArgs           []cniSkel.CmdArgs
		wantErr           []bool
		wantEndpointErr   bool
		wantErrMsg        string
		expectedEndpoints int
	}{
		{
			name:    "ipam add fail",
			methods: []string{CNI_ADD, CNI_DEL},
			cniArgs: []cniSkel.CmdArgs{
				{
					ContainerID: "test1-container",
					Netns:       "test1-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
					IfName:      eth0IfName,
				},
				{
					ContainerID: "test1-container",
					Netns:       "test1-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
					IfName:      eth0IfName,
				},
			},
			wantErr:           []bool{true, false},
			wantErrMsg:        "v4 fail",
			expectedEndpoints: 0,
		},
		{
			name:    "ipam add fail for second add call",
			methods: []string{CNI_ADD, CNI_ADD, CNI_DEL},
			cniArgs: []cniSkel.CmdArgs{
				{
					ContainerID: "test1-container",
					Netns:       "test1-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
					IfName:      eth0IfName,
				},
				{
					ContainerID: "test2-container",
					Netns:       "test2-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container2", "container2-ns"),
					IfName:      eth0IfName,
				},
				{
					ContainerID: "test2-container",
					Netns:       "test2-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container2", "container2-ns"),
					IfName:      eth0IfName,
				},
			},
			wantErr:           []bool{false, true, false},
			wantErrMsg:        "v4 fail",
			expectedEndpoints: 1,
		},
		{
			name:    "cleanup ipam add fail",
			methods: []string{CNI_ADD},
			cniArgs: []cniSkel.CmdArgs{
				{
					ContainerID: "test1-container",
					Netns:       "test1-container",
					StdinData:   nwCfg.Serialize(),
					Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
					IfName:      eth0IfName,
				},
			},
			wantErr:           []bool{false},
			wantEndpointErr:   true,
			wantErrMsg:        "failed to create endpoint: MockEndpointClient Error : Endpoint Error",
			expectedEndpoints: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for i, method := range tt.methods {
				fmt.Println("method", method, "wanterr", tt.wantErr[i])
				if tt.wantErr[i] {
					plugin.ipamInvoker = NewMockIpamInvoker(false, true, false, false, false)
				} else {
					plugin.ipamInvoker = NewMockIpamInvoker(false, false, false, false, false)
				}

				if tt.wantEndpointErr {
					plugin.nm = acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(func(*acnnetwork.EndpointInfo) error {
						return acnnetwork.NewErrorMockEndpointClient("Endpoint Error") //nolint:wrapcheck // ignore wrapping for test
					}))
				}

				if method == CNI_ADD {
					err = plugin.Add(&tt.cniArgs[i])
				} else if method == CNI_DEL {
					err = plugin.Delete(&tt.cniArgs[i])
				}

				if tt.wantErr[i] || tt.wantEndpointErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				} else {
					require.NoError(t, err)
				}

				if tt.wantEndpointErr {
					assert.Len(t, plugin.ipamInvoker.(*MockIpamInvoker).ipMap, 0)
				}
			}
		})

		endpoints, _ := plugin.nm.GetAllEndpoints(nwCfg.Name)
		require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == tt.expectedEndpoints }))
	}
}

// Test cni handles ipam CNI_DEL failures as expected
func TestIpamDeleteFail(t *testing.T) {
	plugin := GetTestResources()

	tests := []struct {
		name       string
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "ipv4 delete fail",
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: "delete fail",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.Add(tt.args)
			require.NoError(t, err)

			plugin.ipamInvoker = NewMockIpamInvoker(false, true, false, false, false)
			err = plugin.Delete(args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				endpoints, _ := plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }), "Expected 0 but got %v", len(endpoints))
			}
		})
	}
}

// test v4 and v6 address allocation from ipam
func TestAddDualStack(t *testing.T) {
	nwCfg.IPV6Mode = "ipv6nat"
	args.StdinData = nwCfg.Serialize()
	cniPlugin, _ := cni.NewPlugin("test", "0.3.0")

	tests := []struct {
		name       string
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Dualstack happy path",
			plugin: &NetPlugin{
				Plugin:      cniPlugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(true, false, false, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr: false,
		},
		{
			name: "Dualstack ipv6 fail",
			plugin: &NetPlugin{
				Plugin:      cniPlugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(true, false, true, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			} else {
				require.NoError(t, err)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }))
			}
		})
	}

	nwCfg.IPV6Mode = ""
	args.StdinData = nwCfg.Serialize()
}

// Test CNI Get call
func TestPluginGet(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")

	tests := []struct {
		name       string
		methods    []string
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "CNI Get happy path",
			methods: []string{CNI_ADD, "GET"},
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr: false,
		},
		{
			name:    "CNI Get fail with network not found",
			methods: []string{"GET"},
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr:    true,
			wantErrMsg: "Network not found",
		},
		{
			name:    "CNI Get fail with endpoint not found",
			methods: []string{CNI_ADD, CNI_DEL, "GET"},
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr:    true,
			wantErrMsg: "Endpoint not found",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error

			for _, method := range tt.methods {
				switch method {
				case CNI_ADD:
					err = tt.plugin.Add(args)
				case CNI_DEL:
					err = tt.plugin.Delete(args)
				case "GET":
					err = tt.plugin.Get(args)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

/*
Multitenancy scenarios
*/
// For use with GetNetworkContainer
func GetTestCNSResponse0() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "192.168.0.4",
				PrefixLength: ipPrefixLen,
			},
			GatewayIPAddress: "192.168.0.1",
		},
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "169.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "169.254.0.1",
		},

		PrimaryInterfaceIdentifier: "10.240.0.4/24",
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        1,
		},
	}
}

// For use with GetAllNetworkContainers
func GetTestCNSResponse1() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.0.0.10",
				PrefixLength: ipPrefixLen,
			},
			GatewayIPAddress: "20.0.0.1",
		},
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "168.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "168.254.0.1",
		},

		PrimaryInterfaceIdentifier: "20.240.0.4/24",
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        multiTenancyVlan1,
		},
	}
}

// For use with GetAllNetworkContainers in windows dualnic
func GetTestCNSResponse2() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "10.0.0.10",
				PrefixLength: ipPrefixLen,
			},
			GatewayIPAddress: "10.0.0.1",
		},
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "169.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "169.254.0.1",
		},

		PrimaryInterfaceIdentifier: "10.240.0.4/24",
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        multiTenancyVlan2,
		},
	}
}

// For use with GetAllNetworkContainers in linux multitenancy
func GetTestCNSResponse3() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
		NetworkContainerID: "Swift_74b34111-6e92-49ee-a82a-8881c850ce0e",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.0.0.10",
				PrefixLength: ipPrefixLen,
			},
			DNSServers: []string{
				"168.63.129.16",
			},
			GatewayIPAddress: "20.0.0.1",
		},
		Routes: []cns.Route{
			// dummy route
			{
				IPAddress:        "192.168.0.4/24",
				GatewayIPAddress: "192.168.0.1",
			},
		},
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        multiTenancyVlan1,
		},
		PrimaryInterfaceIdentifier: "20.240.0.4/24",
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "168.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "168.254.0.1",
		},
		AllowHostToNCCommunication: true,
		AllowNCToHostCommunication: false,
	}
}

// Test Multitenancy Add
func TestPluginMultitenancyAdd(t *testing.T) {
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
				nm:                 acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
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
				nm:                 acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
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

				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }))
			}
		})
	}
}

func TestPluginMultitenancyDelete(t *testing.T) {
	plugin := GetTestResources()
	plugin.multitenancyClient = NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1()})
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	happyArgs := &cniSkel.CmdArgs{
		StdinData:   localNwCfg.Serialize(),
		ContainerID: "test-container",
		Netns:       "test-container",
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
		IfName:      eth0IfName,
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		delArgs    *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "Multitenancy delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args:    happyArgs,
			delArgs: happyArgs,
			wantErr: false,
		},
		{
			name:    "Multitenancy delete net not found",
			methods: []string{CNI_ADD, CNI_DEL},
			args:    happyArgs,
			delArgs: &cniSkel.CmdArgs{
				StdinData: (&cni.NetworkConfig{
					CNIVersion:                 "0.3.0",
					Name:                       "othernet",
					MultiTenancy:               true,
					EnableExactMatchForPodName: true,
					Master:                     "eth0",
				}).Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for _, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(tt.args)
				} else if method == CNI_DEL {
					err = plugin.Delete(tt.delArgs)
				}
			}
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			}
		})
	}
}

/*
Baremetal scenarios
*/
func TestPluginBaremetalAdd(t *testing.T) {
	plugin, _ := cni.NewPlugin("test", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "baremetal-net",
		ExecutionMode:              string(util.Baremetal),
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
			name: "Baremetal Add Happy path",
			plugin: &NetPlugin{
				Plugin:    plugin,
				nm:        acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				tb:        &telemetry.TelemetryBuffer{},
				report:    &telemetry.CNIReport{},
				nnsClient: &nns.MockGrpcClient{},
			},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
		{
			name: "Baremetal Add Fail",
			plugin: &NetPlugin{
				Plugin:    plugin,
				nm:        acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				tb:        &telemetry.TelemetryBuffer{},
				report:    &telemetry.CNIReport{},
				nnsClient: &nns.MockGrpcClient{Fail: true},
			},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: nns.ErrMockNnsAdd.Error(),
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
			}
		})
	}
}

func TestPluginBaremetalDelete(t *testing.T) {
	plugin := GetTestResources()
	plugin.nnsClient = &nns.MockGrpcClient{}
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "baremetal-net",
		ExecutionMode:              string(util.Baremetal),
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "Baremetal delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for _, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(tt.args)
				} else if method == CNI_DEL {
					err = plugin.Delete(tt.args)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			}
		})
	}
}

/*
AKS-Swift scenario
*/
func TestPluginAKSSwiftAdd(t *testing.T) {
	plugin := GetTestResources()

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "aksswift-net",
		ExecutionMode:              string(util.V4Swift),
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
			name:   "AKS Swift Add Happy path",
			plugin: plugin,
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
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
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }))
			}
		})
	}
}

func TestPluginAKSSwiftDelete(t *testing.T) {
	plugin := GetTestResources()
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "aksswift-net",
		ExecutionMode:              string(util.V4Swift),
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "AKS Swift delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for _, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(tt.args)
				} else if method == CNI_DEL {
					err = plugin.Delete(tt.args)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			}
		})
	}
}

func TestNewPlugin(t *testing.T) {
	tests := []struct {
		name    string
		config  common.PluginConfig
		wantErr bool
	}{
		{
			name: "Test new plugin",
			config: common.PluginConfig{
				Version: "0.3.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := NewPlugin("test", &tt.config, nil, nil)
			if tt.wantErr {
				require.NoError(t, err)
				require.NotNil(t, plugin)
			}

			err = plugin.Start(&tt.config)
			if tt.wantErr {
				require.NoError(t, err)
			}

			plugin.Stop()
		})
	}
}

// Test CNI Update call
func TestPluginUpdate(t *testing.T) {
	plugin := GetTestResources()

	err := plugin.Add(args)
	require.NoError(t, err)

	err = plugin.Update(args)
	require.Error(t, err)
}

func getTestEndpoint(podname, podnamespace, ipwithcidr, podinterfaceid, infracontainerid string) *acnnetwork.EndpointInfo {
	ip, ipnet, _ := net.ParseCIDR(ipwithcidr)
	ipnet.IP = ip
	ep := acnnetwork.EndpointInfo{
		PODName:      podname,
		PODNameSpace: podnamespace,
		EndpointID:   podinterfaceid,
		ContainerID:  infracontainerid,
		IPAddresses: []net.IPNet{
			*ipnet,
		},
	}

	return &ep
}

func TestGetAllEndpointState(t *testing.T) {
	plugin := GetTestResources()
	networkid := "azure"

	ep1 := getTestEndpoint("podname1", "podnamespace1", "10.0.0.1/24", "podinterfaceid1", "testcontainerid1")
	ep2 := getTestEndpoint("podname2", "podnamespace2", "10.0.0.2/24", "podinterfaceid2", "testcontainerid2")
	ep3 := getTestEndpoint("podname3", "podnamespace3", "10.240.1.242/16", "podinterfaceid3", "testcontainerid3")

	err := plugin.nm.CreateEndpoint(nil, networkid, ep1)
	require.NoError(t, err)

	err = plugin.nm.CreateEndpoint(nil, networkid, ep2)
	require.NoError(t, err)

	err = plugin.nm.CreateEndpoint(nil, networkid, ep3)
	require.NoError(t, err)

	state, err := plugin.GetAllEndpointState(networkid)
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			ep1.EndpointID: {
				PodEndpointId: ep1.EndpointID,
				PodName:       ep1.PODName,
				PodNamespace:  ep1.PODNameSpace,
				ContainerID:   ep1.ContainerID,
				IPAddresses:   ep1.IPAddresses,
			},
			ep2.EndpointID: {
				PodEndpointId: ep2.EndpointID,
				PodName:       ep2.PODName,
				PodNamespace:  ep2.PODNameSpace,
				ContainerID:   ep2.ContainerID,
				IPAddresses:   ep2.IPAddresses,
			},
			ep3.EndpointID: {
				PodEndpointId: ep3.EndpointID,
				PodName:       ep3.PODName,
				PodNamespace:  ep3.PODNameSpace,
				ContainerID:   ep3.ContainerID,
				IPAddresses:   ep3.IPAddresses,
			},
		},
	}

	require.Exactly(t, res, state)
}

func TestEndpointsWithEmptyState(t *testing.T) {
	plugin := GetTestResources()
	networkid := "azure"
	state, err := plugin.GetAllEndpointState(networkid)
	require.NoError(t, err)
	require.Equal(t, 0, len(state.ContainerInterfaces))
}

func TestGetNetworkName(t *testing.T) {
	plugin := GetTestResources()
	tests := []struct {
		name  string
		nwCfg cni.NetworkConfig
	}{
		{
			name: "get network name",
			nwCfg: cni.NetworkConfig{
				Name: "test-network",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nwName, _ := plugin.getNetworkName("", nil, &tt.nwCfg)
			require.Equal(t, tt.nwCfg.Name, nwName)
		})
	}
}

func TestGetOverlayNatInfo(t *testing.T) {
	nwCfg := &cni.NetworkConfig{ExecutionMode: string(util.V4Swift), IPAM: cni.IPAM{Mode: string(util.V4Overlay)}}
	natInfo := getNATInfo(nwCfg, nil, false)
	require.Empty(t, natInfo, "overlay natInfo should be empty")
}

func TestGetPodSubnetNatInfo(t *testing.T) {
	ncPrimaryIP := "10.241.0.4"
	nwCfg := &cni.NetworkConfig{ExecutionMode: string(util.V4Swift)}
	natInfo := getNATInfo(nwCfg, ncPrimaryIP, false)
	if runtime.GOOS == "windows" {
		require.Equalf(t, natInfo, []policy.NATInfo{
			{VirtualIP: ncPrimaryIP, Destinations: []string{networkutils.AzureDNS}},
			{Destinations: []string{networkutils.AzureIMDS}},
		}, "invalid windows podsubnet natInfo")
	} else {
		require.Empty(t, natInfo, "linux podsubnet natInfo should be empty")
	}
}

type InterfaceGetterMock struct {
	interfaces     []net.Interface
	interfaceAddrs map[string][]net.Addr // key is interfaceName, value is one interface's CIDRs(IPs+Masks)
	err            error
}

func (n *InterfaceGetterMock) GetNetworkInterfaces() ([]net.Interface, error) {
	if n.err != nil {
		return nil, n.err
	}
	return n.interfaces, nil
}

func (n *InterfaceGetterMock) GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error) {
	if n.err != nil {
		return nil, n.err
	}

	// actual net.Addr invokes syscall; here just create a mocked net.Addr{}
	netAddrs := []net.Addr{}
	for _, intf := range n.interfaces {
		if iface.Name == intf.Name {
			return n.interfaceAddrs[iface.Name], nil
		}
	}
	return netAddrs, nil
}

func TestPluginSwiftV2Add(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "swiftv2",
		ExecutionMode:              string(util.V4Overlay),
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	args := &cniSkel.CmdArgs{
		StdinData:   localNwCfg.Serialize(),
		ContainerID: "test-container",
		Netns:       "test-container",
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
		IfName:      eth0IfName,
	}

	tests := []struct {
		name       string
		plugin     *NetPlugin
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "SwiftV2 Add Happy path",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:    args,
			wantErr: false,
		},
		{
			name: "SwiftV2 Invoker Add fail",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, true),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantErr:    true,
			wantErrMsg: "IPAM Invoker Add failed with error: failed to add ipam invoker: NodeNetworkInterfaceFrontendNIC fail",
		},
		{
			name: "SwiftV2 EndpointClient Add fail with NodeNetworkInterfaceFrontendNIC",
			plugin: &NetPlugin{
				Plugin: plugin,
				nm: acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(func(ep *acnnetwork.EndpointInfo) error {
					if ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
						return acnnetwork.NewErrorMockEndpointClient("AddEndpoints Delegated VM NIC failed") //nolint:wrapcheck // ignore wrapping for test
					}

					return nil
				})),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantErr:    true,
			wantErrMsg: "failed to create endpoint: MockEndpointClient Error : AddEndpoints Delegated VM NIC failed",
		},
		{
			name: "SwiftV2 Find Interface By MAC Address Fail with delegated VM NIC",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{},
				},
			},
			args:       args,
			wantErr:    true,
			wantErrMsg: "Failed to find the master interface",
		},
		{
			name: "SwiftV2 Find Interface By Subnet Prefix Fail",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{},
				},
			},
			args: &cniSkel.CmdArgs{
				StdinData: (&cni.NetworkConfig{
					CNIVersion:                 "0.3.0",
					Name:                       "swiftv2",
					ExecutionMode:              string(util.V4Overlay),
					EnableExactMatchForPodName: true,
				}).Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: "Failed to find the master interface",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantErrMsg, err.Error(), "Expected %v but got %+v", tt.wantErrMsg, err.Error())
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			} else {
				require.NoError(t, err)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }))
			}
		})
	}
}

func TestPluginSwiftV2MultipleAddDelete(t *testing.T) {
	// checks cases where we create multiple endpoints in one call (also checks endpoint id)
	// assumes we never get two infras created in one add call
	plugin, _ := cni.NewPlugin("name", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "swiftv2",
		ExecutionMode:              string(util.V4Overlay),
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	args := &cniSkel.CmdArgs{
		StdinData:   localNwCfg.Serialize(),
		ContainerID: "test-container",
		Netns:       "test-container",
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
		IfName:      eth0IfName,
	}

	tests := []struct {
		name       string
		plugin     *NetPlugin
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
		wantNumEps int
		validEpIDs map[string]struct{}
	}{
		{
			name: "SwiftV2 Add Infra and Delegated",
			plugin: &NetPlugin{
				Plugin: plugin,
				nm:     acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewCustomMockIpamInvoker(map[string]acnnetwork.InterfaceInfo{
					"eth0": {
						NICType: cns.InfraNIC,
					},
					"eth2": {
						NICType: cns.NodeNetworkInterfaceFrontendNIC,
					},
				}),
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantErr:    false,
			wantNumEps: 2,
		},
		{
			name: "SwiftV2 Add Infra and InfiniteBand",
			plugin: &NetPlugin{
				Plugin: plugin,
				nm:     acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewCustomMockIpamInvoker(map[string]acnnetwork.InterfaceInfo{
					"eth0": {
						NICType: cns.InfraNIC,
					},
					"eth1": {
						NICType: cns.BackendNIC,
					},
				}),
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantErr:    false,
			wantNumEps: 2,
		},
		{
			name: "SwiftV2 Add Two Delegated",
			plugin: &NetPlugin{
				Plugin: plugin,
				nm:     acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil)),
				ipamInvoker: NewCustomMockIpamInvoker(map[string]acnnetwork.InterfaceInfo{
					"eth1": {
						NICType: cns.NodeNetworkInterfaceFrontendNIC,
					},
					"eth2": {
						NICType: cns.NodeNetworkInterfaceFrontendNIC,
					},
				}),
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantErr:    false,
			wantNumEps: 2,
		},
		{
			// creates 2 endpoints, the first succeeds, the second doesn't
			// ensures that delete is called to clean up the first endpoint that succeeded
			name: "SwiftV2 Partial Add fail with Delegated VM NIC",
			plugin: &NetPlugin{
				Plugin: plugin,
				nm: acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(func(ep *acnnetwork.EndpointInfo) error {
					if ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
						return acnnetwork.NewErrorMockEndpointClient("AddEndpoints Delegated VM NIC failed") //nolint:wrapcheck // ignore wrapping for test
					}

					return nil
				})),
				ipamInvoker: NewCustomMockIpamInvoker(map[string]acnnetwork.InterfaceInfo{
					"eth0": {
						NICType: cns.InfraNIC,
					},
					"eth1": {
						NICType: cns.NodeNetworkInterfaceFrontendNIC,
					},
				}),
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{Name: "eth0"},
					},
				},
			},
			args:       args,
			wantNumEps: 0,
			wantErr:    true,
			wantErrMsg: "failed to create endpoint: MockEndpointClient Error : AddEndpoints Delegated VM NIC failed",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantErrMsg, err.Error(), "Expected %v but got %+v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
			endpoints, _ := tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)
			require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == tt.wantNumEps }))
			for _, ep := range endpoints {
				if ep.NICType == cns.InfraNIC {
					require.Equal(t, "test-con-"+tt.args.IfName, ep.EndpointID, "infra nic must use ifname for its endpoint id")
				} else {
					require.Regexp(t, `\d+$`, ep.EndpointID, "other nics must use an index for their endpoint ids")
				}
			}

			err = tt.plugin.Delete(tt.args)
			require.NoError(t, err)

			endpoints, _ = tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)
			require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
		})
	}
}

// test findMasterInterface with different NIC types
func TestFindMasterInterface(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")
	endpointIndex := 1
	macAddress := "12:34:56:78:90:ab"

	tests := []struct {
		name        string
		endpointOpt createEpInfoOpt
		plugin      *NetPlugin
		nwCfg       *cni.NetworkConfig
		want        string // expected master interface name
		wantErr     bool
	}{
		{
			name: "Find master interface by infraNIC with a master interfaceName in swiftv1 path",
			plugin: &NetPlugin{
				Plugin: plugin,
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{
							Name: "eth0",
						},
					},
				},
			},
			endpointOpt: createEpInfoOpt{
				ipamAddConfig: &IPAMAddConfig{
					nwCfg: &cni.NetworkConfig{
						Master: "eth0", // return this master interface name
					},
				},
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType: cns.InfraNIC,
					HostSubnetPrefix: net.IPNet{
						IP:   net.ParseIP("10.255.0.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
			want:    "eth0",
			wantErr: false,
		},
		{
			name: "Find master interface by one infraNIC",
			plugin: &NetPlugin{
				Plugin: plugin,
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{
							Index: 0,
							Name:  "eth0",
						},
					},
					interfaceAddrs: map[string][]net.Addr{
						"eth0": {
							&net.IPNet{
								IP:   net.IPv4(10, 255, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
							&net.IPNet{
								IP:   net.IPv4(192, 168, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
						},
					},
				},
			},
			endpointOpt: createEpInfoOpt{
				ipamAddConfig: &IPAMAddConfig{
					nwCfg: &cni.NetworkConfig{
						Master: "",
					},
				},
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType: cns.InfraNIC,
					HostSubnetPrefix: net.IPNet{
						IP:   net.ParseIP("10.255.0.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
			want:    "eth0",
			wantErr: false,
		},
		{
			name: "Find master interface from multiple infraNIC interfaces",
			plugin: &NetPlugin{
				Plugin: plugin,
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{
							Index: 0,
							Name:  "eth0",
						},
						{
							Index: 1,
							Name:  "eth1",
						},
					},
					interfaceAddrs: map[string][]net.Addr{
						"eth0": {
							&net.IPNet{
								IP:   net.IPv4(10, 255, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
							&net.IPNet{
								IP:   net.IPv4(192, 168, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
						},
						"eth1": {
							&net.IPNet{
								IP:   net.IPv4(20, 255, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
							&net.IPNet{
								IP:   net.IPv4(30, 255, 0, 1),
								Mask: net.IPv4Mask(255, 255, 255, 0),
							},
						},
					},
				},
			},
			endpointOpt: createEpInfoOpt{
				ipamAddConfig: &IPAMAddConfig{
					nwCfg: &cni.NetworkConfig{
						Master: "",
					},
				},
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType: cns.InfraNIC,
					HostSubnetPrefix: net.IPNet{
						IP:   net.ParseIP("20.255.0.0"),
						Mask: net.CIDRMask(24, 32),
					},
				},
			},
			want:    "eth1",
			wantErr: false,
		},
		{
			name: "Find master interface by delegatedVMNIC",
			plugin: &NetPlugin{
				Plugin: plugin,
				report: &telemetry.CNIReport{},
				tb:     &telemetry.TelemetryBuffer{},
				netClient: &InterfaceGetterMock{
					interfaces: []net.Interface{
						{
							Name:         "eth1",
							HardwareAddr: net.HardwareAddr(macAddress),
						},
					},
				},
			},
			endpointOpt: createEpInfoOpt{
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					MacAddress: net.HardwareAddr(macAddress),
				},
			},
			want:    "eth1",
			wantErr: false,
		},
		{
			name: "Find master interface by backend NIC",
			endpointOpt: createEpInfoOpt{
				endpointIndex: endpointIndex,
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType:    cns.BackendNIC,
					MacAddress: net.HardwareAddr(macAddress),
				},
			},
			want:    ibInterfacePrefix + strconv.Itoa(endpointIndex),
			wantErr: false,
		},
		{
			name: "Find master interface by invalid NIC type",
			endpointOpt: createEpInfoOpt{
				endpointIndex: endpointIndex,
				ifInfo: &acnnetwork.InterfaceInfo{
					NICType:    "invalidType",
					MacAddress: net.HardwareAddr(macAddress),
				},
			},
			want:    "", // default interface name is ""
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			masterInterface := tt.plugin.findMasterInterface(&tt.endpointOpt)
			t.Logf("masterInterface is %s\n", masterInterface)
			require.Equal(t, tt.want, masterInterface)
		})
	}
}

func TestValidateArgs(t *testing.T) {
	p, _ := cni.NewPlugin("name", "0.3.0")
	plugin := &NetPlugin{
		Plugin: p,
	}

	tests := []struct {
		name    string
		args    *cniSkel.CmdArgs
		nwCfg   *cni.NetworkConfig
		wantErr bool
	}{
		{
			name: "Args",
			args: &cniSkel.CmdArgs{
				ContainerID: "5419067fa51b3b942bdd1af1ae78ea5f9cabc67ae71c7b5ef57ba8ca1b2386ec",
				IfName:      "eth0",
			},
			nwCfg: &cni.NetworkConfig{
				Bridge: "azure0",
			},
			wantErr: false,
		},
		{
			name: "Args with spaces and special characters",
			args: &cniSkel.CmdArgs{
				ContainerID: "test2-container",
				IfName:      "vEthernet (Ethernet 2)",
			},
			nwCfg: &cni.NetworkConfig{
				Bridge: ".-_",
			},
			wantErr: false,
		},
		{
			name: "Empty args",
			args: &cniSkel.CmdArgs{
				ContainerID: "",
				IfName:      "",
			},
			nwCfg: &cni.NetworkConfig{
				Bridge: "",
			},
			wantErr: false,
		},
		{
			name: "Invalid args",
			args: &cniSkel.CmdArgs{
				ContainerID: "",
				IfName:      "",
			},
			nwCfg: &cni.NetworkConfig{
				Bridge: "\\value/\"",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.validateArgs(tt.args, tt.nwCfg)
			if tt.wantErr {
				require.Error(t, err, "Expected error but did not receive one")
			} else {
				require.NoError(t, err, "Expected no error but received one")
			}
		})
	}
}
