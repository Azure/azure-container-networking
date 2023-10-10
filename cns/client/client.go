package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	cns.GetAllNetworkContainers,
	cns.CreateHostNCApipaEndpointPath,
	cns.DeleteHostNCApipaEndpointPath,
	cns.RequestIPConfig,
	cns.RequestIPConfigs,
	cns.ReleaseIPConfig,
	cns.ReleaseIPConfigs,
	cns.PathDebugIPAddresses,
	cns.PathDebugPodContext,
	cns.PathDebugRestData,
	cns.UnpublishNetworkContainer,
	cns.PublishNetworkContainer,
	cns.CreateOrUpdateNetworkContainer,
	cns.SetOrchestratorType,
	cns.NumberOfCPUCores,
	cns.NMAgentSupportedAPIs,
	cns.DeleteNetworkContainer,
	cns.NetworkContainersURLPath,
	cns.GetHomeAz,
}

type do interface {
	Do(*http.Request) (*http.Response, error)
}

// Client specifies a client to connect to Ipam Plugin.
type Client struct {
	client do
	routes map[string]url.URL
}

type ConnectionFailureErr struct {
	cause error
}

func (e *ConnectionFailureErr) Error() string {
	return e.cause.Error()
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
		client: &http.Client{
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

// GetAllNetworkContainers Request to get network container configs.
func (c *Client) GetAllNetworkContainers(ctx context.Context, orchestratorContext []byte) ([]cns.GetNetworkContainerResponse, error) {
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

	u := c.routes[cns.GetAllNetworkContainers]
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

	//  investigate 404 orchestratorContext which is invalid and make sure this is addressed
	if res.StatusCode == http.StatusNotFound {
		return nil, &CNSClientError{
			Code: types.UnsupportedAPI,
			Err:  errors.Errorf("Unsupported API"),
		}
	}

	if res.StatusCode != http.StatusOK {
		return nil, &CNSClientError{
			Code: types.UnexpectedError,
			Err:  errors.Errorf("http response %d", res.StatusCode),
		}
	}

	var resp cns.GetAllNetworkContainersResponse
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

	return resp.NetworkContainers, nil
}

// GetNetworkContainer Request to get network container config.
func (c *Client) GetNetworkContainer(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
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
		return &ConnectionFailureErr{
			cause: err,
		}
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

// RequestIPs calls the RequestIPConfigs in CNS
func (c *Client) RequestIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	var err error
	defer func() {
		if err != nil {
			if e := c.ReleaseIPs(ctx, ipconfig); e != nil {
				err = errors.Wrap(e, err.Error())
			}
		}
	}()

	var body bytes.Buffer
	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode IPConfigsRequest")
	}

	u := c.routes[cns.RequestIPConfigs]
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

	if res.StatusCode == http.StatusNotFound {
		return nil, &CNSClientError{
			Code: types.UnsupportedAPI,
			Err:  errors.Errorf("Unsupported API"),
		}
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}

	var response cns.IPConfigsResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode IPConfigsResponse")
	}

	if response.Response.ReturnCode != 0 {
		return nil, errors.New(response.Response.Message)
	}

	return &response, nil
}

// ReleaseIPs calls releaseIPs on which releases the IPs on the pod
func (c *Client) ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error {
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		return errors.Wrap(err, "failed to encode IPConfigsRequest")
	}

	u := c.routes[cns.ReleaseIPConfigs]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		return &ConnectionFailureErr{
			cause: err,
		}
	}
	defer res.Body.Close()

	// if we get a 404 error
	if res.StatusCode == http.StatusNotFound {
		return &CNSClientError{
			Code: types.UnsupportedAPI,
			Err:  errors.Errorf("Unsupported API"),
		}
	}

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
// usage GetIPAddressesWithStates(ctx, types.Available...)
func (c *Client) GetIPAddressesMatchingStates(ctx context.Context, stateFilter ...types.IPState) ([]cns.IPConfigurationStatus, error) {
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
func (c *Client) GetPodOrchestratorContext(ctx context.Context) (map[string][]string, error) {
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
	b, err := io.ReadAll(res.Body)
	s := string(b)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to read body %s", s))
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}
	var resp restserver.GetHTTPServiceDataResponse
	err = json.NewDecoder(bytes.NewReader(b)).Decode(&resp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode GetHTTPServiceDataResponse")
	}

	if resp.Response.ReturnCode != 0 {
		return nil, errors.New(resp.Response.Message)
	}

	return &resp, nil
}

// NumOfCPUCores returns the number of CPU cores available on the host that
// CNS is running on.
func (c *Client) NumOfCPUCores(ctx context.Context) (*cns.NumOfCPUCoresResponse, error) {
	// build the request
	u := c.routes[cns.NumberOfCPUCores]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "building http request")
	}

	// submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response
	var out cns.NumOfCPUCoresResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return nil, errors.Wrap(err, "decoding response as JSON")
	}

	// if the return code is non-zero, something went wrong and it should be
	// surfaced to the caller
	if out.Response.ReturnCode != 0 {
		return nil, &CNSClientError{
			Code: out.Response.ReturnCode,
			Err:  errors.New(out.Response.Message),
		}
	}

	return &out, nil
}

