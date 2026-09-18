package restserver_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
)

func TestClientRequestIPsPreservesExistingAssignmentOnEndpointWriteFailure(t *testing.T) {
	service, err := restserver.NewHTTPRestService(
		&common.ServiceConfig{},
		&fakes.WireserverClientFake{},
		&fakes.WireserverProxyFake{},
		&restserver.IPtablesProvider{},
		&fakes.NMAgentClientFake{},
		store.NewMockStore(""),
		nil,
		nil,
		fakes.NewMockIMDSClient(),
	)
	require.NoError(t, err)
	service.SetOption(acn.OptManageEndpointState, true)
	service.SetNodeOrchestrator(&cns.SetOrchestratorTypeRequest{OrchestratorType: cns.KubernetesCRD})
	code := service.CreateOrUpdateNetworkContainerInternal(&cns.CreateNetworkContainerRequest{
		NetworkContainerid:   "nc",
		NetworkContainerType: cns.Docker,
		Version:              "-1",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet:         cns.IPSubnet{IPAddress: "10.0.0.4", PrefixLength: 24},
			GatewayIPAddress: "10.0.0.1",
		},
		SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
			"ip": {IPAddress: "10.0.0.5", NCVersion: -1},
		},
	})
	require.Equal(t, types.Success, code)
	var releaseCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc(cns.RequestIPConfigs, service.RequestIPConfigsHandler)
	mux.HandleFunc(cns.ReleaseIPConfigs, func(w http.ResponseWriter, r *http.Request) {
		releaseCalls.Add(1)
		service.ReleaseIPConfigsHandler(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := cnsclient.New(server.URL, time.Second)
	require.NoError(t, err)
	podInfo := cns.NewPodInfo("container-eth0", "container", "pod", "namespace")
	orchestratorContext, err := podInfo.OrchestratorContext()
	require.NoError(t, err)
	req := cns.IPConfigsRequest{
		PodInterfaceID:      podInfo.InterfaceID(),
		InfraContainerID:    podInfo.InfraContainerID(),
		OrchestratorContext: orchestratorContext,
		Ifname:              restserver.InfraInterfaceName,
	}
	_, err = client.RequestIPs(t.Context(), req)
	require.NoError(t, err)

	service.Lock()
	service.EndpointState = make(map[string]*restserver.EndpointInfo)
	service.EndpointStateStore = failedEndpointWriteStore{KeyValueStore: service.EndpointStateStore}
	service.Unlock()
	_, err = client.RequestIPs(t.Context(), req)
	require.ErrorContains(t, err, restserver.ErrEndpointStateUpdate.Error())
	require.Zero(t, releaseCalls.Load())
	service.RLock()
	defer service.RUnlock()
	ipState := service.PodIPConfigState["ip"]
	require.Equal(t, types.Assigned, ipState.GetState())
	require.Equal(t, []string{"ip"}, service.PodIPIDByPodInterfaceKey[podInfo.Key()])
}

type failedEndpointWriteStore struct {
	store.KeyValueStore
}

func (s failedEndpointWriteStore) Write(key string, value any) error {
	if key == restserver.EndpointStoreKey {
		return io.ErrShortWrite
	}
	if err := s.KeyValueStore.Write(key, value); err != nil {
		return fmt.Errorf("writing %q: %w", key, err)
	}
	return nil
}
