package restserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAreNCsPresent(t *testing.T) {
	tests := []struct {
		name    string
		service HTTPRestService
		want    bool
	}{
		{
			name: "container status present",
			service: HTTPRestService{
				state: &httpRestServiceState{
					ContainerStatus: map[string]containerstatus{
						"nc1": {},
					},
				},
			},
			want: true,
		},
		{
			name: "containerIDByOrchestorContext present",
			service: HTTPRestService{
				state: &httpRestServiceState{
					ContainerIDByOrchestratorContext: map[string]string{
						"nc1": "present",
					},
				},
			},
			want: true,
		},
		{
			name: "neither containerStatus nor containerIDByOrchestratorContext present",
			service: HTTPRestService{
				state: &httpRestServiceState{},
			},
			want: false,
		},
	}
	for _, tt := range tests { //nolint:govet // this mutex copy is to keep a local reference to this variable in the test func closure, and is ok
		tt := tt //nolint:govet // this mutex copy is to keep a local reference to this variable in the test func closure, and is ok
		t.Run(tt.name, func(t *testing.T) {
			got := tt.service.areNCsPresent()
			assert.Equal(t, got, tt.want)
		})
	}
}
