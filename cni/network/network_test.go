package network

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/common"
	acnnetwork "github.com/Azure/azure-container-networking/network"
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
		Ipam: struct {
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

func GetTestResources() (*NetPlugin, *acnnetwork.MockNetworkManager) {
	pluginName := "testplugin"
	config := &common.PluginConfig{}
	grpcClient := &nns.MockGrpcClient{}
	plugin, _ := NewPlugin(pluginName, config, grpcClient, &Multitenancy{}, nil)
	plugin.report = &telemetry.CNIReport{}
	mockNetworkManager := acnnetwork.NewMockNetworkmanager()
	plugin.nm = mockNetworkManager
	plugin.ipamInvoker = NewMockIpamInvoker(false, false, false)
	return plugin, mockNetworkManager
}

// Happy path scenario for add and delete
func TestPluginAdd(t *testing.T) {
	plugin, _ := GetTestResources()
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
	plugin, _ := GetTestResources()
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
	plugin, _ := GetTestResources()

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
	plugin, _ := GetTestResources()

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
	plugin, _ := GetTestResources()

	tests := []struct {
		name              string
		methods           []string
		cniArgs           []cniSkel.CmdArgs
		wantErr           []bool
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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for i, method := range tt.methods {
				if tt.wantErr[i] {
					plugin.ipamInvoker = NewMockIpamInvoker(false, true, false)
				} else {
					plugin.ipamInvoker = NewMockIpamInvoker(false, false, false)
				}

				if method == CNI_ADD {
					err = plugin.Add(&tt.cniArgs[i])
				} else if method == CNI_DEL {
					err = plugin.Delete(&tt.cniArgs[i])
				}

				if tt.wantErr[i] {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				} else {
					require.NoError(t, err)
				}
			}
		})

		endpoints, _ := plugin.nm.GetAllEndpoints(nwCfg.Name)
		require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == tt.expectedEndpoints }))
	}
}

// Test cni handles ipam CNI_DEL failures as expected
func TestIpamDeleteFail(t *testing.T) {
	plugin, _ := GetTestResources()

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

			plugin.ipamInvoker = NewMockIpamInvoker(false, true, false)
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
				nm:          acnnetwork.NewMockNetworkmanager(),
				ipamInvoker: NewMockIpamInvoker(true, false, false),
				report:      &telemetry.CNIReport{},
				tb:          &telemetry.TelemetryBuffer{},
			},
			wantErr: false,
		},
		{
			name: "Dualstack ipv6 fail",
			plugin: &NetPlugin{
				Plugin:      cniPlugin,
				nm:          acnnetwork.NewMockNetworkmanager(),
				ipamInvoker: NewMockIpamInvoker(true, false, true),
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
				nm:          acnnetwork.NewMockNetworkmanager(),
				ipamInvoker: NewMockIpamInvoker(false, false, false),
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
				nm:          acnnetwork.NewMockNetworkmanager(),
				ipamInvoker: NewMockIpamInvoker(false, false, false),
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
				nm:          acnnetwork.NewMockNetworkmanager(),
				ipamInvoker: NewMockIpamInvoker(false, false, false),
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
				nm:                 acnnetwork.NewMockNetworkmanager(),
				tb:                 &telemetry.TelemetryBuffer{},
				report:             &telemetry.CNIReport{},
				multitenancyClient: NewMockMultitenancy(false),
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
			name: "Add Fail",
			plugin: &NetPlugin{
				Plugin:             plugin,
				nm:                 acnnetwork.NewMockNetworkmanager(),
				tb:                 &telemetry.TelemetryBuffer{},
				report:             &telemetry.CNIReport{},
				multitenancyClient: NewMockMultitenancy(true),
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
	plugin, _ := GetTestResources()
	plugin.multitenancyClient = NewMockMultitenancy(false)
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
Baremetal scenarios
*/

func TestPluginBaremetalAdd(t *testing.T) {
	plugin, _ := cni.NewPlugin("test", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "baremetal-net",
		ExecutionMode:              string(Baremetal),
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
				nm:        acnnetwork.NewMockNetworkmanager(),
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
				nm:        acnnetwork.NewMockNetworkmanager(),
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
	plugin, _ := GetTestResources()
	plugin.nnsClient = &nns.MockGrpcClient{}
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "baremetal-net",
		ExecutionMode:              string(Baremetal),
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
			plugin, err := NewPlugin("test", &tt.config, nil, nil, nil)
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
	plugin, _ := GetTestResources()

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
		Id:           podinterfaceid,
		ContainerID:  infracontainerid,
		IPAddresses: []net.IPNet{
			*ipnet,
		},
	}

	return &ep
}

func TestGetAllEndpointState(t *testing.T) {
	plugin, mockNetworkManager := GetTestResources()
	networkid := "azure"

	ep1 := getTestEndpoint("podname1", "podnamespace1", "10.0.0.1/24", "podinterfaceid1", "testcontainerid1")
	ep2 := getTestEndpoint("podname2", "podnamespace2", "10.0.0.2/24", "podinterfaceid2", "testcontainerid2")

	err := mockNetworkManager.CreateEndpoint(nil, networkid, ep1)
	require.NoError(t, err)

	err = mockNetworkManager.CreateEndpoint(nil, networkid, ep2)
	require.NoError(t, err)

	state, err := plugin.GetAllEndpointState(networkid)
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			ep1.Id: {
				PodEndpointId: ep1.Id,
				PodName:       ep1.PODName,
				PodNamespace:  ep1.PODNameSpace,
				ContainerID:   ep1.ContainerID,
				IPAddresses:   ep1.IPAddresses,
			},
			ep2.Id: {
				PodEndpointId: ep2.Id,
				PodName:       ep2.PODName,
				PodNamespace:  ep2.PODNameSpace,
				ContainerID:   ep2.ContainerID,
				IPAddresses:   ep2.IPAddresses,
			},
		},
	}

	require.Exactly(t, res, state)
}

func TestEndpointsWithEmptyState(t *testing.T) {
	plugin, _ := GetTestResources()
	networkid := "azure"
	state, err := plugin.GetAllEndpointState(networkid)
	require.NoError(t, err)
	require.Equal(t, 0, len(state.ContainerInterfaces))
}

func TestGetNetworkName(t *testing.T) {
	plugin, _ := GetTestResources()
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
			nwName, _ := plugin.getNetworkName("", "", "", &tt.nwCfg)
			require.Equal(t, tt.nwCfg.Name, nwName)
		})
	}
}
