package nmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/common"
	"github.com/pkg/errors"
)

const (
	// GetNmAgentSupportedApiURLFmt Api endpoint to get supported Apis of NMAgent
	GetNmAgentSupportedApiURLFmt       = "http://%s/machine/plugins/?comp=nmagent&type=GetSupportedApis"
	GetNetworkContainerVersionURLFmt   = "http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/version/authenticationToken/%s/api-version/1"
	GetNcVersionListWithOutTokenURLFmt = "http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/api-version/%s"
	JoinNetworkURLFmt                  = "NetworkManagement/joinedVirtualNetworks/%s/api-version/1"
	PutNetworkValueFmt                 = "NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1"
	DeleteNetworkContainerURLFmt       = "NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1/method/DELETE"
)

// WireServerIP - wire server ip
var (
	WireserverIP                           = "168.63.129.16"
	WireServerPath                         = "machine/plugins"
	WireServerScheme                       = "http"
	getNcVersionListWithOutTokenURLVersion = "2"
)

// NetworkContainerResponse - NMAgent response.
type NetworkContainerResponse struct {
	ResponseCode       string `json:"httpStatusCode"`
	NetworkContainerID string `json:"networkContainerId"`
	Version            string `json:"version"`
}

type ContainerInfo struct {
	NetworkContainerID string `json:"networkContainerId"`
	Version            string `json:"version"`
}

type NetworkContainerListResponse struct {
	ResponseCode string          `json:"httpStatusCode"`
	Containers   []ContainerInfo `json:"networkContainers"`
}

// Client is client to handle queries to nmagent
type Client struct {
	connectionURL string
}

// NewClient create a new nmagent client.
func NewClient(url string) (*Client, error) {
	if url == "" {
		url = fmt.Sprintf(GetNcVersionListWithOutTokenURLFmt, WireserverIP, getNcVersionListWithOutTokenURLVersion)
	}
	return &Client{
		connectionURL: url,
	}, nil
}

// GetNetworkContainerVersion :- Retrieves NC version from NMAgent
func GetNetworkContainerVersion(networkContainerID, getNetworkContainerVersionURL string) (*http.Response, error) {
	logger.Printf("[NMAgentClient] GetNetworkContainerVersion NC: %s", networkContainerID)

	response, err := common.GetHttpClient().Get(getNetworkContainerVersionURL)

	logger.Printf("[NMAgentClient][Response] GetNetworkContainerVersion NC: %s. Response: %+v. Error: %v",
		networkContainerID, response, err)
	return response, err
}

// GetNCVersionList query nmagent for programmed container versions.
func (c *Client) GetNCVersionList(ctx context.Context) (*NetworkContainerListResponse, error) {
	now := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.connectionURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build nmagent request")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make nmagent request")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	logger.Printf("[NMAgentClient][Response] GetNcVersionListWithOutToken response: %s, latency is %d", string(b), time.Since(now).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to GetNCVersionList with status %d", resp.StatusCode)
	}

	var response NetworkContainerListResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	return &response, nil
}
