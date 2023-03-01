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
	publishNCURLFmt   = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1`
	unpublishNCURLFmt = `http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1/method/DELETE`
)

type Proxy struct {
	Host       string
	HTTPClient do
}

func (p *Proxy) JoinNetwork(ctx context.Context, vnetID string) (*http.Response, error) {
	reqURL := fmt.Sprintf(joinNetworkURLFmt, p.Host, vnetID)

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

func (p *Proxy) PublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error) {
	reqURL := fmt.Sprintf(publishNCURLFmt, p.Host, ncParams.AssociatedInterfaceID, ncParams.NCID, ncParams.AuthToken)

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

func (p *Proxy) UnpublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error) {
	reqURL := fmt.Sprintf(unpublishNCURLFmt, p.Host, ncParams.AssociatedInterfaceID, ncParams.NCID, ncParams.AuthToken)

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
