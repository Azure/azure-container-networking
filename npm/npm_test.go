package npm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	controllersv2 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v2"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/debug"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNPMCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	dp.EXPECT().GetAllIPSets()

	npmMgr := NetworkPolicyManager{
		config: npmconfig.Config{
			Toggles: npmconfig.Toggles{
				EnableV2NPM: true,
			},
		},
		AzureConfig: models.AzureConfig{
			NodeName: "TestNode",
		},
		K8SControllersV2: models.K8SControllersV2{
			NamespaceControllerV2: &controllersv2.NamespaceController{},
		},
		Dataplane: dp,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != api.NPMMgrPath {
			t.Errorf("Expected to request '/fixedvalue', got: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		encoder := &npmMgr
		b, err := json.Marshal(encoder)
		require.NoError(t, err)
		_, err = w.Write(b)
		require.NoError(t, err)
	}))
	defer server.Close()

	host := strings.Split(server.URL[7:], ":")
	hostip := host[0]

	c := &debug.Converter{
		NPMDebugEndpointHost: fmt.Sprintf("http://%s", hostip),
		NPMDebugEndpointPort: host[1],
		EnableV2NPM:          true,
	}
	require.NoError(t, c.InitConverter())
}
