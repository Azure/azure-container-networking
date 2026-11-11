package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
)

var errMissingSupportedAPIs = errors.New("cns/client: response is missing supported APIs")

// SupportedAPIs returns the curated capabilities advertised by CNS.
func (c *Client) SupportedAPIs(ctx context.Context) ([]string, error) {
	u := c.routes[cns.SupportedAPIsPath]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "building http request")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "sending http request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &FailedHTTPRequest{Code: resp.StatusCode}
	}

	var out cns.SupportedAPIsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "decoding response body")
	}
	if out.SupportedAPIs == nil {
		return nil, errMissingSupportedAPIs
	}
	return out.SupportedAPIs, nil
}
