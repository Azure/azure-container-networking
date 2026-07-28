package wireserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
)

const (
	joinNetworkURLFmt = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/joinedVirtualNetworks/%s/api-version/1`
	joinSubnetURLFmt  = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/joinedVirtualNetworks/%s/joinedSubnets/%s/authenticationToken/%s/api-version/1?useLegacyChannel=false`
	publishNCURLFmt   = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1`
	unpublishNCURLFmt = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1/method/DELETE`
)

type Proxy struct {
	Host       string
	HTTPClient do
}

func (p *Proxy) JoinNetwork(ctx context.Context, vnetID string, useRNCPublisher bool) (*http.Response, error) {
	var joinNetworkURLFormat string
	if useRNCPublisher {
		joinNetworkURLFormat = joinNetworkURLFmt + "?useLegacyChannel=false"
	} else {
		joinNetworkURLFormat = joinNetworkURLFmt
	}
	reqURL := fmt.Sprintf(joinNetworkURLFormat, p.Host, vnetID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBufferString(`""`))
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: join network: could not build http request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: join network: could not perform http request")
	}

	return resp, nil
}

func (p *Proxy) JoinSubnet(ctx context.Context, vnetID, subnetName string, ncParams cns.NetworkContainerParameters) (*http.Response, error) {
	reqURL := fmt.Sprintf(joinSubnetURLFmt, p.Host, vnetID, subnetName, ncParams.AuthToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBufferString(`""`))
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: join subnet: could not build http request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: join subnet: could not perform http request")
	}

	return resp, nil
}

func (p *Proxy) PublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte, useRNCPublisher bool) (*http.Response, error) {
	var publishNCURLFormat string
	if useRNCPublisher {
		publishNCURLFormat = publishNCURLFmt + "?useLegacyChannel=false"
	} else {
		publishNCURLFormat = publishNCURLFmt
	}
	reqURL := fmt.Sprintf(publishNCURLFormat, p.Host, ncParams.AssociatedInterfaceID, ncParams.NCID, ncParams.AuthToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: publish nc: could not build http request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: publish nc: could not perform http request")
	}

	return resp, nil
}

func (p *Proxy) UnpublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte, useRNCPublisher bool) (*http.Response, error) {
	var unpublishNCURLFormat string
	if useRNCPublisher {
		unpublishNCURLFormat = unpublishNCURLFmt + "?useLegacyChannel=false"
	} else {
		unpublishNCURLFormat = unpublishNCURLFmt
	}
	reqURL := fmt.Sprintf(unpublishNCURLFormat, p.Host, ncParams.AssociatedInterfaceID, ncParams.NCID, ncParams.AuthToken)

	// a POST to wireserver must contain a body. For legacy purposes,
	// an empty json string (two quote characters) should be sent by default.
	body := []byte(`""`)
	if len(payload) > 0 {
		body = payload
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: unpublish nc: could not build http request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "wireserver proxy: unpublish nc: could not perform http request")
	}

	return resp, nil
}
