package dncclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
)

const (
	// RegisterNodeURLFmt: /networks/{infraNetworkID}/node/{nodeID}
	registerNodeURLFmt = "%s/networks/%s/node/%s%s"
	// SyncNodeNetworkContainersURLFmt: /networks/{infraNetworkID}/node/{nodeID}/networkcontainers
	syncNodeNetworkContainersURLFmt = "%s/networks/%s/node/%s/networkcontainers%s"

	dncAPIVersion             = "?api-version=2018-03-01"
	registerNodeRetryInterval = 5 * time.Second
)

// NodeRegistrationRequest - Struct to hold node registration request.
type NodeRegistrationRequest struct {
	NumCores int `json:"NumCores"`
}

// RegisterNode registers the node with managed DNC
func RegisterNode(httpRestService cns.HTTPService, dncEP, infraVnet, nodeID string) {
	logger.Printf("[dncclient] Registering node: %s with Infrastructure Network: %s dncEP: %s", nodeID, infraVnet, dncEP)

	var (
		registerNodeURL = fmt.Sprintf(registerNodeURLFmt, dncEP, infraVnet, nodeID, dncAPIVersion)
		body            bytes.Buffer
		httpc           = acn.GetHttpClient()
	)

	// Create a body with number of CPU cores
	nodeRegistrationRequest := NodeRegistrationRequest{
		NumCores: runtime.NumCPU(),
	}
	json.NewEncoder(&body).Encode(nodeRegistrationRequest)

	for {
		orchestratorDetails, err := registerNode(httpc, registerNodeURL, &body)
		if err != nil {
			logger.Errorf("[dncclient] Failed to register node: %s with error: %+v", nodeID, err)
			// todo: make this interval configurable
			time.Sleep(registerNodeRetryInterval)
			continue
		}

		httpRestService.SetNodeOrchestrator(&orchestratorDetails)
		break
	}

	logger.Printf("[dncclient] Successfully registered node: %s", nodeID)
}

func registerNode(httpCl *http.Client, url string, body io.Reader) (cns.SetOrchestratorTypeRequest, error) {
	var orchestratorDetails cns.SetOrchestratorTypeRequest
	response, err := httpCl.Post(url, acn.JsonContent, body)
	if err != nil {
		return orchestratorDetails, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return orchestratorDetails,
			fmt.Errorf("[dncclient] Failed to register node with http status code: %d", response.StatusCode)
	}

	_ = json.NewDecoder(response.Body).Decode(&orchestratorDetails)
	return orchestratorDetails, nil
}

// SyncNodeNcStatus retrieves the NCs scheduled on this node by DNC
func SyncNodeNcStatus(dncEP, infraVnet, nodeID string) (cns.NodeInfoResponse, error) {
	var (
		syncNodeNcStatusURL = fmt.Sprintf(syncNodeNetworkContainersURLFmt, dncEP, infraVnet, nodeID, dncAPIVersion)
		nodeInfoResponse    cns.NodeInfoResponse
		httpc               = acn.GetHttpClient()
	)

	logger.Printf("[dncclient] SyncNodeNcStatus: Node: %s, InfraVnet: %s", nodeID, infraVnet)

	response, err := httpc.Get(syncNodeNcStatusURL)
	if err != nil {
		return nodeInfoResponse, err
	}

	if response.StatusCode == http.StatusOK {
		err = json.NewDecoder(response.Body).Decode(&nodeInfoResponse)
	} else {
		err = fmt.Errorf("%d", response.StatusCode)
	}
	response.Body.Close()

	return nodeInfoResponse, err
}
