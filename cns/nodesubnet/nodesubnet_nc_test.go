package nodesubnet_test

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/go-cmp/cmp"
)

func TestCreateNodeSubnetNCRequest_EmptySecondaryIPs(t *testing.T) {
	secondaryIPs := []string{}
	expectedRequest := &cns.CreateNetworkContainerRequest{
		HostPrimaryIP:        nodesubnet.NodeSubnetHostPrimaryIP,
		SecondaryIPConfigs:   map[string]cns.SecondaryIPConfig{},
		NetworkContainerid:   nodesubnet.NodeSubnetNCID,
		NetworkContainerType: cns.Docker,
		Version:              "0",
		IPConfiguration:      cns.IPConfiguration{},
		NCStatus:             v1alpha.NCUpdateSuccess,
	}

	request := nodesubnet.CreateNodeSubnetNCRequest(secondaryIPs)
	if !cmp.Equal(request, expectedRequest) {
		t.Errorf("Unexepected diff in NodeSubnetNCRequest: %v", cmp.Diff(request, expectedRequest))
	}
}

func TestCreateNodeSubnetNCRequest_NonEmptySecondaryIPs(t *testing.T) {
	secondaryIPs := []string{"10.0.0.1", "10.0.0.2"}
	expectedRequest := &cns.CreateNetworkContainerRequest{
		HostPrimaryIP: nodesubnet.NodeSubnetHostPrimaryIP,
		SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
			"10.0.0.1": {IPAddress: "10.0.0.1", NCVersion: nodesubnet.NodeSubnetNCVersion},
			"10.0.0.2": {IPAddress: "10.0.0.2", NCVersion: nodesubnet.NodeSubnetNCVersion},
		},
		NetworkContainerid:   nodesubnet.NodeSubnetNCID,
		NetworkContainerType: cns.Docker,
		Version:              "0",
		IPConfiguration:      cns.IPConfiguration{},
		NCStatus:             v1alpha.NCUpdateSuccess,
	}

	request := nodesubnet.CreateNodeSubnetNCRequest(secondaryIPs)
	if !cmp.Equal(request, expectedRequest) {
		t.Errorf("Unexepected diff in NodeSubnetNCRequest: %v", cmp.Diff(request, expectedRequest))
	}
}

func init() {
	logger.InitLogger("testlogs", 0, 0, "./")
}
