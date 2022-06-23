package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo"
	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/require"
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
	nothappyPodArgs = "K8S_POD_NAMESPACE=testns;K8S_POD_NAME=testname;K8S_POD_INFRA_CONTAINER_ID=testid;break=break" // this will break the pod config parsing
)

type scenario struct {
	name    string
	args    *cniSkel.CmdArgs
	wantErr bool
}

// build args for tests
func buildArgs(containerID, args string, stdin []byte) *cniSkel.CmdArgs {
	return &cniSkel.CmdArgs{
		ContainerID: containerID,
		Netns:       "testnetns",
		IfName:      "testifname",
		Args:        args,
		StdinData:   stdin,
	}
}

func TestCmdAdd(t *testing.T) {
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

	tests := []scenario{
		{
			name:    "Happy CNI add",
			args:    buildArgs("happyArgs", happyPodArgs, happyNetConfByteArr),
			wantErr: false,
		},
		{
			name:    "Fail create CNS request during CmdAdd",
			args:    buildArgs("failCreateCNSReqArgs", nothappyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
		{
			name:    "Fail request CNS ipconfig during CmdAdd",
			args:    buildArgs("failRequestCNSArgs", happyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
		{
			name:    "Fail process CNS response during CmdAdd",
			args:    buildArgs("failProcessCNSResp", happyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
		{
			name:    "Fail parse netconf during CmdAdd",
			args:    buildArgs("failParseNetConf", happyPodArgs, invalidNetConf),
			wantErr: true,
		},
		{
			name:    "Fail get versioned result during CmdAdd",
			args:    buildArgs("failGetVersionedResult", happyPodArgs, invalidVersionNetConfByteArr),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("test : %v\n", tt.name)
			mockCNSClient := &MockCNSClient{}
			buildinfo.BuildEnv = 2 // test env flag
			logger, cleanup, err := NewLogger(Env(buildinfo.BuildEnv))
			if err != nil {
				fmt.Println(err)
				return
			}
			defer cleanup(logger) // nolint
			ipamPlugin, _ := NewPlugin(logger, mockCNSClient)
			err = ipamPlugin.CmdAdd(tt.args)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCmdDel(t *testing.T) {
	happyNetConf := &cniTypes.NetConf{
		CNIVersion: "0.1.0",
		Name:       "happynetconf",
	}

	happyNetConfByteArr, err := json.Marshal(happyNetConf)
	if err != nil {
		panic(err)
	}

	tests := []scenario{
		{
			name:    "Happy CNI del",
			args:    buildArgs("happyArgs", happyPodArgs, happyNetConfByteArr),
			wantErr: false,
		},
		{
			name:    "Fail create CNS request during CmdDel",
			args:    buildArgs("failCreateCNSReqArgs", nothappyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
		{
			name:    "Fail request CNS release IP during CmdDel",
			args:    buildArgs("failRequestCNSReleaseIPArgs", happyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("test : %v\n", tt.name)
			mockCNSClient := &MockCNSClient{}
			buildinfo.BuildEnv = 2
			logger, cleanup, err := NewLogger(Env(buildinfo.BuildEnv))
			if err != nil {
				return
			}
			defer cleanup(logger) // nolint
			ipamPlugin, _ := NewPlugin(logger, mockCNSClient)
			err = ipamPlugin.CmdDel(tt.args)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCmdCheck(t *testing.T) {
	fmt.Println("test : cmdCheck")
	mockCNSClient := &MockCNSClient{}
	buildinfo.BuildEnv = 2
	logger, cleanup, err := NewLogger(Env(buildinfo.BuildEnv))
	if err != nil {
		return
	}
	defer cleanup(logger) // nolint
	ipamPlugin, _ := NewPlugin(logger, mockCNSClient)
	err = ipamPlugin.CmdCheck(nil)
	require.NoError(t, err)
}
