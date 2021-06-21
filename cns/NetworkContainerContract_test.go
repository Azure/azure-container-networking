package cns

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalPodInfo(t *testing.T) {
	marshalledKubernetesPodInfo, _ := json.Marshal(KubernetesPodInfo{PodName: "pod", PodNamespace: "namespace"})
	tests := []struct {
		name string
		b    []byte
		want *podInfo
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := UnmarshalPodInfo(tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}
