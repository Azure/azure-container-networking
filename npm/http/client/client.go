package client

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/npm/http/api"

	"github.com/Azure/azure-container-networking/npm"
)

type NPMHttpClient struct {
	endpoint string
	client   *http.Client
}

func NewNPMHttpClient(endpoint string) *NPMHttpClient {
	return &NPMHttpClient{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

func (n *NPMHttpClient) GetNpmMgr() (*npm.NetworkPolicyManager, error) {
	url := n.endpoint + api.NPMMgrPath
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}

	var ns npm.NetworkPolicyManager
	err = json.NewDecoder(res.Body).Decode(&ns)
	if err != nil {
		return nil, err
	}

	return &ns, nil
}
