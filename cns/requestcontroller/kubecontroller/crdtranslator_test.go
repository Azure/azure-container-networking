package kubecontroller

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

const (
	ncID               = "160005ba-cd02-11ea-87d0-0242ac130003"
	primaryIp          = "10.0.0.1"
	ipInCIDR           = "10.0.0.1/32"
	ipMalformed        = "10.0.0.0.0"
	defaultGateway     = "10.0.0.2"
	subnetName         = "subnet1"
	subnetAddressSpace = "10.0.0.0/24"
	subnetPrefixLen    = 24
	testSecIp1         = "10.0.0.2"
	version            = 1
)

func TestStatusToNCRequestMalformedPrimaryIP(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: ipMalformed,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   testSecIp1,
					},
				},
				SubnetAddressSpace: subnetAddressSpace,
			},
		},
	}

	// Test with malformed primary ip
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with malformed ip to fail.")
	}
}

func TestStatusToNCRequestMalformedIPAssignment(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: primaryIp,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipMalformed,
					},
				},
				SubnetAddressSpace: subnetAddressSpace,
			},
		},
	}

	// Test with malformed ip assignment
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with malformed ip assignment to fail.")
	}
}

func TestStatusToNCRequestPrimaryIPInCIDR(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: ipInCIDR,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   testSecIp1,
					},
				},
				SubnetAddressSpace: subnetAddressSpace,
			},
		},
	}

	// Test with primary ip not in CIDR form
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with primary ip not CIDR, to fail.")
	}
}

func TestStatusToNCRequestIPAssignmentNotCIDR(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: primaryIp,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipInCIDR,
					},
				},
				SubnetAddressSpace: subnetAddressSpace,
			},
		},
	}

	// Test with ip assignment not in CIDR form
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with ip assignment not CIDR, to fail.")
	}
}

func TestStatusToNCRequestWithIncorrectSubnetAddressSpace(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: primaryIp,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   testSecIp1,
					},
				},
				SubnetAddressSpace: "10.0.0.0", // not a cidr range
			},
		},
	}

	// Test with ip assignment not in CIDR form
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with ip assignment not CIDR, to fail.")
	}
}

func TestStatusToNCRequestSuccess(t *testing.T) {
	var (
		status       nnc.NodeNetworkConfigStatus
		ncRequest    cns.CreateNetworkContainerRequest
		secondaryIPs map[string]cns.SecondaryIPConfig
		secondaryIP  cns.SecondaryIPConfig
		ok           bool
		err          error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: primaryIp,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   testSecIp1,
					},
				},
				SubnetName:         subnetName,
				DefaultGateway:     defaultGateway,
				SubnetAddressSpace: subnetAddressSpace,
				Version:            version,
			},
		},
	}

	// Test with ips formed correctly as CIDRs
	ncRequest, err = CRDStatusToNCRequest(status)

	if err != nil {
		t.Fatalf("Expected translation of CRD status to succeed, got error :%v", err)
	}

	if ncRequest.IPConfiguration.IPSubnet.IPAddress != primaryIp {
		t.Fatalf("Expected ncRequest's ipconfiguration to have the ip %v but got %v", primaryIp, ncRequest.IPConfiguration.IPSubnet.IPAddress)
	}

	if ncRequest.IPConfiguration.IPSubnet.PrefixLength != uint8(subnetPrefixLen) {
		t.Fatalf("Expected ncRequest's ipconfiguration prefix length to be %v but got %v", subnetPrefixLen, ncRequest.IPConfiguration.IPSubnet.PrefixLength)
	}

	if ncRequest.IPConfiguration.GatewayIPAddress != defaultGateway {
		t.Fatalf("Expected ncRequest's ipconfiguration gateway to be %s but got %s", defaultGateway, ncRequest.IPConfiguration.GatewayIPAddress)
	}

	if ncRequest.NetworkContainerid != ncID {
		t.Fatalf("Expected ncRequest's network container id to equal %v but got %v", ncID, ncRequest.NetworkContainerid)
	}

	if ncRequest.NetworkContainerType != cns.Docker {
		t.Fatalf("Expected ncRequest's network container type to be %v but got %v", cns.Docker, ncRequest.NetworkContainerType)
	}

	secondaryIPs = ncRequest.SecondaryIPConfigs

	if secondaryIP, ok = secondaryIPs[allocatedUUID]; !ok {
		t.Fatalf("Expected there to be a secondary ip with the key %v but found nothing", allocatedUUID)
	}

	if secondaryIP.IPAddress != testSecIp1 {
		t.Fatalf("Expected %v as the secondary IP config but got %v", testSecIp1, secondaryIP.IPAddress)
	}

	if secondaryIP.NCVersion != version {
		t.Fatalf("Expected %d as the secondary IP config NC version but got %v", version, secondaryIP.NCVersion)
	}
}
