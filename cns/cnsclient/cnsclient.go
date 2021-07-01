package cnsclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/log"
)

// CNSClient specifies a client to connect to Ipam Plugin.
type CNSClient struct {
	connectionURL string
	httpc         http.Client
}

const (
	defaultCnsURL   = "http://localhost:10090"
	contentTypeJSON = "application/json"
)

var (
	cnsClient *CNSClient
)

// InitCnsClient initializes new cns client and returns the object
func InitCnsClient(url string, requestTimeout time.Duration) (*CNSClient, error) {
	if cnsClient == nil {
		if url == "" {
			url = defaultCnsURL
		}

		cnsClient = &CNSClient{
			connectionURL: url,
			httpc: http.Client{
				Timeout: requestTimeout,
			},
		}
	}

	return cnsClient, nil
}

// GetCnsClient returns the cns client object
func GetCnsClient() (*CNSClient, error) {
	var err error

	if cnsClient == nil {
		err = &CNSClientError{
			restserver.UnexpectedError,
			fmt.Errorf("[Azure CNSClient] CNS Client not initialized")}
	}

	return cnsClient, err
}

// GetNetworkConfiguration Request to get network config.
func (cnsClient *CNSClient) GetNetworkConfiguration(orchestratorContext []byte) (
	*cns.GetNetworkContainerResponse, error) {
	var (
		body bytes.Buffer
	)

	url := cnsClient.connectionURL + cns.GetNetworkContainerByOrchestratorContext
	log.Printf("GetNetworkConfiguration url %v", url)

	payload := &cns.GetNetworkContainerRequest{
		OrchestratorContext: orchestratorContext,
	}

	err := json.NewEncoder(&body).Encode(payload)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return nil, &CNSClientError{restserver.UnexpectedError, err}
	}

	res, err := cnsClient.httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return nil, &CNSClientError{restserver.UnexpectedError, err}
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] GetNetworkConfiguration invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return nil, &CNSClientError{restserver.UnexpectedError, fmt.Errorf(errMsg)}
	}

	var resp cns.GetNetworkContainerResponse

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing GetNetworkConfiguration response resp:%v err:%v", res.Body, err.Error())
		return nil, &CNSClientError{restserver.UnexpectedError, err}
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
func (cnsClient *CNSClient) CreateHostNCApipaEndpoint(networkContainerID string) (string, error) {
	var (
		err  error
		body bytes.Buffer
	)

	url := cnsClient.connectionURL + cns.CreateHostNCApipaEndpointPath
	log.Printf("CreateHostNCApipaEndpoint url: %v for NC: %s", url, networkContainerID)

	payload := &cns.CreateHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	if err = json.NewEncoder(&body).Encode(payload); err != nil {
		log.Errorf("encoding json failed with %v", err)
		return "", err
	}

	res, err := cnsClient.httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] CreateHostNCApipaEndpoint: Invalid http status code: %v",
			res.StatusCode)
		log.Errorf(errMsg)
		return "", fmt.Errorf(errMsg)
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
func (cnsClient *CNSClient) DeleteHostNCApipaEndpoint(networkContainerID string) error {
	var body bytes.Buffer

	url := cnsClient.connectionURL + cns.DeleteHostNCApipaEndpointPath
	log.Printf("DeleteHostNCApipaEndpoint url: %v for NC: %s", url, networkContainerID)

	payload := &cns.DeleteHostNCApipaEndpointRequest{
		NetworkContainerID: networkContainerID,
	}

	err := json.NewEncoder(&body).Encode(payload)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return err
	}

	res, err := cnsClient.httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] DeleteHostNCApipaEndpoint: Invalid http status code: %v",
			res.StatusCode)
		log.Errorf(errMsg)
		return fmt.Errorf(errMsg)
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
func (cnsClient *CNSClient) RequestIPAddress(ipconfig *cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	var (
		err      error
		res      *http.Response
		response *cns.IPConfigResponse
	)

	var body bytes.Buffer

	url := cnsClient.connectionURL + cns.RequestIPConfig

	defer func() {
		if err != nil {
			if er := cnsClient.ReleaseIPAddress(ipconfig); er != nil {
				log.Errorf("failed to release IP address [%v] after failed add [%v]", er, err)
			}
		}
	}()

	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return response, err
	}

	res, err = cnsClient.httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return response, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] RequestIPAddress invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return response, fmt.Errorf(errMsg)
	}

	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing RequestIPAddress response resp:%v err:%v", res.Body, err.Error())
		return response, err
	}

	if response.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] RequestIPAddress received error response :%v", response.Response.Message)
		return response, fmt.Errorf(response.Response.Message)
	}

	return response, err
}

