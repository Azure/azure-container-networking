package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/azure-ipam/logger"
	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var (
	errFoo                   = errors.New("err")
	loggerCfg *logger.Config = &logger.Config{}
)

// MOckCNSClient is a mock implementation of the CNSClient interface
type MockCNSClient struct{}

func (c *MockCNSClient) RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	switch ipconfig.InfraContainerID {
	case "failRequestCNSArgs":
		return nil, errFoo
	case "failProcessCNSResp":
		result := &cns.IPConfigResponse{
			PodIpInfo: cns.PodIpInfo{
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "10.0.1.10.2", // invalid ip address
						PrefixLength: 24,
					},
				}
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
			PodIpInfo: []cns.PodIpInfo{
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "10.0.1.10",
						PrefixLength: 24,
					},
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

func (c *MockCNSClient) RequestIPs(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigsResponse, error) {
	switch ipconfig.InfraContainerID {
	case "failRequestCNSArgs":
		return nil, errFoo
	case "failProcessCNSResp":
		result := &cns.IPConfigsResponse{
			PodIpInfo: []cns.PodIpInfo{
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "10.0.1.10.2", // invalid ip address
						PrefixLength: 24,
					},

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
			PodIpInfo: []cns.PodIpInfo{
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "10.0.1.10",
						PrefixLength: 24,
					},
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

func (c *MockCNSClient) ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigRequest) error {
	switch ipconfig.InfraContainerID {
	case "failRequestCNSReleaseIPArgs":
		return errFoo
	default:
		return nil
	}
}

// cniResultsWriter is a helper struct to write CNI results to a byte array
type cniResultsWriter struct {
	result *types100.Result
}

func (w *cniResultsWriter) Write(data []byte) (int, error) {
	err := json.Unmarshal(data, &w.result)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to unmarshal CNI result")
	}
	return len(data), nil
}

const (
	happyPodArgs = "K8S_POD_NAMESPACE=testns;K8S_POD_NAME=testname;K8S_POD_INFRA_CONTAINER_ID=testid"
)

type scenario struct {
	name    string
	args    *cniSkel.CmdArgs
	want    *types100.Result
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
		CNIVersion: "1.0.0",
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
			name: "Happy CNI add",
			args: buildArgs("happyArgs", happyPodArgs, happyNetConfByteArr),
			want: &types100.Result{
				CNIVersion: "1.0.0",
				IPs: []*types100.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.IPv4(10, 0, 1, 10),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				DNS: cniTypes.DNS{},
			},
			wantErr: false,
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
		writer := &cniResultsWriter{}
		t.Run(tt.name, func(t *testing.T) {
			mockCNSClient := &MockCNSClient{}
			testLogger, cleanup, err := logger.New(loggerCfg)
			if err != nil {
				fmt.Println(err)
				return
			}
			defer cleanup()
			ipamPlugin, _ := NewPlugin(testLogger, mockCNSClient, writer)
			err = ipamPlugin.CmdAdd(tt.args)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tt.want != nil {
				require.Equal(t, tt.want, writer.result)
			}
		})
	}
}

func TestCmdDel(t *testing.T) {
	happyNetConf := &cniTypes.NetConf{
		CNIVersion: "1.0.0",
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
			name:    "Fail request CNS release IP during CmdDel",
			args:    buildArgs("failRequestCNSReleaseIPArgs", happyPodArgs, happyNetConfByteArr),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mockCNSClient := &MockCNSClient{}
			testLogger, cleanup, err := logger.New(loggerCfg)
			if err != nil {
				return
			}
			defer cleanup()
			ipamPlugin, _ := NewPlugin(testLogger, mockCNSClient, nil)
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
	mockCNSClient := &MockCNSClient{}
	testLogger, cleanup, err := logger.New(loggerCfg)
	if err != nil {
		return
	}
	defer cleanup()
	ipamPlugin, _ := NewPlugin(testLogger, mockCNSClient, nil)
	err = ipamPlugin.CmdCheck(nil)
	require.NoError(t, err)
}
