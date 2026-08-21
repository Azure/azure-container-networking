package v2

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acncommon "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
)

type wireserverClientFake struct{}

func (c *wireserverClientFake) GetInterfaces(_ context.Context) (*wireserver.GetInterfacesResult, error) {
	return &wireserver.GetInterfacesResult{
		Interface: []wireserver.Interface{
			{
				IsPrimary: true,
				IPSubnet: []wireserver.Subnet{
					{
						Prefix: "10.0.0.0/24",
						IPAddress: []wireserver.Address{
							{
								Address:   "10.0.0.4",
								IsPrimary: true,
							},
						},
					},
				},
			},
		},
	}, nil
}

type nmaClientFake struct{}

func (n *nmaClientFake) SupportedAPIs(_ context.Context) ([]string, error)              { return nil, nil }
func (n *nmaClientFake) GetNCVersionList(_ context.Context) (nmagent.NCVersionList, error) {
	return nmagent.NCVersionList{}, nil
}
func (n *nmaClientFake) GetHomeAz(_ context.Context) (nmagent.AzResponse, error) {
	return nmagent.AzResponse{}, nil
}
func (n *nmaClientFake) GetInterfaceIPInfo(_ context.Context) (nmagent.Interfaces, error) {
	return nmagent.Interfaces{}, nil
}

type wireserverProxyFake struct{}

func (w *wireserverProxyFake) JoinNetwork(_ context.Context, _ string) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(bytes.NewBufferString(`{"httpStatusCode":"200"}`)),
		ContentLength: int64(len(`{"httpStatusCode":"200"}`)),
	}, nil
}
func (w *wireserverProxyFake) PublishNC(_ context.Context, _ cns.NetworkContainerParameters, _ []byte) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(bytes.NewBufferString(`{"httpStatusCode":"200"}`)),
		ContentLength: int64(len(`{"httpStatusCode":"200"}`)),
	}, nil
}
func (w *wireserverProxyFake) UnpublishNC(_ context.Context, _ cns.NetworkContainerParameters, _ []byte) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(bytes.NewBufferString(`{"httpStatusCode":"200"}`)),
		ContentLength: int64(len(`{"httpStatusCode":"200"}`)),
	}, nil
}

type mockIMDSClient struct{}

func (m *mockIMDSClient) GetVMUniqueID(_ context.Context) (string, error) {
	return "55b8499d-9b42-4f85-843f-24ff69f4a643", nil
}

func (m *mockIMDSClient) GetNetworkInterfaces(_ context.Context) ([]imds.NetworkInterface, error) {
	return nil, nil
}

func (m *mockIMDSClient) GetIMDSVersions(_ context.Context) (*imds.APIVersionsResponse, error) {
	return &imds.APIVersionsResponse{APIVersions: []string{"2021-01-01"}}, nil
}

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

	nmagentClient := &nmaClientFake{}
	service, err := restserver.NewHTTPRestService(&config, &wireserverClientFake{},
		&wireserverProxyFake{}, &restserver.IPtablesProvider{}, nmagentClient, nil, nil, nil,
		&mockIMDSClient{})
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
