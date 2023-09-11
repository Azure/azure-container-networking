package nmagent

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/nmagent/internal"
	"github.com/pkg/errors"
)

// NewClient returns an initialized Client using the provided configuration.
func NewClient(c Config) (*Client, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Wrap(err, "validating config")
	}

	client := &Client{
		httpClient: &http.Client{
			Transport: &internal.WireserverTransport{
				Transport: http.DefaultTransport,
			},
		},
		host:      c.Host,
		port:      c.Port,
		enableTLS: c.UseTLS,
		retrier: internal.Retrier{
			// nolint:gomnd // the base parameter is explained in the function
			Cooldown: internal.Exponential(1*time.Second, 2),
		},
	}

	return client, nil
}

// Client is an agent for exchanging information with NMAgent.
type Client struct {
	httpClient *http.Client

	// config
	host string
	port uint16

	enableTLS bool

	retrier interface {
		Do(context.Context, func() error) error
	}
}

// JoinNetwork joins a node to a customer's virtual network.
func (c *Client) JoinNetwork(ctx context.Context, jnr JoinNetworkRequest) error {
	req, err := c.buildRequest(ctx, jnr)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	err = c.retrier.Do(ctx, func() error {
		resp, err := c.httpClient.Do(req) // nolint:govet // the shadow is intentional
		if err != nil {
			return errors.Wrap(err, "executing request")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
		}
		return nil
	})

	return err // nolint:wrapcheck // wrapping this just introduces noise
}

// DeleteNetwork deletes a customer network and it's associated subnets.
func (c *Client) DeleteNetwork(ctx context.Context, dnr DeleteNetworkRequest) error {
	req, err := c.buildRequest(ctx, dnr)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req) // nolint:govet // the shadow is intentional
	if err != nil {
		return errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}
	return nil
}

// GetNetworkConfiguration retrieves the configuration of a customer's virtual
// network. Only subnets which have been delegated will be returned.
func (c *Client) GetNetworkConfiguration(ctx context.Context, gncr GetNetworkConfigRequest) (VirtualNetwork, error) {
	var out VirtualNetwork

	req, err := c.buildRequest(ctx, gncr)
	if err != nil {
		return out, errors.Wrap(err, "building request")
	}

	err = c.retrier.Do(ctx, func() error {
		resp, err := c.httpClient.Do(req) // nolint:govet // the shadow is intentional
		if err != nil {
			return errors.Wrap(err, "executing http request to")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
		}

		ct := resp.Header.Get(internal.HeaderContentType)
		if ct != internal.MimeJSON {
			return NewContentError(ct, resp.Body, resp.ContentLength)
		}

		err = json.NewDecoder(resp.Body).Decode(&out)
		if err != nil {
			return errors.Wrap(err, "decoding json response")
		}

		return nil
	})

	return out, err // nolint:wrapcheck // wrapping just introduces noise here
}

// GetNetworkContainerVersion gets the current goal state version of a Network
// Container. This method can be used to detect changes in a Network
// Container's state that would indicate that it requires re-programming. The
// request must originate from a VM network interface that has a Swift
// Provisioning OwningServiceInstanceId property. The authentication token must
// match the token on the subnet containing the Network Container address.
func (c *Client) GetNCVersion(ctx context.Context, ncvr NCVersionRequest) (NCVersion, error) {
	req, err := c.buildRequest(ctx, ncvr)
	if err != nil {
		return NCVersion{}, errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NCVersion{}, errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return NCVersion{}, die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}

	var out NCVersion
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return NCVersion{}, errors.Wrap(err, "decoding response")
	}

	return out, nil
}

// PutNetworkContainer applies a Network Container goal state and publishes it
// to PubSub.
func (c *Client) PutNetworkContainer(ctx context.Context, pncr *PutNetworkContainerRequest) error {
	req, err := c.buildRequest(ctx, pncr)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}
	return nil
}

// SupportedAPIs retrieves the capabilities of the nmagent running on
// the node. This is useful for detecting if GRE Keys are supported.
func (c *Client) SupportedAPIs(ctx context.Context) ([]string, error) {
	sar := &SupportedAPIsRequest{}
	req, err := c.buildRequest(ctx, sar)
	if err != nil {
		return nil, errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	var out SupportedAPIsResponseXML
	err = xml.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return nil, errors.Wrap(err, "decoding response")
	}

	return out.SupportedApis, nil
}

// DeleteNetworkContainer removes a Network Container, its associated IP
// addresses, and network policies from an interface.
func (c *Client) DeleteNetworkContainer(ctx context.Context, dcr DeleteContainerRequest) error {
	req, err := c.buildRequest(ctx, dcr)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}

	return nil
}

func (c *Client) GetNCVersionList(ctx context.Context) (NCVersionList, error) {
	req, err := c.buildRequest(ctx, &NCVersionListRequest{})
	if err != nil {
		return NCVersionList{}, errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NCVersionList{}, errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return NCVersionList{}, die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}

	var out NCVersionList
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return NCVersionList{}, errors.Wrap(err, "decoding response")
	}

	return out, nil
}

// GetHomeAz gets node's home az from nmagent
func (c *Client) GetHomeAz(ctx context.Context) (AzResponse, error) {
	getHomeAzRequest := &GetHomeAzRequest{}
	var homeAzResponse AzResponse
	req, err := c.buildRequest(ctx, getHomeAzRequest)
	if err != nil {
		return homeAzResponse, errors.Wrap(err, "building request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return homeAzResponse, errors.Wrap(err, "submitting request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return homeAzResponse, die(resp.StatusCode, resp.Header, resp.Body, req.URL.Path)
	}

	err = json.NewDecoder(resp.Body).Decode(&homeAzResponse)
	if err != nil {
		return homeAzResponse, errors.Wrap(err, "decoding response")
	}

	return homeAzResponse, nil
}

func die(code int, headers http.Header, body io.ReadCloser, path string) error {
	// nolint:errcheck // make a best effort to return whatever information we can
	// returning an error here without the code and source would
	// be less helpful
	bodyContent, _ := io.ReadAll(body)
	return Error{
		Code: code,
		// this is a little strange, but the conversion below is to avoid forcing
		// consumers to depend on an internal type (which they can't anyway)
		Source: internal.GetErrorSource(headers).String(),
		Body:   bodyContent,
		Path:   path,
	}
}

func (c *Client) hostPort() string {
	port := strconv.Itoa(int(c.port))
	//nolint:gomnd // 80 is commonly understood to be HTTP
	if c.port == 80 {
		return c.host
	}
	return net.JoinHostPort(c.host, port)
}

func (c *Client) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(err, "validating request")
	}

	fullURL := &url.URL{
		Scheme: c.scheme(),
		Host:   c.hostPort(),
		Path:   req.Path(),
	}

	body, err := req.Body()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving request body")
	}

	// nolint:wrapcheck // wrapping doesn't provide useful information
	return http.NewRequestWithContext(ctx, req.Method(), fullURL.String(), body)
}

func (c *Client) scheme() string {
	if c.enableTLS {
		return "https"
	}
	return "http"
}