// DeleteNetworkContainer destroys the requested network container matching the
// provided ID.
func (c *Client) DeleteNetworkContainer(ctx context.Context, ncID string) error {
	// the network container ID is required by the API, so ensure that we have
	// one before we even make the request
	if ncID == "" {
		return errors.New("no network container ID provided")
	}

	// build the request
	dncr := cns.DeleteNetworkContainerRequest{
		NetworkContainerid: ncID,
	}
	body, err := json.Marshal(dncr)
	if err != nil {
		return errors.Wrap(err, "encoding request body")
	}
	u := c.routes[cns.DeleteNetworkContainer]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response
	var out cns.DeleteNetworkContainerResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return errors.Wrap(err, "decoding response as JSON")
	}

	// if a non-zero response code was received from CNS, it means something went
	// wrong and it should be surfaced to the caller as an error
	if out.Response.ReturnCode != 0 {
		return errors.New(out.Response.Message)
	}

	// otherwise the response isn't terribly useful in a successful case, so it
	// doesn't make sense to provide it to callers. The absence of an error is
	// sufficient to communicate success.
	return nil
}

// SetOrchestratorType sets the orchestrator type for a given node
func (c *Client) SetOrchestratorType(ctx context.Context, sotr cns.SetOrchestratorTypeRequest) error {
	// validate that the request has all of the required fields before we waste a
	// round trip
	if sotr.OrchestratorType == "" {
		return errors.New("request missing field OrchestratorType")
	}

	if sotr.DncPartitionKey == "" {
		return errors.New("request missing field DncPartitionKey")
	}

	if sotr.NodeID == "" {
		return errors.New("request missing field NodeID")
	}

	// build the HTTP request using the supplied request body
	// submit the request
	body, err := json.Marshal(sotr)
	if err != nil {
		return errors.Wrap(err, "encoding request body")
	}
	u := c.routes[cns.SetOrchestratorType]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// send the request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response
	var out cns.Response
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return errors.Wrap(err, "decoding JSON response")
	}

	// if there was a non-zero response code, this is an error that
	// should be communicated back to the caller...
	if out.ReturnCode != 0 {
		return errors.New(out.Message)
	}

	// ...otherwise it's a success and returning nil is sufficient to
	// communicate that
	return nil
}

// CreateNetworkContainer will create the provided network container, or update
// an existing one if one already exists.
func (c *Client) CreateNetworkContainer(ctx context.Context, cncr cns.CreateNetworkContainerRequest) error {
	// CreateNetworkContainerRequest is a deep and complicated struct, so
	// validating fields before we send it off is difficult and likely redundant
	// since the backend will have similar checks. However, we can be pretty
	// certain that if the NetworkContainerid is missing, it's likely an invalid
	// request (since that parameter is mandatory).
	if cncr.NetworkContainerid == "" {
		return errors.New("empty request provided")
	}

	// build the request using the supplied struct and the client's internal
	// routes
	body, err := json.Marshal(cncr)
	if err != nil {
		return errors.Wrap(err, "encoding request as JSON")
	}
	u := c.routes[cns.CreateOrUpdateNetworkContainer]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// send the request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response
	var out cns.Response
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return errors.Wrap(err, "decoding JSON response")
	}

	// if there was a non-zero response code, this is an error that
	// should be communicated back to the caller...
	if out.ReturnCode != 0 {
		return errors.New(out.Message)
	}

	// ...otherwise the request was successful so
	return nil
}

// PublishNetworkContainer publishes the provided network container via the
// NMAgent resident on the node where CNS is running. This effectively proxies
// the publication through CNS which can be useful for avoiding throttling
// issues from Wireserver.
func (c *Client) PublishNetworkContainer(ctx context.Context, pncr cns.PublishNetworkContainerRequest) error {
	// Given that the PublishNetworkContainer endpoint is intended to publish
	// network containers, it's reasonable to assume that the request is invalid
	// if it's missing a NetworkContainerID. Check for its presence and
	// pre-emptively fail if that ID is missing:
	if pncr.NetworkContainerID == "" {
		return errors.New("network container id missing from request")
	}

	// Now that the request is valid it can be packaged as an HTTP request:
	body, err := json.Marshal(pncr)
	if err != nil {
		return errors.Wrap(err, "encoding request body as json")
	}
	u := c.routes[cns.PublishNetworkContainer]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// send the HTTP request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response to see if it was successful
	var out cns.PublishNetworkContainerResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return errors.Wrap(err, "decoding JSON response")
	}

	// if there was a non-zero response code, this is an error that
	// should be communicated back to the caller...
	if out.Response.ReturnCode != 0 {
		return errors.New(out.Response.Message)
	}

	// ...otherwise the request was successful so
	return nil
}

