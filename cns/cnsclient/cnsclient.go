package cnsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
)

const (
	contentTypeJSON = "application/json"
	defaultBaseURL  = "http://localhost:10090"
	// DefaultTimeout default timeout duration for CNS Client.
	DefaultTimeout = 5 * time.Second
)

// Client specifies a client to connect to Ipam Plugin.
type Client struct {
	client  http.Client
	baseURL url.URL
}

// New returns a new CNS client configured with the passed URL and timeout.
func New(baseURL string, requestTimeout time.Duration) (*Client, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse base URL %s", baseURL)
	}

	return &Client{
		baseURL: *u,
		client: http.Client{
			Timeout: requestTimeout,
		},
	}, nil
}

// GetNetworkConfiguration Request to get network config.
func (c *Client) GetNetworkConfiguration(orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
	u := c.baseURL
	pathURI, err := url.Parse(cns.GetNetworkContainerByOrchestratorContext)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse path URI %s", cns.GetNetworkContainerByOrchestratorContext)
	}
	u.Path = pathURI.Path

	payload := &cns.GetNetworkContainerRequest{
		OrchestratorContext: orchestratorContext,
	}

	var body bytes.Buffer
	if err = json.NewEncoder(&body).Encode(payload); err != nil {
		log.Errorf("encoding json failed with %v", err)
		return nil, &CNSClientError{types.UnexpectedError, err}
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return nil, errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, &CNSClientError{types.UnexpectedError, errors.Errorf("http response %d", res.StatusCode)}
	}

	var resp cns.GetNetworkContainerResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing GetNetworkConfiguration response resp:%v err:%v", res.Body, err.Error())
		return nil, &CNSClientError{types.UnexpectedError, err}
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf(
			"[Azure CNSClient] GetNetworkConfiguration received error response :%v , Code : %d",
			resp.Response.Message,
			resp.Response.ReturnCode)
		return nil, &CNSClientError{resp.Response.ReturnCode, fmt.Errorf(resp.Response.Message)}
	}

	return &resp, nil
}

// CreateHostNCApipaEndpoint creates an endpoint in APIPA network for host container connectivity.
func (c *Client) CreateHostNCApipaEndpoint(networkContainerID string) (string, error) {
	u := c.baseURL
	pathURI, err := url.Parse(cns.CreateHostNCApipaEndpointPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse path URI %s", cns.CreateHostNCApipaEndpointPath)
	}
	u.Path = pathURI.Path

	payload := &cns.CreateHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	var body bytes.Buffer
	if err = json.NewEncoder(&body).Encode(payload); err != nil {
		log.Errorf("encoding json failed with %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return "", errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return "", errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.CreateHostNCApipaEndpointResponse

	if err = json.NewDecoder(res.Body).Decode(&resp); err != nil {
		log.Errorf("[Azure CNSClient] Error parsing CreateHostNCApipaEndpoint response resp: %v err: %v",
			res.Body, err.Error())
		return "", err
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] CreateHostNCApipaEndpoint received error response :%v", resp.Response.Message)
		return "", fmt.Errorf(resp.Response.Message)
	}

	return resp.EndpointID, nil
}

// DeleteHostNCApipaEndpoint deletes the endpoint in APIPA network created for host container connectivity.
func (c *Client) DeleteHostNCApipaEndpoint(networkContainerID string) error {
	u := c.baseURL
	pathURI, err := url.Parse(cns.DeleteHostNCApipaEndpointPath)
	if err != nil {
		return errors.Wrapf(err, "failed to parse path URI %s", cns.DeleteHostNCApipaEndpointPath)
	}
	u.Path = pathURI.Path

	payload := &cns.DeleteHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		log.Errorf("encoding json failed with %v", err)
		return err
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.DeleteHostNCApipaEndpointResponse

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error parsing DeleteHostNCApipaEndpoint response resp: %v err: %v",
			res.Body, err.Error())
		return err
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] DeleteHostNCApipaEndpoint received error response :%v", resp.Response.Message)
		return fmt.Errorf(resp.Response.Message)
	}

	return nil
}

// RequestIPAddress calls the requestIPAddress in CNS
func (c *Client) RequestIPAddress(ipconfig *cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	u := c.baseURL
	pathURI, err := url.Parse(cns.RequestIPConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse path URI %s", cns.RequestIPConfig)
	}
	u.Path = pathURI.Path

	defer func() {
		if err != nil {
			if er := c.ReleaseIPAddress(ipconfig); er != nil {
				log.Errorf("failed to release IP address [%v] after failed add [%v]", er, err)
			}
		}
	}()

	var body bytes.Buffer
	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return nil, errors.Wrap(err, "failed to encode IPConfigRequest")
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return nil, errors.Wrap(err, "http request failed")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http response %d", res.StatusCode)
	}

	var response cns.IPConfigResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing RequestIPAddress response resp:%v err:%v", res.Body, err.Error())
		return nil, errors.Wrap(err, "failed to decode IPConfigResponse")
	}

	if response.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] RequestIPAddress received error response :%v", response.Response.Message)
		return nil, errors.New(response.Response.Message)
	}

	return &response, nil
}

// ReleaseIPAddress calls releaseIPAddress on CNS, ipaddress ex: (10.0.0.1)
func (c *Client) ReleaseIPAddress(ipconfig *cns.IPConfigRequest) error {
	u := c.baseURL
	pathURI, err := url.Parse(cns.ReleaseIPConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to parse path URI %s", cns.ReleaseIPConfig)
	}
	u.Path = pathURI.Path

	var body bytes.Buffer
	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return err
	}

	log.Printf("Releasing ipconfig %s", body.String())

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return errors.Wrap(err, "http request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.Errorf("http response %d", res.StatusCode)
	}

	var resp cns.Response

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing ReleaseIPAddress response resp:%v err:%v", res.Body, err.Error())
		return err
	}

	if resp.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] ReleaseIPAddress received error response :%v", resp.Message)
		return fmt.Errorf(resp.Message)
	}

	return err
}

// GetIPAddressesWithStates takes a variadic number of string parameters, to get all IP Addresses matching a number of states
// usage GetIPAddressesWithStates(cns.Available, cns.Allocated)
func (c *Client) GetIPAddressesMatchingStates(stateFilter ...cns.IPConfigState) ([]cns.IPConfigurationStatus, error) {
	if len(stateFilter) == 0 {
		return nil, nil
	}
	u := c.baseURL
	pathURI, err := url.Parse(cns.PathDebugIPAddresses)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse path URI %s", cns.PathDebugIPAddresses)
	}
	u.Path = pathURI.Path

	log.Debugf("GetIPAddressesMatchingStates url %s", u.String())

	payload := cns.GetIPAddressesRequest{
		IPConfigStateFilter: stateFilter,
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return nil, errors.Wrap(err, "failed to encode GetIPAddressesRequest")
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	res, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
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
func (c *Client) GetPodOrchestratorContext() (map[string]string, error) {
	u := c.baseURL
	pathURI, err := url.Parse(cns.PathDebugPodContext)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse path URI %s", cns.PathDebugPodContext)
	}
	u.Path = pathURI.Path
	log.Printf("GetPodIPOrchestratorContext url %v", u)

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, u.String(), nil)
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
func (c *Client) GetHTTPServiceData() (*restserver.GetHTTPServiceDataResponse, error) {
	u := c.baseURL
	pathURI, err := url.Parse(cns.PathDebugRestData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse path URI %s", cns.PathDebugRestData)
	}
	u.Path = pathURI.Path
	log.Printf("GetHTTPServiceStruct url %v", u.String())

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, u.String(), nil)
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
