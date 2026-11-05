//go:build unit
// +build unit

package client

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	ver "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	utilexec "k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"
)

func TestGetState(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"/opt/cni/bin/azure-vnet"}, Stdout: `{"ContainerInterfaces":{"3f813b02-eth0":{"PodName":"metrics-server-77c8679d7d-6ksdh","PodNamespace":"kube-system","PodEndpointID":"3f813b02-eth0","ContainerID":"3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46","IPAddresses":[{"IP":"10.241.0.17","Mask":"//8AAA=="}]},"6e688597-eth0":{"PodName":"tunnelfront-5d96f9b987-65xbn","PodNamespace":"kube-system","PodEndpointID":"6e688597-eth0","ContainerID":"6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed","IPAddresses":[{"IP":"10.241.0.13","Mask":"//8AAA=="}]}}}`},
	}

	fakeexec := testutils.GetFakeExecWithScripts(calls)

	c := New(fakeexec)
	state, err := c.GetEndpointState()
	require.NoError(t, err)

	res := &api.AzureCNIState{
		ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			"3f813b02-eth0": testGetPodNetworkInterfaceInfo("3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system", "3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "10.241.0.17/16"),
			"6e688597-eth0": testGetPodNetworkInterfaceInfo("6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system", "6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "10.241.0.13/16"),
		},
	}

	require.Equal(t, res, state)
}

func TestGetStateOutputStreams(t *testing.T) {
	const response = `{"ContainerInterfaces":{"endpoint":{"PodName":"pod"}}}`
	const diagnostic = "can't rename log file: azure-vnet.log is being used by another process"
	exitErr := &fakeexec.FakeExitError{Status: 1}
	tests := []struct {
		name    string
		stdout  string
		stderr  string
		execErr error
		wantErr bool
	}{
		{name: "valid state with diagnostics", stdout: response, stderr: diagnostic},
		{name: "execution failure with valid state", stdout: response, stderr: diagnostic, execErr: exitErr, wantErr: true},
		{name: "invalid state with diagnostics", stdout: "invalid state", stderr: diagnostic, wantErr: true},
		{name: "state on stderr only", stderr: response, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			oldLogger := logger
			logger = zap.New(core)
			t.Cleanup(func() { logger = oldLogger })

			cmd := &fakeexec.FakeCmd{
				RunScript: []fakeexec.FakeAction{
					func() ([]byte, []byte, error) { return []byte(tt.stdout), []byte(tt.stderr), tt.execErr },
				},
				CombinedOutputScript: []fakeexec.FakeAction{
					func() ([]byte, []byte, error) { return []byte(tt.stdout + tt.stderr), nil, tt.execErr },
				},
			}
			exec := &fakeexec.FakeExec{
				CommandScript: []fakeexec.FakeCommandAction{
					func(command string, args ...string) utilexec.Cmd {
						return fakeexec.InitFakeCmd(cmd, command, args...)
					},
				},
			}

			state, err := New(exec).GetEndpointState()
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, state)
				require.ErrorContains(t, err, "stdout: ["+tt.stdout+"]")
				require.ErrorContains(t, err, "stderr: ["+tt.stderr+"]")
				if tt.execErr != nil {
					require.ErrorIs(t, err, tt.execErr)
				} else {
					var syntaxErr *json.SyntaxError
					require.ErrorAs(t, err, &syntaxErr)
				}
				require.Zero(t, logs.Len())
				return
			}
			require.NoError(t, err)
			require.Equal(t, &api.AzureCNIState{
				ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
					"endpoint": {PodName: "pod"},
				},
			}, state)
			require.Equal(t, 1, logs.Len())
			require.Equal(t, tt.stderr, logs.All()[0].ContextMap()["stderr"])
		})
	}
}

func TestGetVersion(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"/opt/cni/bin/azure-vnet", "-v"}, Stdout: `Azure CNI Version v1.4.0-2-g984c5a5e-dirty`},
	}

	fakeexec := testutils.GetFakeExecWithScripts(calls)

	c := New(fakeexec)
	version, err := c.GetVersion()
	require.NoError(t, err)

	expectedVersion, err := ver.NewVersion("v1.4.0-2-g984c5a5e-dirty")
	require.NoError(t, err)

	require.Equal(t, expectedVersion, version)
}
