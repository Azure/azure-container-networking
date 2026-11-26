package v2

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	acncommon "github.com/Azure/azure-container-networking/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestStartServices will test three scenarios:
// 1. when customer provides -p option, make sure local server is running and server is using this port
// 2. when customer provides -c option, local server will this -c URL

func TestStartServerWithCNSPort(t *testing.T) {
	var err error

	logger.InitLogger("testlogs", 0, 0, "./")
	cnsPort := "8000"

	// Create the service with -p 8000
	if err = startService(cnsPort, ""); err != nil {
		t.Fatalf("Failed to connect to CNS Service on expected port:%s. Error: %v", cnsPort, err)
	}
}

func TestStartServerWithCNSURL(t *testing.T) {
	var err error

	logger.InitLogger("testlogs", 0, 0, "./")

	// Create the service with -c "localhost:8000"
	cnsURL := "tcp://localhost:8500"
	if err = startService("", cnsURL); err != nil {
		t.Fatalf("Failed to connect to CNS Service by this cns url:%s. Error: %v", cnsURL, err)
	}
}

// startService will return a URL that running server is using and check if sever can start
// mock primaryVMIP as a fixed IP
func startService(cnsPort, cnsURL string) error {
	// Create the service.
	config := common.ServiceConfig{}

	nmagentClient := &fakes.NMAgentClientFake{}
	service, err := restserver.NewHTTPRestService(&config, &fakes.WireserverClientFake{},
		&fakes.WireserverProxyFake{}, &restserver.IPtablesProvider{}, nmagentClient, nil, nil, nil,
		fakes.NewMockIMDSClient())
	if err != nil {
		return errors.Wrap(err, "Failed to initialize service")
	}

	if service != nil {
		service.Name = "cns-test-server"

		service.SetOption(acncommon.OptCnsPort, cnsPort)
		service.SetOption(acncommon.OptCnsURL, cnsURL)

		config.Server.PrimaryInterfaceIP = "localhost"

		err = service.Init(&config)
		if err != nil {
			logger.Errorf("Failed to Init CNS, err:%v.\n", err)
			return errors.Wrap(err, "Failed to Init CNS")
		}

		err = service.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return errors.Wrap(err, "Failed to Start CNS")
		}
	}

	if cnsPort == "" {
		cnsPort = "10090"
	}
	// check if we can reach this URL
	urls := "localhost:" + cnsPort

	if cnsURL != "" {
		u, _ := url.Parse(cnsURL)
		port := u.Port()
		urls = "localhost:" + port
	}

	_, err = net.DialTimeout("tcp", urls, 10*time.Millisecond)
	if err != nil {
		return errors.Wrapf(err, "Failed to check reachability to urls %+v", urls)
	}

	service.Stop()

	return nil
}

func TestServerOmitsTCPIPAMRoutesWhenUnixSocketEnabled(t *testing.T) {
	if logger.Log == nil {
		log, err := logger.New("testlogs", 0, 0, t.TempDir()+"/") //nolint:staticcheck // existing handlers require the global logger
		require.NoError(t, err)
		logger.Log = log
	}
	config := common.ServiceConfig{IPAMSocket: true}
	service, err := restserver.NewHTTPRestService(&config, &fakes.WireserverClientFake{}, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	service.SetOption(acncommon.OptCnsURL, "tcp://127.0.0.1:0")
	service.SetOption(acncommon.OptCnsPort, "")
	require.NoError(t, service.Init(&config))

	server := httptest.NewServer(New(service).newEcho())
	t.Cleanup(server.Close)
	for _, route := range []string{cns.RequestIPConfig, cns.RequestIPConfigs, cns.ReleaseIPConfig, cns.ReleaseIPConfigs} {
		t.Run(route, func(t *testing.T) {
			req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+route, bytes.NewBufferString("{}"))
			require.NoError(t, requestErr)
			response, responseErr := server.Client().Do(req)
			require.NoError(t, responseErr)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	}
}
