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

func TestNewPodInfoFromIPConfigsRequest(t *testing.T) {
	GlobalPodInfoScheme = InterfaceIDPodInfoScheme
	defer func() { GlobalPodInfoScheme = KubernetesPodInfoScheme }()
	tests := []struct {
		name    string
		req     IPConfigsRequest
		want    PodInfo
		wantErr bool
	}{
		{
			name: "full req",
			req: IPConfigsRequest{
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
			req: IPConfigsRequest{
				InfraContainerID:    "abcdef",
				OrchestratorContext: []byte(`{"PodName":"pod","PodNamespace":"namespace"}`),
			},
			want:    &podInfo{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPodInfoFromIPConfigsRequest(tt.req)
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

func TestCreateNetworkContainerRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateNetworkContainerRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: CreateNetworkContainerRequest{
				NetworkContainerid: "f47ac10b-58cc-0372-8567-0e02b2c3d479",
			},
			wantErr: false,
		},
		{
			name: "valid",
			req: CreateNetworkContainerRequest{
				NetworkContainerid: SwiftPrefix + "f47ac10b-58cc-0372-8567-0e02b2c3d479",
			},
			wantErr: false,
		},
		{
			name: "invalid",
			req: CreateNetworkContainerRequest{
				NetworkContainerid: "-f47ac10b-58cc-0372-8567-0e02b2c3d479",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("CreateNetworkContainerRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPostNetworkContainersRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     PostNetworkContainersRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: PostNetworkContainersRequest{
				CreateNetworkContainerRequests: []CreateNetworkContainerRequest{
					{
						NetworkContainerid: "f47ac10b-58cc-0372-8567-0e02b2c3d479",
					},
					{
						NetworkContainerid:         "f47ac10b-58cc-0372-8567-0e02b2c3d478",
						PrimaryInterfaceIdentifier: "10.240.0.4",
						IPConfiguration: IPConfiguration{
							GatewayIPAddress: "10.0.0.1",
						},
					},
					{
						NetworkContainerid:         "a47ac10b-58cc-0372-8567-0e02b2c3d478",
						PrimaryInterfaceIdentifier: "10.240.0.4/24",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid",
			req: PostNetworkContainersRequest{
				CreateNetworkContainerRequests: []CreateNetworkContainerRequest{
					{
						NetworkContainerid: "f47ac10b-58cc-0372-8567-0e02b2c3d479",
					},
					{
						NetworkContainerid: SwiftPrefix + "f47ac10b-58cc-0372-8567-0e02b2c3d478",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid",
			req: PostNetworkContainersRequest{
				CreateNetworkContainerRequests: []CreateNetworkContainerRequest{
					{
						NetworkContainerid: "f47ac10b-58cc-0372-8567-0e02b2c3d479",
					},
					{
						NetworkContainerid: "-f47ac10b-58cc-0372-8567-0e02b2c3d478",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid",
			req: PostNetworkContainersRequest{
				CreateNetworkContainerRequests: []CreateNetworkContainerRequest{
					{
						NetworkContainerid:         "f47ac10b-58cc-0372-8567-0e02b2c3d478",
						PrimaryInterfaceIdentifier: "10.240.0.4",
						IPConfiguration: IPConfiguration{
							GatewayIPAddress: "10.0.0.1;",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid",
			req: PostNetworkContainersRequest{
				CreateNetworkContainerRequests: []CreateNetworkContainerRequest{
					{
						NetworkContainerid:         "f47ac10b-58cc-0372-8567-0e02b2c3d478",
						PrimaryInterfaceIdentifier: "-10.240.0.4",
						IPConfiguration: IPConfiguration{
							GatewayIPAddress: "10.0.0.1",
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("PostNetworkContainersRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
