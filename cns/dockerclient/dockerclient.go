// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package dockerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
)

const (
	defaultDockerConnectionURL = "http://127.0.0.1:2375"
	defaultIpamPlugin          = "azure-vnet"
	networkMode                = "com.microsoft.azure.network.mode"
	bridgeMode                 = "bridge"
)

type interfaceGetter interface {
	GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error)
}

// Client specifies a client to connect to docker.
type Client struct {
	connectionURL string
	wscli         interfaceGetter
}

// NewDefaultClient create a new docker client.
func NewDefaultClient(wscli interfaceGetter) (*Client, error) {
	return &Client{
		connectionURL: defaultDockerConnectionURL,
		wscli:         wscli,
	}, nil
}

// NetworkExists tries to retrieve a network from docker (if it exists).
func (c *Client) NetworkExists(networkName string) error {
	logger.Printf("[Azure CNS] NetworkExists")

	res, err := http.Get(
		c.connectionURL + inspectNetworkPath + networkName)
	if err != nil {
		logger.Errorf("[Azure CNS] Error received from http Post for docker network inspect %v %v", networkName, err.Error())
		return err
	}

	defer res.Body.Close()

	// network exists
	if res.StatusCode == 200 {
		logger.Debugf("[Azure CNS] Network with name %v already exists. Docker return code: %v", networkName, res.StatusCode)
		return nil
	}

	// network not found
	if res.StatusCode == 404 {
		logger.Debugf("[Azure CNS] Network with name %v does not exist. Docker return code: %v", networkName, res.StatusCode)
		return fmt.Errorf("Network not found")
	}

	return fmt.Errorf("Unknown return code from docker inspect %d", res.StatusCode)
}

// CreateNetwork creates a network using docker network create.
func (c *Client) CreateNetwork(networkName string, nicInfo *wireserver.InterfaceInfo, options map[string]interface{}) error {
	logger.Printf("[Azure CNS] CreateNetwork")

	enableSnat := true

	config := &Config{
		Subnet: nicInfo.Subnet,
	}

	configs := make([]Config, 1)
	configs[0] = *config
	ipamConfig := &IPAM{
		Driver: defaultIpamPlugin,
		Config: configs,
	}

	netConfig := &NetworkConfiguration{
		Name:     networkName,
		Driver:   defaultNetworkPlugin,
		IPAM:     *ipamConfig,
		Internal: true,
	}

	if options != nil {
		if _, ok := options[OptDisableSnat]; ok {
			enableSnat = false
		}
	}

	if enableSnat {
		netConfig.Options = make(map[string]interface{})
		netConfig.Options[networkMode] = bridgeMode
	}

	logger.Printf("[Azure CNS] Going to create network with config: %+v", netConfig)

	netConfigJSON := new(bytes.Buffer)
	err := json.NewEncoder(netConfigJSON).Encode(netConfig)
	if err != nil {
		return err
	}

	res, err := http.Post(
		c.connectionURL+createNetworkPath,
		common.JsonContent,
		netConfigJSON)
	if err != nil {
		logger.Printf("[Azure CNS] Error received from http Post for docker network create %v", networkName)
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != 201 {
		var createNetworkResponse DockerErrorResponse
		err = json.NewDecoder(res.Body).Decode(&createNetworkResponse)
		var ermsg string
		ermsg = ""
		if err != nil {
			ermsg = err.Error()
		}
		return fmt.Errorf("[Azure CNS] Create docker network failed with error code %v - %v - %v",
			res.StatusCode, createNetworkResponse.message, ermsg)
	}

	if enableSnat {
		err = platform.SetOutboundSNAT(nicInfo.Subnet)
		if err != nil {
			logger.Printf("[Azure CNS] Error setting up SNAT outbound rule %v", err)
		}
	}

	return nil
}

// DeleteNetwork creates a network using docker network create.
func (c *Client) DeleteNetwork(networkName string) error {
	p := platform.NewExecClient(nil)
	logger.Printf("[Azure CNS] DeleteNetwork")

	url := c.connectionURL + inspectNetworkPath + networkName
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		logger.Printf("[Azure CNS] Error received while creating http DELETE request for network delete %v %v", networkName, err.Error())
		return err
	}

	req.Header.Set(common.ContentType, common.JsonContent)
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		logger.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return err
	}

	defer res.Body.Close()

	// network successfully deleted.
	if res.StatusCode == 204 {
		res, err := c.wscli.GetInterfaces(context.TODO()) // TODO(rbtr): thread context through this client
		if err != nil {
			return errors.Wrap(err, "failed to get interfaces from IMDS")
		}
		primaryNic, err := wireserver.GetPrimaryInterfaceFromResult(res)
		if err != nil {
			return errors.Wrap(err, "failed to get primary interface from IMDS response")
		}

		cmd := fmt.Sprintf("iptables -t nat -D POSTROUTING -m iprange ! --dst-range 168.63.129.16 -m addrtype ! --dst-type local ! -d %v -j MASQUERADE",
			primaryNic.Subnet)
		_, err = p.ExecuteCommand(cmd)
		if err != nil {
			logger.Printf("[Azure CNS] Error Removing Outbound SNAT rule %v", err)
		}

		return nil
	}

	// network not found.
	if res.StatusCode == 404 {
		return fmt.Errorf("[Azure CNS] Network not found %v", networkName)
	}

	return fmt.Errorf("[Azure CNS] Unknown return code from docker delete network %v: ret = %d",
		networkName, res.StatusCode)
}
