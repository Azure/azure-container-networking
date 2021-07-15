package cnireconciler

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/exec"
)

func newCNIStateFakeExec(stdout string) exec.Interface {
	calls := []testutils.TestCmd{
		{Cmd: []string{"/opt/cni/bin/azure-vnet"}, Stdout: stdout},
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
				`{"ContainerInterfaces":{"3f813b02-eth0":{"PodName":"metrics-server-77c8679d7d-6ksdh","PodNamespace":"kube-system","PodEndpointID":"3f813b02-eth0","ContainerID":"3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46","IPAddresses":[{"IP":"10.241.0.17","Mask":"//8AAA=="}]},"6e688597-eth0":{"PodName":"tunnelfront-5d96f9b987-65xbn","PodNamespace":"kube-system","PodEndpointID":"6e688597-eth0","ContainerID":"6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed","IPAddresses":[{"IP":"10.241.0.13","Mask":"//8AAA=="}]}}}`,
			),
			want: map[string]cns.PodInfo{
				"10.241.0.13": cns.NewPodInfo("6e688597eafb97c83c84e402cc72b299bfb8aeb02021e4c99307a037352c0bed", "6e688597-eth0", "tunnelfront-5d96f9b987-65xbn", "kube-system"),
				"10.241.0.17": cns.NewPodInfo("3f813b029429b4e41a09ab33b6f6d365d2ed704017524c78d1d0dece33cdaf46", "3f813b02-eth0", "metrics-server-77c8679d7d-6ksdh", "kube-system"),
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
		t.Run(tt.name, func(t *testing.T) {
			got, err := newCNIPodInfoProvider(tt.exec)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got.PodInfoByIP())
		})
	}
}