// UnpublishNC unpublishes the network container via the NMAgent running
// alongside the CNS service. This is useful to avoid throttling issues imposed
// by Wireserver.
func (c *Client) UnpublishNC(ctx context.Context, uncr cns.UnpublishNetworkContainerRequest) error {
	// In order to unpublish a Network Container, we need its ID. If the ID is
	// missing, we can assume that the request is invalid and immediately return
	// an error
	if uncr.NetworkContainerID == "" {
		return errors.New("request missing network container id")
	}

	// Now that the request is valid it can be packaged as an HTTP request:
	body, err := json.Marshal(uncr)
	if err != nil {
		return errors.Wrap(err, "encoding request body as json")
	}
	u := c.routes[cns.UnpublishNetworkContainer]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// send the HTTP request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response to see if it was successful
	var out cns.UnpublishNetworkContainerResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return errors.Wrap(err, "decoding JSON response")
	}

	// if there was a non-zero response code, this is an error that
	// should be communicated back to the caller...
	if out.Response.ReturnCode != 0 {
		return errors.New(out.Response.Message)
	}

	// ...otherwise the request was successful so
	return nil
}

// NMAgentSupportedAPIs returns the supported API names from NMAgent. This can
// be used, for example, to detect whether the node is capable for GRE
// allocations.
func (c *Client) NMAgentSupportedAPIs(ctx context.Context) (*cns.NmAgentSupportedApisResponse, error) {
	// build the request
	reqBody := &cns.NmAgentSupportedApisRequest{
		// the IP used below is that of the Wireserver
		GetNmAgentSupportedApisURL: "http://168.63.129.16/machine/plugins/?comp=nmagent&type=GetSupportedApis",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "encoding request body")
	}

	u := c.routes[cns.NMAgentSupportedAPIs]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "building http request")
	}

	// submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "sending http request")
	}
	defer resp.Body.Close()

	if code := resp.StatusCode; code != http.StatusOK {
		return nil, &FailedHTTPRequest{
			Code: code,
		}
	}

	// decode response
	var out cns.NmAgentSupportedApisResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return nil, errors.Wrap(err, "decoding response body")
	}

	// if there was a non-zero status code, that indicates an error and should be
	// communicated as such
	if out.Response.ReturnCode != 0 {
		return nil, &CNSClientError{
			Code: out.Response.ReturnCode,
			Err:  errors.New(out.Response.Message),
		}
	}

	return &out, nil
}

func (c *Client) GetAllNCsFromCns(ctx context.Context) (cns.GetAllNetworkContainersResponse, error) {
	// Build the request
	urlPath := c.routes[cns.NetworkContainersURLPath]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath.String(), http.NoBody)
	if err != nil {
		return cns.GetAllNetworkContainersResponse{}, errors.Wrap(err, "building HTTP request")
	}

	// Submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return cns.GetAllNetworkContainersResponse{}, errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// Decode the response
	var response cns.GetAllNetworkContainersResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil || response.Response.ReturnCode != types.Success {
		return cns.GetAllNetworkContainersResponse{}, errors.Wrap(err, "decoding GetAllNetworkContainersResponse as JSON")
	}

	return response, nil
}

func (c *Client) PostAllNetworkContainers(ctx context.Context, createNcRequest cns.PostNetworkContainersRequest) error {
	if createNcRequest.CreateNetworkContainerRequests == nil || len(createNcRequest.CreateNetworkContainerRequests) == 0 {
		return errors.New("empty request provided")
	}

	// Build the request
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(createNcRequest)
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}
	urlPath := c.routes[cns.NetworkContainersURLPath]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath.String(), &body)
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}

	// Submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// Decode the response
	var response cns.PostNetworkContainersResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil || response.Response.ReturnCode != types.Success {
		return errors.Wrap(err, "decoding PostNetworkContainersResponse as JSON")
	}

	return nil
}

// GetHomeAz gets home AZ of host
func (c *Client) GetHomeAz(ctx context.Context) (*cns.GetHomeAzResponse, error) {
	// build the request
	u := c.routes[cns.GetHomeAz]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "building http request")
	}

	// submit the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	// decode the response
	var getHomeAzResponse cns.GetHomeAzResponse
	err = json.NewDecoder(resp.Body).Decode(&getHomeAzResponse)
	if err != nil {
		return nil, errors.Wrap(err, "decoding response as JSON")
	}

	// if the return code is non-zero, something went wrong and it should be
	// surfaced to the caller
	if getHomeAzResponse.Response.ReturnCode != 0 {
		return nil, &CNSClientError{
			Code: getHomeAzResponse.Response.ReturnCode,
			Err:  errors.New(getHomeAzResponse.Response.Message),
		}
	}

	return &getHomeAzResponse, nil
}
