package cns

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalPodInfo(t *testing.T) {
	marshalledKubernetesPodInfo, _ := json.Marshal(KubernetesPodInfo{PodName: "pod", PodNamespace: "namespace"})
	tests := []struct {
		name    string
		b       []byte
		want    *podInfo
		wantErr bool
	}{
		{
			name: "orchestrator context",
			b:    []byte(`{"PodName":"pod","PodNamespace":"namespace"}`),
			want: &podInfo{
				KubernetesPodInfo: KubernetesPodInfo{
					PodName:      "pod",
					PodNamespace: "namespace",
				},
			},
		},
		{
			name: "marshalled orchestrator context",
			b:    marshalledKubernetesPodInfo,
			want: &podInfo{
				KubernetesPodInfo: KubernetesPodInfo{
					PodName:      "pod",
					PodNamespace: "namespace",
				},
			},
		},
		{
			name:    "malformed",
			b:       []byte(`{{}`),
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalPodInfo(tt.b)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPodInfoFromIPConfigRequest(t *testing.T) {
	GlobalPodInfoScheme = InterfaceIDPodInfoScheme
	defer func() { GlobalPodInfoScheme = KubernetesPodInfoScheme }()
	tests := []struct {
		name    string
		req     IPConfigRequest
		want    PodInfo
		wantErr bool
	}{
		{
			name: "full req",
			req: IPConfigRequest{
				PodInterfaceID:      "abcdef-eth0",
				InfraContainerID:    "abcdef",
				OrchestratorContext: []byte(`{"PodName":"pod","PodNamespace":"namespace"}`),
			},
			want: &podInfo{
				KubernetesPodInfo: KubernetesPodInfo{
					PodName:      "pod",
					PodNamespace: "namespace",
				},
				PodInterfaceID:      "abcdef-eth0",
				PodInfraContainerID: "abcdef",
				Version:             InterfaceIDPodInfoScheme,
			},
		},
		{
			name: "empty interface id",
			req: IPConfigRequest{
				InfraContainerID:    "abcdef",
				OrchestratorContext: []byte(`{"PodName":"pod","PodNamespace":"namespace"}`),
			},
			want:    &podInfo{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPodInfoFromIPConfigRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
