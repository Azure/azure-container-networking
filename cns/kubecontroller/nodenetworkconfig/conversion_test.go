package nodenetworkconfig

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
)

const (
	uuid                        = "539970a2-c2dd-11ea-b3de-0242ac130004"
	defaultGateway              = "10.0.0.2"
	ipIsCIDR                    = "10.0.0.1/32"
	ipMalformed                 = "10.0.0.0.0"
	ncID                        = "160005ba-cd02-11ea-87d0-0242ac130003"
	primaryIP                   = "10.0.0.1"
	overlayPrimaryIP            = "10.0.0.1/30"
	subnetAddressSpace          = "10.0.0.0/24"
	subnetName                  = "subnet1"
	subnetPrefixLen             = 24
	testSecIP                   = "10.0.0.2"
	version                     = 1
	nodeIP                      = "10.1.0.5"
	vnetBlockPrimaryIP          = "10.224.0.4"
	vnetBlockPrimaryIPPrefix    = "10.224.0.4/30"
	vnetBlockSubnetAddressSpace = "10.224.0.0/14"
	vnetBlockSubnetPrefixLen    = 14
	vnetBlockNodeIP             = "10.228.0.6"
	vnetBlockDefaultGateway     = "10.224.0.1"
	vnetBlockCIDR1              = "10.224.0.8/30"
	vnetBlockCIDR2              = "10.224.0.12/30"
)

var invalidStatusMultiNC = v1alpha.NodeNetworkConfigStatus{
	NetworkContainers: []v1alpha.NetworkContainer{
		{},
		{},
	},
}

var validSwiftNC = v1alpha.NetworkContainer{
	ID:             ncID,
	AssignmentMode: v1alpha.Dynamic,
	Type:           v1alpha.VNET,
	PrimaryIP:      primaryIP,
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
	NodeIP:             nodeIP,
}

var validSwiftStatus = v1alpha.NodeNetworkConfigStatus{
	NetworkContainers: []v1alpha.NetworkContainer{
		validSwiftNC,
	},
}

var validSwiftRequest = &cns.CreateNetworkContainerRequest{
	HostPrimaryIP: nodeIP,
	Version:       strconv.FormatInt(version, 10),
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

var validOverlayNC = v1alpha.NetworkContainer{
	ID:                 ncID,
	AssignmentMode:     v1alpha.Static,
	Type:               v1alpha.Overlay,
	PrimaryIP:          overlayPrimaryIP,
	NodeIP:             nodeIP,
	SubnetName:         subnetName,
	SubnetAddressSpace: subnetAddressSpace,
	Version:            version,
}

var validVNETBlockNC = v1alpha.NetworkContainer{
	ID:             ncID,
	AssignmentMode: v1alpha.Static,
	Type:           v1alpha.VNETBlock,
	IPAssignments: []v1alpha.IPAssignment{
		{
			Name: uuid,
			IP:   vnetBlockCIDR1,
		},
		{
			Name: uuid,
			IP:   vnetBlockCIDR2,
		},
	},
	NodeIP:             vnetBlockNodeIP,
	PrimaryIP:          vnetBlockPrimaryIPPrefix,
	SubnetName:         subnetName,
	SubnetAddressSpace: vnetBlockSubnetAddressSpace,
	DefaultGateway:     vnetBlockDefaultGateway,
	Version:            version,
}

func TestCreateNCRequestFromDynamicNC(t *testing.T) {
	tests := []struct {
		name    string
		input   v1alpha.NetworkContainer
		want    *cns.CreateNetworkContainerRequest
		wantErr bool
	}{
		{
			name:    "valid swift",
			input:   validSwiftNC,
			wantErr: false,
			want:    validSwiftRequest,
		},
		{
			name: "malformed primary IP",
			input: v1alpha.NetworkContainer{
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

			wantErr: true,
		},
		{
			name: "malformed IP assignment",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
		{
			name: "IP is CIDR",
			input: v1alpha.NetworkContainer{
				PrimaryIP: ipIsCIDR,
				ID:        ncID,
				NodeIP:    nodeIP,
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
			wantErr: false,
			want:    validSwiftRequest,
		},
		{
			name: "IP assignment is CIDR",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
		{
			name: "address space is not CIDR",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateNCRequestFromDynamicNC(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}

func TestCreateNCRequestFromStaticNC(t *testing.T) {
	tests := []struct {
		name    string
		input   v1alpha.NetworkContainer
		want    *cns.CreateNetworkContainerRequest
		wantErr bool
	}{
		{
			name:    "valid overlay",
			input:   validOverlayNC,
			wantErr: false,
			want:    validOverlayRequest,
		},
		{
			name: "malformed primary IP",
			input: v1alpha.NetworkContainer{
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

			wantErr: true,
		},
		{
			name: "malformed IP assignment",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
		{
			name: "IP assignment is CIDR",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
		{
			name: "address space is not CIDR",
			input: v1alpha.NetworkContainer{
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
			wantErr: true,
		},
		// VNET Block test cases
		{
			name:    "valid VNET Block",
			input:   validVNETBlockNC,
			wantErr: false,
			want:    validVNETBlockRequest,
		},
		{
			name: "PrimaryIP is not CIDR",
			input: v1alpha.NetworkContainer{
				AssignmentMode:     v1alpha.Static,
				Type:               v1alpha.VNETBlock,
				PrimaryIP:          vnetBlockPrimaryIP,
				ID:                 ncID,
				SubnetAddressSpace: "10.224.0.0/14",
			},
			wantErr: true,
		},
		{
			name: "IP assignment is not CIDR",
			input: v1alpha.NetworkContainer{
				AssignmentMode: v1alpha.Static,
				Type:           v1alpha.VNETBlock,
				PrimaryIP:      vnetBlockPrimaryIPPrefix,
				ID:             ncID,
				IPAssignments: []v1alpha.IPAssignment{
					{
						Name: uuid,
						IP:   "10.224.0.4",
					},
				},
				SubnetAddressSpace: "10.224.0.0/14",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateNCRequestFromStaticNC(tt.input, false)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