// ReleaseIPAddress calls releaseIPAddress on CNS, ipaddress ex: (10.0.0.1)
func (cnsClient *CNSClient) ReleaseIPAddress(ipconfig *cns.IPConfigRequest) error {
	var (
		err  error
		res  *http.Response
		body bytes.Buffer
	)

	url := cnsClient.connectionURL + cns.ReleaseIPConfig

	err = json.NewEncoder(&body).Encode(ipconfig)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return err
	}

	log.Printf("Releasing ipconfig %s", string(body.Bytes()))

	res, err = cnsClient.httpc.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] ReleaseIPAddress invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return fmt.Errorf(errMsg)
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
func (cnsClient *CNSClient) GetIPAddressesMatchingStates(StateFilter ...string) ([]cns.IPConfigurationStatus, error) {
	var (
		resp cns.GetIPAddressStatusResponse
		err  error
		res  *http.Response
		body bytes.Buffer
	)

	if len(StateFilter) == 0 {
		return resp.IPConfigurationStatus, nil
	}

	url := cnsClient.connectionURL + cns.GetIPAddresses
	log.Printf("GetIPAddressesMatchingStates url %v", url)

	payload := &cns.GetIPAddressesRequest{
		IPConfigStateFilter: StateFilter,
	}

	err = json.NewEncoder(&body).Encode(payload)
	if err != nil {
		log.Errorf("encoding json failed with %v", err)
		return resp.IPConfigurationStatus, err
	}

	res, err = http.Post(url, contentTypeJSON, &body)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Post returned error %v", err.Error())
		return resp.IPConfigurationStatus, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] GetIPAddressesMatchingStates invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return resp.IPConfigurationStatus, fmt.Errorf(errMsg)
	}

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing GetIPAddressesMatchingStates response resp:%v err:%v", res.Body, err.Error())
		return resp.IPConfigurationStatus, err
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] GetIPAddressesMatchingStates received error response :%v", resp.Response.Message)
		return resp.IPConfigurationStatus, fmt.Errorf(resp.Response.Message)
	}

	return resp.IPConfigurationStatus, err
}

//GetPodOrchestratorContext calls GetPodIpOrchestratorContext API on CNS
func (cnsClient *CNSClient) GetPodOrchestratorContext() (map[string]string, error) {
	var (
		resp cns.GetPodContextResponse
		err  error
		res  *http.Response
	)

	url := cnsClient.connectionURL + cns.GetPodIPOrchestratorContext
	log.Printf("GetPodIPOrchestratorContext url %v", url)

	res, err = http.Get(url)
	if err != nil {
		log.Errorf("[Azure CNSClient] GetPodIPOrchestratorContext HTTP Get returned error %v", err.Error())
		return resp.PodContext, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] GetPodIPOrchestratorContext invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return resp.PodContext, fmt.Errorf(errMsg)
	}

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing GetPodContext response resp:%v err:%v", res.Body, err.Error())
		return resp.PodContext, err
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] GetPodContext received error response :%v", resp.Response.Message)
		return resp.PodContext, fmt.Errorf(resp.Response.Message)
	}

	return resp.PodContext, err
}

//GetHTTPServiceData gets all public in-memory struct details for debugging purpose
func (cnsClient *CNSClient) GetHTTPServiceData() (restserver.GetHTTPServiceDataResponse, error) {
	var (
		resp restserver.GetHTTPServiceDataResponse
		err  error
		res  *http.Response
	)

	url := cnsClient.connectionURL + cns.GetHTTPRestData
	log.Printf("GetHTTPServiceStruct url %v", url)

	res, err = http.Get(url)
	if err != nil {
		log.Errorf("[Azure CNSClient] HTTP Get returned error %v", err.Error())
		return resp, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("[Azure CNSClient] GetHTTPServiceStruct invalid http status code: %v", res.StatusCode)
		log.Errorf(errMsg)
		return resp, fmt.Errorf(errMsg)
	}

	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		log.Errorf("[Azure CNSClient] Error received while parsing GetHTTPServiceStruct response resp:%v err:%v", res.Body, err.Error())
		return resp, err
	}

	if resp.Response.ReturnCode != 0 {
		log.Errorf("[Azure CNSClient] GetTTPServiceStruct received error response :%v", resp.Response.Message)
		return resp, fmt.Errorf(resp.Response.Message)
	}

	return resp, err
}
