package kubecontroller

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

const (
	uuid               = "539970a2-c2dd-11ea-b3de-0242ac130004"
	defaultGateway     = "10.0.0.2"
	ipInCIDR           = "10.0.0.1/32"
	ipMalformed        = "10.0.0.0.0"
	ncID               = "160005ba-cd02-11ea-87d0-0242ac130003"
	primaryIP          = "10.0.0.1"
	subnetAddressSpace = "10.0.0.0/24"
	subnetName         = "subnet1"
	subnetPrefixLen    = 24
	testSecIP          = "10.0.0.2"
	version            = 1
)

var invalidStatusMultiNC = v1alpha.NodeNetworkConfigStatus{
	NetworkContainers: []v1alpha.NetworkContainer{
		{},
		{},
	},
}

var validStatus = v1alpha.NodeNetworkConfigStatus{
	NetworkContainers: []v1alpha.NetworkContainer{
		{
			PrimaryIP: primaryIP,
			ID:        ncID,
			IPAssignments: []v1alpha.IPAssignment{
				{
					Name: uuid,
					IP:   testSecIP,
				},
			},
			SubnetName:         subnetName,
			DefaultGateway:     defaultGateway,
			SubnetAddressSpace: subnetAddressSpace,
			Version:            version,
		},
	},
	Scaler: v1alpha.Scaler{
		BatchSize: 1,
	},
}

var validRequest = cns.CreateNetworkContainerRequest{
	Version: strconv.FormatInt(version, 10),
	IPConfiguration: cns.IPConfiguration{
		GatewayIPAddress: defaultGateway,
		IPSubnet: cns.IPSubnet{
			PrefixLength: uint8(subnetPrefixLen),
			IPAddress:    primaryIP,
		},
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		uuid: {
			IPAddress: testSecIP,
			NCVersion: version,
		},
	},
}

func TestConvertNNCStatusToNCRequest(t *testing.T) {
	tests := []struct {
		name    string
		status  v1alpha.NodeNetworkConfigStatus
		ncreq   cns.CreateNetworkContainerRequest
		wantErr bool
	}{
		{
			name:    "no nc",
			status:  v1alpha.NodeNetworkConfigStatus{},
			wantErr: false,
			ncreq:   cns.CreateNetworkContainerRequest{},
		},
		{
			name:    ">1 nc",
			status:  invalidStatusMultiNC,
			wantErr: true,
		},
		{
			name: "malformed primary IP",
			status: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: ipMalformed,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   testSecIP,
							},
						},
						SubnetAddressSpace: subnetAddressSpace,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "malformed IP assignment",
			status: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: primaryIP,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   ipMalformed,
							},
						},
						SubnetAddressSpace: subnetAddressSpace,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "IP is CIDR",
			status: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: ipInCIDR,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   testSecIP,
							},
						},
						SubnetAddressSpace: subnetAddressSpace,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "IP assignment is CIDR",
			status: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: primaryIP,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   ipInCIDR,
							},
						},
						SubnetAddressSpace: subnetAddressSpace,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "address space is not CIDR",
			status: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: primaryIP,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   testSecIP,
							},
						},
						SubnetAddressSpace: "10.0.0.0", // not a cidr range
					},
				},
			},
			wantErr: true,
		},
		{
			name:    "valid",
			status:  validStatus,
			wantErr: false,
			ncreq:   validRequest,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := CRDStatusToNCRequest(&tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertNNCStatusToNCRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.ncreq) {
				t.Errorf("ConvertNNCStatusToNCRequest()\nhave: %+v\n want: %+v", got, tt.ncreq)
			}
		})
	}
}
