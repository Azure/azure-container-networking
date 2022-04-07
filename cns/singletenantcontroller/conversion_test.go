package kubecontroller

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
)

const (
	uuid               = "539970a2-c2dd-11ea-b3de-0242ac130004"
	defaultGateway     = "10.0.0.2"
	ipIsCIDR           = "10.0.0.1/32"
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
		input   v1alpha.NodeNetworkConfigStatus
		want    cns.CreateNetworkContainerRequest
		wantErr bool
	}{
		{
			name:    "valid",
			input:   validStatus,
			wantErr: false,
			want:    validRequest,
		},
		{
			name:    "no nc",
			input:   v1alpha.NodeNetworkConfigStatus{},
			wantErr: false,
			want:    cns.CreateNetworkContainerRequest{},
		},
		{
			name:    ">1 nc",
			input:   invalidStatusMultiNC,
			wantErr: true,
		},
		{
			name: "malformed primary IP",
			input: v1alpha.NodeNetworkConfigStatus{
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
			input: v1alpha.NodeNetworkConfigStatus{
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
			input: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: ipIsCIDR,
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
			},
			wantErr: false,
			want:    validRequest,
		},
		{
			name: "IP assignment is CIDR",
			input: v1alpha.NodeNetworkConfigStatus{
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						PrimaryIP: primaryIP,
						ID:        ncID,
						IPAssignments: []v1alpha.IPAssignment{
							{
								Name: uuid,
								IP:   ipIsCIDR,
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
			input: v1alpha.NodeNetworkConfigStatus{
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := CRDStatusToNCRequest(&tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
