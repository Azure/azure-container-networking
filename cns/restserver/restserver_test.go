package restserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/Azure/azure-container-networking/nmagent"
)

const (
	hostPrimaryIP = "10.0.0.4"
	hostSubnet    = "10.0.0.0/24"
)

type mockIMDSCtxKey string

const simulateError mockIMDSCtxKey = "simulate-error"

type wireserverClientFake struct{}

func (c *wireserverClientFake) GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error) {
	return &wireserver.GetInterfacesResult{
		Interface: []wireserver.Interface{
			{
				IsPrimary: true,
				IPSubnet: []wireserver.Subnet{
					{
						Prefix: hostSubnet,
						IPAddress: []wireserver.Address{
							{
								Address:   hostPrimaryIP,
								IsPrimary: true,
							},
						},
					},
				},
			},
		},
	}, nil
}

type mockIMDSClient struct{}

func newMockIMDSClient() *mockIMDSClient {
	return &mockIMDSClient{}
}

func (m *mockIMDSClient) GetVMUniqueID(ctx context.Context) (string, error) {
	if ctx.Value(simulateError) != nil {
		return "", imds.ErrUnexpectedStatusCode
	}

	return "55b8499d-9b42-4f85-843f-24ff69f4a643", nil
}

func (m *mockIMDSClient) GetNetworkInterfaces(ctx context.Context) ([]imds.NetworkInterface, error) {
	if ctx.Value(simulateError) != nil {
		return nil, imds.ErrUnexpectedStatusCode
	}

	macAddr1, _ := net.ParseMAC("00:15:5d:01:02:01")
	macAddr2, _ := net.ParseMAC("00:15:5d:01:02:02")

	return []imds.NetworkInterface{
		{
			InterfaceCompartmentID: "nc1",
			MacAddress:             imds.HardwareAddr(macAddr1),
		},
		{
			InterfaceCompartmentID: "nc2",
			MacAddress:             imds.HardwareAddr(macAddr2),
		},
	}, nil
}

func (m *mockIMDSClient) GetIMDSVersions(ctx context.Context) (*imds.APIVersionsResponse, error) {
	if ctx.Value(simulateError) != nil {
		return nil, imds.ErrUnexpectedStatusCode
	}

	return &imds.APIVersionsResponse{
		APIVersions: []string{
			"2017-03-01",
			"2021-01-01",
			"2025-07-24",
		},
	}, nil
}

type nmaClientFake struct {
	SupportedAPIsF      func(context.Context) ([]string, error)
	GetNCVersionListF   func(context.Context) (nmagent.NCVersionList, error)
	GetHomeAzF          func(context.Context) (nmagent.AzResponse, error)
	GetInterfaceIPInfoF func(ctx context.Context) (nmagent.Interfaces, error)
}

func (n *nmaClientFake) SupportedAPIs(ctx context.Context) ([]string, error) {
	return n.SupportedAPIsF(ctx)
}

func (n *nmaClientFake) GetNCVersionList(ctx context.Context) (nmagent.NCVersionList, error) {
	return n.GetNCVersionListF(ctx)
}

func (n *nmaClientFake) GetHomeAz(ctx context.Context) (nmagent.AzResponse, error) {
	return n.GetHomeAzF(ctx)
}

func (n *nmaClientFake) GetInterfaceIPInfo(ctx context.Context) (nmagent.Interfaces, error) {
	return n.GetInterfaceIPInfoF(ctx)
}

type wireserverProxyFake struct {
	JoinNetworkFunc func(context.Context, string) (*http.Response, error)
	PublishNCFunc   func(context.Context, cns.NetworkContainerParameters, []byte) (*http.Response, error)
	UnpublishNCFunc func(context.Context, cns.NetworkContainerParameters, []byte) (*http.Response, error)
}

const defaultResponseBody = `{"httpStatusCode":"200"}`

func defaultResponse() *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(bytes.NewBufferString(defaultResponseBody)),
		ContentLength: int64(len(defaultResponseBody)),
	}
}

func (w *wireserverProxyFake) JoinNetwork(ctx context.Context, vnetID string) (*http.Response, error) {
	if w.JoinNetworkFunc != nil {
		return w.JoinNetworkFunc(ctx, vnetID)
	}

	return defaultResponse(), nil
}

func (w *wireserverProxyFake) PublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error) {
	if w.PublishNCFunc != nil {
		return w.PublishNCFunc(ctx, ncParams, payload)
	}

	return defaultResponse(), nil
}

func (w *wireserverProxyFake) UnpublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error) {
	if w.UnpublishNCFunc != nil {
		return w.UnpublishNCFunc(ctx, ncParams, payload)
	}

	return defaultResponse(), nil
}

func setMockNMAgent(h *HTTPRestService, m *nmaClientFake) func() {
	// this is a hack that exists because the tests are too DRY, so the setup
	// logic has ossified in TestMain

	// save the previous value of the NMAgent so that it can be restored by the
	// cleanup function
	prev := h.nma

	// set the NMAgent to what was requested
	h.nma = m

	// return a cleanup function that will restore NMAgent back to what it was
	return func() {
		h.nma = prev
	}
}

func setWireserverProxy(h *HTTPRestService, w *wireserverProxyFake) func() {
	prev := h.wsproxy
	h.wsproxy = w
	return func() {
		h.wsproxy = prev
	}
}
