package kubecontroller

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

const (
	ncID             = "160005ba-cd02-11ea-87d0-0242ac130003"
	ipCIDR           = "10.0.0.1/32"
	ipCIDRString     = "10.0.0.1"
	ipCIDRMaskLength = 32
	ipNotCIDR        = "10.0.0.1"
	ipMalformed      = "10.0.0.0.0"
	defaultGateway   = "10.0.0.2"
	subnetID         = "subnet1"
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
						IP:   ipCIDR,
					},
				},
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
				PrimaryIP: ipCIDR,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipMalformed,
					},
				},
			},
		},
	}

	// Test with malformed ip assignment
	_, err = CRDStatusToNCRequest(status)

	if err == nil {
		t.Fatalf("Expected translation of CRD status with malformed ip assignment to fail.")
	}
}

func TestStatusToNCRequestPrimaryIPNotCIDR(t *testing.T) {
	var (
		status nnc.NodeNetworkConfigStatus
		err    error
	)

	status = nnc.NodeNetworkConfigStatus{
		NetworkContainers: []nnc.NetworkContainer{
			{
				PrimaryIP: ipNotCIDR,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipCIDR,
					},
				},
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
				PrimaryIP: ipCIDR,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipNotCIDR,
					},
				},
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
				PrimaryIP: ipCIDR,
				ID:        ncID,
				IPAssignments: []nnc.IPAssignment{
					{
						Name: allocatedUUID,
						IP:   ipCIDR,
					},
				},
				SubnetID:       subnetID,
				DefaultGateway: defaultGateway,
				Netmask:        "", // Not currently set by DNC Request Controller
			},
		},
	}

	// Test with ips formed correctly as CIDRs
	ncRequest, err = CRDStatusToNCRequest(status)

	if err != nil {
		t.Fatalf("Expected translation of CRD status to succeed, got error :%v", err)
	}

	if ncRequest.IPConfiguration.IPSubnet.IPAddress != ipCIDRString {
		t.Fatalf("Expected ncRequest's ipconfiguration to have the ip %v but got %v", ipCIDRString, ncRequest.IPConfiguration.IPSubnet.IPAddress)
	}

	if ncRequest.IPConfiguration.IPSubnet.PrefixLength != uint8(ipCIDRMaskLength) {
		t.Fatalf("Expected ncRequest's ipconfiguration prefix length to be %v but got %v", ipCIDRMaskLength, ncRequest.IPConfiguration.IPSubnet.PrefixLength)
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

	if secondaryIP.IPSubnet.IPAddress != ipCIDRString {
		t.Fatalf("Expected %v as the secondary IP config but got %v", ipCIDRString, secondaryIP.IPSubnet.IPAddress)
	}

	if secondaryIP.IPSubnet.PrefixLength != ipCIDRMaskLength {
		t.Fatalf("Expected %v as the prefix length for the secondary IP config but got %v", ipCIDRMaskLength, secondaryIP.IPSubnet.PrefixLength)
	}
}

func TestSecondaryIPsToCRDSpecNilMap(t *testing.T) {
	var (
		secondaryIPs map[string]cns.SecondaryIPConfig
		ipCount      int
		err          error
	)

	ipCount = 10

	// Test with nil secondaryIPs map
	_, err = CNSToCRDSpec(secondaryIPs, ipCount)

	if err == nil {
		t.Fatalf("Expected error when converting nil map of secondary IPs into crd spec")
	}
}

func TestSecondaryIPsToCRDSpecSuccess(t *testing.T) {
	var (
		secondaryIPs map[string]cns.SecondaryIPConfig
		spec         nnc.NodeNetworkConfigSpec
		ipCount      int
		err          error
	)

	ipCount = 10

	secondaryIPs = map[string]cns.SecondaryIPConfig{
		allocatedUUID: {
			IPSubnet: cns.IPSubnet{
				IPAddress:    ipCIDRString,
				PrefixLength: ipCIDRMaskLength,
			},
		},
	}

	// Test with secondary ip with ip and mask length correct
	spec, err = CNSToCRDSpec(secondaryIPs, ipCount)

	if err != nil {
		t.Fatalf("Expected no error when converting secondary ips into crd spec but got %v", err)
	}

	if len(spec.IPsNotInUse) != 1 {
		t.Fatalf("Expected crd spec's IPsNotInUse to have length 1, but has length %v", len(spec.IPsNotInUse))
	}

	if spec.IPsNotInUse[0] != allocatedUUID {
		t.Fatalf("Expected crd's spec to contain UUID %v but got %v", allocatedUUID, spec.IPsNotInUse[0])
	}

	if spec.RequestedIPCount != int64(ipCount) {
		t.Fatalf("Expected crd's spec RequestedIPCount to be equal to ipCount %v but got %v", ipCount, spec.RequestedIPCount)
	}
}
