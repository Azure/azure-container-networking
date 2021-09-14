package network

import (
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/common"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/nns"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/require"
)

func getTestResources() (*netPlugin, *acnnetwork.MockNetworkManager) {
	pluginName := "testplugin"
	config := &common.PluginConfig{}
	grpcClient := &nns.MockGrpcClient{}
	plugin, _ := NewPlugin(pluginName, config, grpcClient)
	plugin.report = &telemetry.CNIReport{}
	mockNetworkManager := acnnetwork.NewMockNetworkmanager()
	plugin.nm = mockNetworkManager
	plugin.ipamInvoker = NewMockIpamInvoker(false)
	return plugin, mockNetworkManager
}

// the Add/Delete methods in Plugin require refactoring to have UT's written for them,
// but the mocks in this test are a start
func TestPluginAdd(t *testing.T) {
	plugin, _ := getTestResources()

	nwCfg := cni.NetworkConfig{
		Name:              "test-nwcfg",
		Type:              "azure-vnet",
		Mode:              "bridge",
		Master:            "eth0",
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

	args := &cniSkel.CmdArgs{
		ContainerID: "test-container",
		Netns:       "test-container",
	}
	args.StdinData = nwCfg.Serialize()
	podEnv := cni.K8SPodEnvArgs{
		K8S_POD_NAME:      "test-pod",
		K8S_POD_NAMESPACE: "test-pod-namespace",
	}
	args.Args = fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podEnv.K8S_POD_NAME, podEnv.K8S_POD_NAMESPACE)
	args.IfName = "azure0"

	plugin.Add(args)
}

// the Delete methods in Plugin require refactoring to have UT's written for them,
// but the mocks in this test are a start
func TestPluginDelete(t *testing.T) {
	plugin, _ := getTestResources()

	nwCfg := cni.NetworkConfig{
		Name:              "test-nwcfg",
		Type:              "azure-vnet",
		Mode:              "bridge",
		Master:            "eth0",
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

	args := &cniSkel.CmdArgs{
		ContainerID: "test-container",
		Netns:       "test-container",
	}
	args.StdinData = nwCfg.Serialize()
	podEnv := cni.K8SPodEnvArgs{
		K8S_POD_NAME:      "test-pod",
		K8S_POD_NAMESPACE: "test-pod-namespace",
	}
	args.Args = fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podEnv.K8S_POD_NAME, podEnv.K8S_POD_NAMESPACE)
	args.IfName = "azure0"

	plugin.Delete(args)
}

// the Delete methods in Plugin require refactoring to have UT's written for them,
// but the mocks in this test are a start
func TestPluginGet(t *testing.T) {
	plugin, _ := getTestResources()

	nwCfg := cni.NetworkConfig{
		Name:              "test-nwcfg",
		Type:              "azure-vnet",
		Mode:              "bridge",
		Master:            "eth0",
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

	args := &cniSkel.CmdArgs{
		ContainerID: "test-container",
		Netns:       "test-container",
	}
	args.StdinData = nwCfg.Serialize()
	podEnv := cni.K8SPodEnvArgs{
		K8S_POD_NAME:      "test-pod",
		K8S_POD_NAMESPACE: "test-pod-namespace",
	}
	args.Args = fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podEnv.K8S_POD_NAME, podEnv.K8S_POD_NAMESPACE)
	args.IfName = "azure0"

	plugin.Get(args)
}

func TestGetAllEndpointState(t *testing.T) {
	plugin, mockNetworkManager := getTestResources()
	networkid := "azure"

	ep1 := getTestEndpoint("podname1", "podnamespace1", "10.0.0.1/24", "podinterfaceid1", "testcontainerid1")
	ep2 := getTestEndpoint("podname2", "podnamespace2", "10.0.0.2/24", "podinterfaceid2", "testcontainerid2")

	err := mockNetworkManager.CreateEndpoint(networkid, ep1)
	require.NoError(t, err)

	err = mockNetworkManager.CreateEndpoint(networkid, ep2)
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
	plugin, _ := getTestResources()
	networkid := "azure"
	state, err := plugin.GetAllEndpointState(networkid)
	require.NoError(t, err)
	require.Equal(t, 0, len(state.ContainerInterfaces))
}
