//+build linux

package network

import (
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/stretchr/testify/require"
)

func TestAddEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		client     *TransparentEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoints",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints netlink fail",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
		},
		{
			name: "Add endpoints get interface fail for old veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints get interface fail for primary interface",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":eth0",
		},
		{
			name: "Add endpoints get interface fail for host veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(true, 3),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":azvcontainer",
		},
		{
			name: "get interface fail for container veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(true, 4),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":azvhost",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.AddEndpoints(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, tt.wantErrMsg, err.Error(), "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
