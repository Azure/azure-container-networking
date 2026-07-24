package cni

import (
	"errors"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/platform"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/exec"
)

func newCNIStateFakeExec(stdout string) exec.Interface {
	calls := []testutils.TestCmd{
		{Cmd: []string{platform.CNIBinaryPath}, Stdout: stdout},
	}

	fake := testutils.GetFakeExecWithScripts(calls)
	return fake
}

func TestNewCNIPodInfoProvider(t *testing.T) {
	tests := []struct {
		name    string
		exec    exec.Interface
		want    map[string]cns.PodInfo
		wantErr bool
	}{
		{
			name: "good",
			exec: newCNIStateFakeExec(
				`{"ContainerInterfaces":{"3f813b02-eth0":{"PodName":"metrics-server-77c8679d7d-6ksdh","IfName":"eth0",
				"PodNamespace":"kube-system","PodEndpointID":"3f813b02-eth0",
				"ContainerID":"3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46",
				"IPAddresses":[{"IP":"10.241.0.17","Mask":"//8AAA=="}]},
				"6e688597-eth0":{"PodName":"tunnelfront-5d96f9b987-65xbn","IfName":"eth0","PodNamespace":"kube-system",
				"PodEndpointID":"6e688597-eth0","ContainerID":"6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed",
				"IPAddresses":[{"IP":"10.241.0.13","Mask":"//8AAA=="}]}}}`,
			),
			want: map[string]cns.PodInfo{
				"10.241.0.13": cns.NewPodInfo("6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system"),
				"10.241.0.17": cns.NewPodInfo("3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system"),
			},
			wantErr: false,
		},
		{
			name: "dualstack",
			exec: newCNIStateFakeExec(
				`{"ContainerInterfaces":{"3f813b02-eth0":{"PodName":"metrics-server-77c8679d7d-6ksdh","IfName":"eth0",
				"PodNamespace":"kube-system","PodEndpointID":"3f813b02-eth0",
				"ContainerID":"3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46",
				"IPAddresses":[{"IP":"10.241.0.17","Mask":"//8AAA=="},{"IP":"2001:0db8:abcd:0015::0","Mask":"//8AAA=="}]},
				"6e688597-eth0":{"PodName":"tunnelfront-5d96f9b987-65xbn","IfName":"eth0","PodNamespace":"kube-system",
				"PodEndpointID":"6e688597-eth0","ContainerID":"6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed",
				"IPAddresses":[{"IP":"10.241.0.13","Mask":"//8AAA=="},{"IP":"2001:0db8:abcd:0014::0","Mask":"//8AAA=="}]}}}`,
			),
			want: map[string]cns.PodInfo{
				"2001:db8:abcd:15::": cns.NewPodInfo("3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system"),
				"2001:db8:abcd:14::": cns.NewPodInfo("6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system"),
				"10.241.0.17":        cns.NewPodInfo("3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system"),
				"10.241.0.13":        cns.NewPodInfo("6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system"),
			},
			wantErr: false,
		},
		{
			name: "empty CNI response",
			exec: newCNIStateFakeExec(
				`{}`,
			),
			want:    map[string]cns.PodInfo{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := podInfoProvider(tt.exec)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			podInfoByIP, _ := got.PodInfoByIP()
			assert.Equal(t, tt.want, podInfoByIP)
		})
	}
}

func TestCNIStateToPodInfoByIPLegacyTolerance(t *testing.T) {
	state := &api.AzureCNIState{ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
		"healthy": {
			PodName:       "pod",
			PodNamespace:  "namespace",
			PodEndpointId: "interface",
			ContainerID:   "container",
			IPAddresses: []net.IPNet{{
				IP:   net.ParseIP("10.0.0.4"),
				Mask: net.CIDRMask(24, 32),
			}},
		},
		"zero-ip-degenerate": {},
		"mismatched-mask-degenerate": {
			IPAddresses: []net.IPNet{{
				IP:   net.ParseIP("2001:db8::4"),
				Mask: net.CIDRMask(24, 32),
			}},
		},
	}}

	got, err := cniStateToPodInfoByIP(state)
	assert.NoError(t, err)
	assert.Equal(t, map[string]cns.PodInfo{
		"10.0.0.4":    cns.NewPodInfo("container", "interface", "pod", "namespace"),
		"2001:db8::4": cns.NewPodInfo("", "", "", ""),
	}, got)
}

func TestCNIStateToPodInfoByIPDuplicateStillErrors(t *testing.T) {
	duplicate := net.IPNet{IP: net.ParseIP("10.0.0.4"), Mask: net.CIDRMask(24, 32)}
	state := &api.AzureCNIState{ContainerInterfaces: map[string]api.PodNetworkInterfaceInfo{
		"first": {
			PodName:       "pod-a",
			PodNamespace:  "namespace",
			PodEndpointId: "interface-a",
			ContainerID:   "container-a",
			IPAddresses:   []net.IPNet{duplicate},
		},
		"second": {
			PodName:       "pod-b",
			PodNamespace:  "namespace",
			PodEndpointId: "interface-b",
			ContainerID:   "container-b",
			IPAddresses:   []net.IPNet{duplicate},
		},
	}}

	got, err := cniStateToPodInfoByIP(state)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, cns.ErrDuplicateIP))
}
