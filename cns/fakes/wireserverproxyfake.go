package fakes

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
)

type WireserverProxyFake struct {
	JoinNetworkFunc func(context.Context, string, bool) (*http.Response, error)
	JoinSubnetFunc  func(context.Context, string, string, cns.NetworkContainerParameters) (*http.Response, error)
	PublishNCFunc   func(context.Context, cns.NetworkContainerParameters, []byte, bool) (*http.Response, error)
	UnpublishNCFunc func(context.Context, cns.NetworkContainerParameters, []byte, bool) (*http.Response, error)
}

const defaultResponseBody = `{"httpStatusCode":"200"}`

func defaultResponse() *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(bytes.NewBufferString(defaultResponseBody)),
		ContentLength: int64(len(defaultResponseBody)),
	}
}

func (w *WireserverProxyFake) JoinNetwork(ctx context.Context, vnetID string, useRNCPublisher bool) (*http.Response, error) {
	if w.JoinNetworkFunc != nil {
		return w.JoinNetworkFunc(ctx, vnetID, useRNCPublisher)
	}

	return defaultResponse(), nil
}

func (w *WireserverProxyFake) JoinSubnet(ctx context.Context, vnetID, subnetName string, ncParams cns.NetworkContainerParameters) (*http.Response, error) {
	if w.JoinSubnetFunc != nil {
		return w.JoinSubnetFunc(ctx, vnetID, subnetName, ncParams)
	}

	return defaultResponse(), nil
}

func (w *WireserverProxyFake) PublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte, useRNCPublisher bool) (*http.Response, error) {
	if w.PublishNCFunc != nil {
		return w.PublishNCFunc(ctx, ncParams, payload, useRNCPublisher)
	}

	return defaultResponse(), nil
}

func (w *WireserverProxyFake) UnpublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte, useRNCPublisher bool) (*http.Response, error) {
	if w.UnpublishNCFunc != nil {
		return w.UnpublishNCFunc(ctx, ncParams, payload, useRNCPublisher)
	}

	return defaultResponse(), nil
}
