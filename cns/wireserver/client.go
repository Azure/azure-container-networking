package wireserver

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/pkg/errors"
)

const hostQueryURL = "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"

type GetNetworkContainerOpts struct {
	NetworkContainerID string
	PrimaryAddress     string
	AuthToken          string
	APIVersion         string
}

type do interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	HTTPClient do
}

// GetInterfaces queries interfaces from the wireserver.
func (c *Client) GetInterfaces(ctx context.Context) (*GetInterfacesResult, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromHost")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hostQueryURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to construct request")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	logger.Printf("[Azure CNS] Response received from NMAgent for get interface details: %s", string(b))

	var res GetInterfacesResult
	if err := xml.NewDecoder(bytes.NewReader(b)).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "failed to decode response body")
	}
	return &res, nil
}
