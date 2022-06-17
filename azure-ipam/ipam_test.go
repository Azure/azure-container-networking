package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type MockCNSClient struct{}

var errFoo = errors.New("err")

func (c *MockCNSClient) RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	switch ipconfig.InfraContainerID {
	case "failRequestCNSArgs":
		return nil, errFoo
	case "failProcessCNSResp":
		result := &cns.IPConfigResponse{
			PodIpInfo: cns.PodIpInfo{
				PodIPConfig: cns.IPSubnet{
					IPAddress:    "10.0.1.10.2",
					PrefixLength: 24,
				},
				NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.1.0",
						PrefixLength: 24,
					},
					DNSServers:       nil,
					GatewayIPAddress: "10.0.0.1",
				},
				HostPrimaryIPInfo: cns.HostIPInfo{
					Gateway:   "10.0.0.1",
					PrimaryIP: "10.0.0.1",
					Subnet:    "10.0.0.0/24",
				},
			},
			Response: cns.Response{
				ReturnCode: 0,
				Message:    "",
			},
		}
		return result, nil
	default:
		result := &cns.IPConfigResponse{
			PodIpInfo: cns.PodIpInfo{
				PodIPConfig: cns.IPSubnet{
					IPAddress:    "10.0.1.10",
					PrefixLength: 24,
				},
				NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.1.0",
						PrefixLength: 24,
					},
					DNSServers:       nil,
					GatewayIPAddress: "10.0.0.1",
				},
				HostPrimaryIPInfo: cns.HostIPInfo{
					Gateway:   "10.0.0.1",
					PrimaryIP: "10.0.0.1",
					Subnet:    "10.0.0.0/24",
				},
			},
			Response: cns.Response{
				ReturnCode: 0,
				Message:    "",
			},
		}
		return result, nil
	}
}

func (c *MockCNSClient) ReleaseIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) error {
	switch ipconfig.InfraContainerID {
	case "failRequestCNSReleaseIPArgs":
		return errFoo
	default:
		return nil
	}
}

const (
	happyPodArgs    = "K8S_POD_NAMESPACE=testns;K8S_POD_NAME=testname;K8S_POD_INFRA_CONTAINER_ID=testid"
	nothappyPodArgs = "K8S_POD_NAMESPACE=testns;K8S_POD_NAME=testname;K8S_POD_INFRA_CONTAINER_ID=testid;break=break"
)

type scenario struct {
	name    string
	args    *cniSkel.CmdArgs
	wantErr bool
}

func TestCmdAdd(t *testing.T) {
	req := require.New(t)

	happyNetConf := &cniTypes.NetConf{
		CNIVersion: "0.1.0",
		Name:       "happynetconf",
	}

	invalidVersionNetConf := &cniTypes.NetConf{
		CNIVersion: "0",
		Name:       "nothappynetconf",
	}

	happyNetConfByteArr, err := json.Marshal(happyNetConf)
	if err != nil {
		panic(err)
	}
	invalidVersionNetConfByteArr, err := json.Marshal(invalidVersionNetConf)
	if err != nil {
		panic(err)
	}
	invalidNetConf := []byte("invalidNetConf")

	happyArgs := &cniSkel.CmdArgs{
		ContainerID: "happyArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failCreateCNSReqArgs := &cniSkel.CmdArgs{
		ContainerID: "failCreateCNSReqArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        nothappyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failRequestCNSArgs := &cniSkel.CmdArgs{
		ContainerID: "failRequestCNSArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failProcessCNSResp := &cniSkel.CmdArgs{
		ContainerID: "failProcessCNSResp",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failParseNetConf := &cniSkel.CmdArgs{
		ContainerID: "failParseNetConf",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   invalidNetConf,
	}

	failGetVersionedResult := &cniSkel.CmdArgs{
		ContainerID: "failGetVersionedResult",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   invalidVersionNetConfByteArr,
	}

	tests := []scenario{
		{
			name:    "Happy CNI add",
			args:    happyArgs,
			wantErr: false,
		},
		{
			name:    "Fail create CNS request during CmdAdd",
			args:    failCreateCNSReqArgs,
			wantErr: true,
		},
		{
			name:    "Fail request CNS ipconfig during CmdAdd",
			args:    failRequestCNSArgs,
			wantErr: true,
		},
		{
			name:    "Fail process CNS response during CmdAdd",
			args:    failProcessCNSResp,
			wantErr: true,
		},
		{
			name:    "Fail parse netconf during CmdAdd",
			args:    failParseNetConf,
			wantErr: true,
		},
		{
			name:    "Fail get versioned result during CmdAdd",
			args:    failGetVersionedResult,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("test : %v\n", tt.name)
			mockCNSClient := &MockCNSClient{}
			logger, err := zap.NewProduction()
			defer logger.Sync() // nolint
			if err != nil {
				return
			}
			ipamPlugin, _ := NewPlugin(logger, mockCNSClient)
			err = ipamPlugin.CmdAdd(tt.args)
			fmt.Println()
			if tt.wantErr {
				req.Error(err)
			} else {
				req.NoError(err)
			}
		})
	}
}

func TestCmdDel(t *testing.T) {
	req := require.New(t)

	happyNetConf := &cniTypes.NetConf{
		CNIVersion: "0.1.0",
		Name:       "happynetconf",
	}

	happyNetConfByteArr, err := json.Marshal(happyNetConf)
	if err != nil {
		panic(err)
	}

	happyArgs := &cniSkel.CmdArgs{
		ContainerID: "happyArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failCreateCNSReqArgs := &cniSkel.CmdArgs{
		ContainerID: "failCreateCNSReqArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        nothappyPodArgs,
		StdinData:   happyNetConfByteArr,
	}

	failRequestCNSReleaseIPArgs := &cniSkel.CmdArgs{
		ContainerID: "failRequestCNSReleaseIPArgs",
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        happyPodArgs,
		StdinData:   happyNetConfByteArr,
	}
	tests := []scenario{
		{
			name:    "Happy CNI del",
			args:    happyArgs,
			wantErr: false,
		},
		{
			name:    "Fail create CNS request during CmdDel",
			args:    failCreateCNSReqArgs,
			wantErr: true,
		},
		{
			name:    "Fail request CNS release IP during CmdDel",
			args:    failRequestCNSReleaseIPArgs,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("test : %v\n", tt.name)
			mockCNSClient := &MockCNSClient{}
			logger, err := zap.NewProduction()
			defer logger.Sync() // nolint
			if err != nil {
				return
			}
			ipamPlugin, _ := NewPlugin(logger, mockCNSClient)
			err = ipamPlugin.CmdDel(tt.args)
			fmt.Println()
			if tt.wantErr {
				req.Error(err)
			} else {
				req.NoError(err)
			}
		})
	}
}

func TestCmdCheck(t *testing.T) {
}
