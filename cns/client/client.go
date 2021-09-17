package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
)

const (
	contentTypeJSON = "application/json"
	defaultBaseURL  = "http://localhost:10090"
	// DefaultTimeout default timeout duration for CNS Client.
	DefaultTimeout    = 5 * time.Second
	headerContentType = "Content-Type"
)

var clientPaths = []string{
	cns.GetNetworkContainerByOrchestratorContext,
	cns.CreateHostNCApipaEndpointPath,
	cns.DeleteHostNCApipaEndpointPath,
	cns.RequestIPConfig,
	cns.ReleaseIPConfig,
	cns.PathDebugIPAddresses,
	cns.PathDebugPodContext,
	cns.PathDebugRestData,
}

// Client specifies a client to connect to Ipam Plugin.
type Client struct {
	client http.Client
	routes map[string]url.URL
}

// New returns a new CNS client configured with the passed URL and timeout.
func New(baseURL string, requestTimeout time.Duration) (*Client, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	routes, err := buildRoutes(baseURL, clientPaths)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: http.Client{
			Timeout: requestTimeout,
		},
		routes: routes,
	}, nil
}

func buildRoutes(baseURL string, paths []string) (map[string]url.URL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse base URL %s", baseURL)
	}

	routes := map[string]url.URL{}
	for _, path := range paths {
		u := *base
		pathURI, err := url.Parse(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse path URI %s", path)
		}
		u.Path = pathURI.Path
		routes[path] = u
	}

	return routes, nil
}

// GetNetworkConfiguration Request to get network config.
func (c *Client) GetNetworkConfiguration(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
	payload := cns.GetNetworkContainerRequest{
		OrchestratorContext: orchestratorContext,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return nil, &CNSClientError{
			Code: types.UnexpectedError,
			Err:  err,
		}
	}

	u := c.routes[cns.GetNetworkContainerByOrchestratorContext]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, &CNSClientError{
			Code: types.UnexpectedError,
			Err:  errors.Errorf("http response %d", res.StatusCode),
		}
	}

	var resp cns.GetNetworkContainerResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return nil, &CNSClientError{
			Code: types.UnexpectedError,
			Err:  err,
		}
	}

	if resp.Response.ReturnCode != 0 {
		return nil, &CNSClientError{
			Code: resp.Response.ReturnCode,
			Err:  errors.New(resp.Response.Message),
		}
	}

	return &resp, nil
}

// CreateHostNCApipaEndpoint creates an endpoint in APIPA network for host container connectivity.
func (c *Client) CreateHostNCApipaEndpoint(ctx context.Context, networkContainerID string) (string, error) {
	payload := cns.CreateHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return "", errors.Wrap(err, "failed to encode CreateCNSHostNCApipaEndpointRequest")
	}

	u := c.routes[cns.CreateHostNCApipaEndpointPath]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return "", errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.CreateHostNCApipaEndpointResponse

	if err = json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return "", errors.Wrap(err, "failed to decode CreateHostNCApipaEndpointResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return "", errors.New(resp.Response.Message)
	}

	return resp.EndpointID, nil
}

// DeleteHostNCApipaEndpoint deletes the endpoint in APIPA network created for host container connectivity.
func (c *Client) DeleteHostNCApipaEndpoint(ctx context.Context, networkContainerID string) error {
	payload := cns.DeleteHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return errors.Wrap(err, "failed to encode DeleteHostNCApipaEndpointRequest")
	}

	u := c.routes[cns.DeleteHostNCApipaEndpointPath]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.DeleteHostNCApipaEndpointResponse

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return errors.Wrap(err, "failed to decode DeleteHostNCApipaEndpointResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return errors.New(resp.Response.Message)
	}

	return nil
}

// RequestIPAddress calls the requestIPAddress in CNS
func (c *Client) RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	var err error
	defer func() {
		if err != nil {
			if e := c.ReleaseIPAddress(ctx, ipconfig); e != nil {
				err = errors.Wrap(e, err.Error())
			}
		}
	}()

	var body bytes.Buffer
	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode IPConfigRequest")
	}

	u := c.routes[cns.RequestIPConfig]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}

	var response cns.IPConfigResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode IPConfigResponse")
	}

	if response.Response.ReturnCode != 0 {
		return nil, errors.New(response.Response.Message)
	}

	return &response, nil
}

// ReleaseIPAddress calls releaseIPAddress on CNS, ipaddress ex: (10.0.0.1)
func (c *Client) ReleaseIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) error {
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		return errors.Wrap(err, "failed to encode IPConfigRequest")
	}

	u := c.routes[cns.ReleaseIPConfig]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.Response

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return errors.Wrap(err, "failed to decode Response")
	}

	if resp.ReturnCode != 0 {
		return errors.New(resp.Message)
	}

	return nil
}

// GetIPAddressesMatchingStates takes a variadic number of string parameters, to get all IP Addresses matching a number of states
// usage GetIPAddressesWithStates(cns.Available, cns.Allocated)
func (c *Client) GetIPAddressesMatchingStates(ctx context.Context, stateFilter ...cns.IPConfigState) ([]cns.IPConfigurationStatus, error) {
	if len(stateFilter) == 0 {
		return nil, nil
	}

	payload := cns.GetIPAddressesRequest{
		IPConfigStateFilter: stateFilter,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return nil, errors.Wrap(err, "failed to encode GetIPAddressesRequest")
	}

	u := c.routes[cns.PathDebugIPAddresses]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.GetIPAddressStatusResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode GetIPAddressStatusResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return nil, errors.New(resp.Response.Message)
	}

	return resp.IPConfigurationStatus, nil
}

// GetPodOrchestratorContext calls GetPodIpOrchestratorContext API on CNS
func (c *Client) GetPodOrchestratorContext(ctx context.Context) (map[string]string, error) {
	u := c.routes[cns.PathDebugPodContext]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.GetPodContextResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, errors.Wrap(err, "failed to decode GetPodContextResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return nil, errors.New(resp.Response.Message)
	}

	return resp.PodContext, nil
}

// GetHTTPServiceData gets all public in-memory struct details for debugging purpose
func (c *Client) GetHTTPServiceData(ctx context.Context) (*restserver.GetHTTPServiceDataResponse, error) {
	u := c.routes[cns.PathDebugRestData]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}
	var resp restserver.GetHTTPServiceDataResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode GetHTTPServiceDataResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return nil, errors.New(resp.Response.Message)
	}

	return &resp, nil
}
