// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package cni

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/platform"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilexec "k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"
)

func TestTranslateEndpointState(t *testing.T) {
	valid := func() *api.AzureCNIState {
		return &api.AzureCNIState{ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
			"if-b": {
				PodName:       "pod",
				PodNamespace:  "ns",
				PodEndpointId: "if-b",
				ContainerID:   "container",
				IfName:        "eth1",
				IPAddresses: []net.IPNet{
					mustIPNet(t, "2001:db8::4/64"),
					mustIPNet(t, "10.0.1.4/24"),
				},
			},
			"if-a": {
				PodName:       "pod",
				PodNamespace:  "ns",
				PodEndpointId: "if-a",
				ContainerID:   "container",
				IfName:        "eth0",
				IPAddresses:   []net.IPNet{mustIPNet(t, "10.0.0.4/24")},
			},
		}}
	}

	records, err := translateEndpointState(valid())
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "if-a", records[0].InterfaceKey)
	assert.Equal(t, "if-b", records[1].InterfaceKey)
	assert.Equal(t, "10.0.1.4", records[1].IPAddresses[0].IP.String())
	assert.Equal(t, "2001:db8::4", records[1].IPAddresses[1].IP.String())
	ones, bits := records[1].IPAddresses[0].Mask.Size()
	assert.Equal(t, 24, ones)
	assert.Equal(t, 32, bits)

	second, err := translateEndpointState(valid())
	require.NoError(t, err)
	assert.Equal(t, records, second)

	tests := []struct {
		name   string
		mutate func(*api.AzureCNIState)
	}{
		{name: "nil state", mutate: func(state *api.AzureCNIState) { *state = api.AzureCNIState{} }},
		{name: "empty container", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-a"]
			value.ContainerID = ""
			state.ContainerInterfaces["if-a"] = value
		}},
		{name: "empty endpoint", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-a"]
			value.PodEndpointId = ""
			state.ContainerInterfaces["if-a"] = value
		}},
		{name: "malformed IP", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-a"]
			value.IPAddresses[0].IP = net.IP{1, 2}
			state.ContainerInterfaces["if-a"] = value
		}},
		{name: "malformed prefix", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-a"]
			value.IPAddresses[0].Mask = net.CIDRMask(64, 128)
			state.ContainerInterfaces["if-a"] = value
		}},
		{name: "duplicate IP", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-b"]
			value.IPAddresses = append(value.IPAddresses, mustIPNet(t, "10.0.0.4/32"))
			state.ContainerInterfaces["if-b"] = value
		}},
		{name: "duplicate endpoint", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-b"]
			value.PodEndpointId = "if-a"
			state.ContainerInterfaces["if-b"] = value
		}},
		{name: "duplicate interface", mutate: func(state *api.AzureCNIState) {
			value := state.ContainerInterfaces["if-b"]
			value.IfName = "eth0"
			state.ContainerInterfaces["if-b"] = value
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := valid()
			tt.mutate(state)
			if tt.name == "nil state" {
				state = nil
			}
			_, err := translateEndpointState(state)
			require.Error(t, err)
		})
	}
}

func TestEndpointStateProviderContextAndRepeatedReads(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd: []string{platform.CNIBinaryPath},
			Stdout: `{"ContainerInterfaces":{"if-a":{"PodName":"pod","PodNamespace":"ns",
				"PodEndpointID":"if-a","ContainerID":"container","IfName":"eth0",
				"IPAddresses":[{"IP":"10.0.0.4","Mask":"////AA=="}]}}}`,
		},
		{
			Cmd: []string{platform.CNIBinaryPath},
			Stdout: `{"ContainerInterfaces":{"if-a":{"PodName":"pod","PodNamespace":"ns",
				"PodEndpointID":"if-a","ContainerID":"container","IfName":"eth0",
				"IPAddresses":[{"IP":"10.0.0.5","Mask":"////AA=="}]}}}`,
		},
	}
	exec := testutils.GetFakeExecWithScripts(calls)
	provider := endpointStateProvider(exec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, exec.CommandCalls)

	first, err := provider(context.Background())
	require.NoError(t, err)
	second, err := provider(context.Background())
	require.NoError(t, err)
	require.Equal(t, "10.0.0.4", first[0].IPAddresses[0].IP.String())
	require.Equal(t, "10.0.0.5", second[0].IPAddresses[0].IP.String())
	require.Equal(t, 2, exec.CommandCalls)

	_, err = provider(nil)
	require.Error(t, err)
	assert.False(t, errors.Is(err, cns.ErrDuplicateIP))

	t.Run("canceled after non-cancelable exec", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		exec := &fakeexec.FakeExec{
			ExactOrder: true,
			CommandScript: []fakeexec.FakeCommandAction{
				func(cmd string, args ...string) utilexec.Cmd {
					command := &fakeexec.FakeCmd{
						CombinedOutputScript: []fakeexec.FakeAction{
							func() ([]byte, []byte, error) {
								cancel()
								return []byte(`{"ContainerInterfaces":{}}`), nil, nil
							},
						},
					}
					return fakeexec.InitFakeCmd(command, cmd, args...)
				},
			},
		}
		_, err := endpointStateProvider(exec)(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func mustIPNet(t *testing.T, value string) net.IPNet {
	t.Helper()
	ip, prefix, err := net.ParseCIDR(value)
	require.NoError(t, err)
	prefix.IP = ip
	return *prefix
}
