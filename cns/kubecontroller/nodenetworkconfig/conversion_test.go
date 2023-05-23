package nodenetworkconfig

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
)

const (
	uuid                   = "539970a2-c2dd-11ea-b3de-0242ac130004"
	defaultGateway         = "10.0.0.2"
	ipIsCIDR               = "10.0.0.1/32"
	ipMalformed            = "10.0.0.0.0"
	ncID                   = "160005ba-cd02-11ea-87d0-0242ac130003"
	primaryIP              = "10.0.0.1"
	overlayPrimaryIP       = "10.0.0.1/30"
	overlayPrimaryIPv6     = "fd04:d27:ea49::/126"
	subnetAddressSpace     = "10.0.0.0/24"
	subnetIPv6AddressSpace = "fd04:d27:ea49::/120"
	subnetName             = "subnet1"
	subnetPrefixLen        = 24
	subnetIPv6PrefixLen    = 120
	testSecIP              = "10.0.0.2"
	version                = 1
	nodeIP                 = "10.1.0.5"
	nodeIPv6               = "fd04:d27:ea49:ffff:ffff:ffff:ffff:ffff"
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

var validIPv6OverlayNC = v1alpha.NetworkContainer{
	ID:                 ncID,
	AssignmentMode:     v1alpha.Static,
	Type:               v1alpha.Overlay,
	PrimaryIP:          overlayPrimaryIPv6,
	NodeIP:             nodeIPv6,
	SubnetName:         subnetName,
	SubnetAddressSpace: subnetIPv6AddressSpace,
	Version:            version,
}

var validIPv6OverlayRequest = &cns.CreateNetworkContainerRequest{
	Version: strconv.FormatInt(version, 10),
	IPConfiguration: cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			PrefixLength: uint8(subnetIPv6PrefixLen),
			IPAddress:    strings.Split(overlayPrimaryIPv6, "/")[0],
		},
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		"fd04:d27:ea49::": {
			IPAddress: "fd04:d27:ea49::",
			NCVersion: version,
		},
		"fd04:d27:ea49::1": {
			IPAddress: "fd04:d27:ea49::1",
			NCVersion: version,
		},
		"fd04:d27:ea49::2": {
			IPAddress: "fd04:d27:ea49::2",
			NCVersion: version,
		},
		"fd04:d27:ea49::3": {
			IPAddress: "fd04:d27:ea49::3",
			NCVersion: version,
		},
	},
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
			name:    "valid IPv6 overlay",
			input:   validIPv6OverlayNC,
			wantErr: false,
			want:    validIPv6OverlayRequest,
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateNCRequestFromStaticNC(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
