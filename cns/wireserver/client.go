package wireserver

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
)

const (
	WireserverIP = "168.63.129.16"
)

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
	Host string
	Port uint16

	HTTPClient do
	Logger     interface {
		Printf(string, ...any)
	}
}

func (c *Client) hostport() string {
	if c.Port != 0 {
		port := strconv.FormatUint(uint64(c.Port), 10)
		return net.JoinHostPort(c.Host, port)
	}
	return c.Host
}

// GetInterfaces queries interfaces from the wireserver.
func (c *Client) GetInterfaces(ctx context.Context) (*GetInterfacesResult, error) {
	c.Logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromHost")

	q := &url.Values{}
	q.Add("comp", "nmagent")
	q.Add("type", "getinterfaceinfov1")

	reqURL := &url.URL{
		Scheme:   "http",
		Host:     c.hostport(),
		Path:     "/machine/plugins",
		RawQuery: q.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
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

	c.Logger.Printf("[Azure CNS] Response received from NMAgent for get interface details: %s", string(b))

	var res GetInterfacesResult
	if err := xml.NewDecoder(bytes.NewReader(b)).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "failed to decode response body")
	}
	return &res, nil
}
