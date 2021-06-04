package client

import (
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"
)

func testGetNetworkInterfaceInfo(podinterfaceid, podname, podnamespace, containerid, ipwithcidr string) api.NetworkInterfaceInfo {
	ip, ipnet, _ := net.ParseCIDR(ipwithcidr)
	ipnet.IP = ip
	return api.NetworkInterfaceInfo{
		PodName:        podname,
		PodNamespace:   podnamespace,
		PodInterfaceID: podinterfaceid,
		ContainerID:    containerid,
		IPAddresses: []net.IPNet{
			*ipnet,
		},
	}
}

func TestGetState(t *testing.T) {
	calls := []testutils.TestCmd{
		// not really stderr, going to change to stdout in #868
		{Cmd: []string{"./azure-vnet"}, Stdout: `{"ContainerInterfaces":{"podinterfaceid1":{"PodName":"podname1","PodNamespace":"podnamespace1","PodInterfaceID":"podinterfaceid1","ContainerID":"testcontainerid1","IPAddresses":[{"IP":"10.0.0.1","Mask":"////AA=="}]},"podinterfaceid2":{"PodName":"podname2","PodNamespace":"podnamespace2","PodInterfaceID":"podinterfaceid2","ContainerID":"testcontainerid2","IPAddresses":[{"IP":"10.0.0.2","Mask":"////AA=="}]}}}`},
	}

	//fakeexec, _ := testutils.GetFakeExecWithScripts(calls)
	fmt.Print(calls)
	fakeexec := exec.New()
	c := NewCNIClient(fakeexec)
	state, err := c.GetState()
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.NetworkInterfaceInfo{
			"podinterfaceid1": testGetNetworkInterfaceInfo("podinterfaceid1", "podname1", "podnamespace1", "testcontainerid1", "10.0.0.1/24"),
			"podinterfaceid2": testGetNetworkInterfaceInfo("podinterfaceid2", "podname2", "podnamespace2", "testcontainerid2", "10.0.0.2/24"),
		},
	}

	require.Exactly(t, res, state)
}
