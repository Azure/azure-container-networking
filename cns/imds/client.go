// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package imds

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/avast/retry-go/v4"
	"github.com/pkg/errors"
)

// see docs for IMDS here: https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service

// Client returns metadata about the VM by querying IMDS
type Client struct {
	cli    *http.Client
	config clientConfig
}

// clientConfig holds config options for a Client
type clientConfig struct {
	endpoint      string
	retryAttempts uint
}

type ClientOption func(*clientConfig)

// Endpoint overrides the default endpoint for a Client
func Endpoint(endpoint string) ClientOption {
	return func(c *clientConfig) {
		c.endpoint = endpoint
	}
}

// RetryAttempts overrides the default retry attempts for the client
func RetryAttempts(attempts uint) ClientOption {
	return func(c *clientConfig) {
		c.retryAttempts = attempts
	}
}

const (
	vmUniqueIDProperty    = "vmId"
	imdsComputePath       = "/metadata/instance/compute"
	imdsComputeAPIVersion = "api-version=2021-01-01"
	imdsFormatJSON        = "format=json"
	metadataHeaderKey     = "Metadata"
	metadataHeaderValue   = "true"
	defaultRetryAttempts  = 3
	defaultIMDSEndpoint   = "http://169.254.169.254"
)

var (
	ErrVMUniqueIDNotFound   = errors.New("vm unique ID not found")
	ErrUnexpectedStatusCode = errors.New("imds returned an unexpected status code")
)

// Define struct for Network Interface
type NetworkInterface struct {
	MacAddress string `json:"macAddress"`
	NcID       string `json:"ncId"`
}

// Define struct for Network
type Network struct {
	Interface []NetworkInterface `json:"interface"`
}

// NewClient creates a new imds client
func NewClient(opts ...ClientOption) *Client {
	config := clientConfig{
		endpoint:      defaultIMDSEndpoint,
		retryAttempts: defaultRetryAttempts,
	}

	for _, o := range opts {
		o(&config)
	}

	return &Client{
		cli:    &http.Client{},
		config: config,
	}
}

func (c *Client) GetVMUniqueID(ctx context.Context) (string, error) {
	var vmUniqueID string
	err := retry.Do(func() error {
		computeDoc, err := c.getInstanceComputeMetadata(ctx)

		if err != nil {
			return errors.Wrap(err, "error getting IMDS compute metadata")
		}

		// logger.Printf("Complete IMDS call response: %v", computeDoc)
		// macaddressData, ok1 := computeDoc["macaddress"].(string)
		// if !ok1 {
		// 	return errors.New("unable to parse IMDS macaddress metadata")
		// }
		// logger.Printf("Complete IMDS call response[network]: %v", macaddressData)

		// ncidData, ok2 := computeDoc["ncId"].(string)
		// if !ok2 {
		// 	return errors.New("unable to parse IMDS ncid metadata")
		// }
		// logger.Printf("Complete IMDS call response[network][macaddress]: %v", ncidData)

		vmUniqueIDUntyped := computeDoc[vmUniqueIDProperty]
		var ok bool
		vmUniqueID, ok = vmUniqueIDUntyped.(string)
		if !ok {
			return errors.New("unable to parse IMDS compute metadata, vmId property is not a string")
		}

		networkDoc, err := c.getInstanceInterfaceMacaddress(ctx)

		if err != nil {
			errors.Wrap(err, "error getting IMDS interface metadata")
		} else {
			for _, int := range networkDoc.Interface {
				logger.Printf("Complete IMDS call [macaddress]: %s, [ncId]: %s", int.MacAddress, int.NcID)
			}
		}

		return nil
	}, retry.Context(ctx), retry.Attempts(c.config.retryAttempts), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return "", errors.Wrap(err, "exhausted retries querying IMDS compute metadata")
	}

	if vmUniqueID == "" {
		return "", ErrVMUniqueIDNotFound
	}

	return vmUniqueID, nil
}

func (c *Client) getInstanceComputeMetadata(ctx context.Context) (map[string]any, error) {
	imdsComputeURL, err := url.JoinPath(c.config.endpoint, imdsComputePath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to build path to IMDS compute metadata")
	}
	imdsComputeURL = imdsComputeURL + "?" + imdsComputeAPIVersion + "&" + imdsFormatJSON

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imdsComputeURL, http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "error building IMDS http request")
	}

	// IMDS requires the "Metadata: true" header
	req.Header.Add(metadataHeaderKey, metadataHeaderValue)
	resp, err := c.cli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error querying IMDS")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Wrapf(ErrUnexpectedStatusCode, "unexpected status code %d", resp.StatusCode)
	}

	logger.Printf("Complete IMDS call response body: %v", resp.Body)

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, errors.Wrap(err, "error decoding IMDS response as json")
	}

	return m, nil
}

func (c *Client) getInstanceInterfaceMacaddress(ctx context.Context) (Network, error) {
	imdsComputeURL, err := url.JoinPath(c.config.endpoint, "/metadata/instance/network")
	if err != nil {
		return Network{}, errors.Wrap(err, "unable to build path to IMDS interface metadata")
	}
	imdsComputeURL = imdsComputeURL + "?" + imdsComputeAPIVersion + "&" + imdsFormatJSON

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imdsComputeURL, http.NoBody)
	if err != nil {
		return Network{}, errors.Wrap(err, "error building IMDS http request")
	}

	// IMDS requires the "Metadata: true" header
	req.Header.Add(metadataHeaderKey, metadataHeaderValue)
	resp, err := c.cli.Do(req)
	if err != nil {
		return Network{}, errors.Wrap(err, "error querying IMDS")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Network{}, errors.Wrapf(ErrUnexpectedStatusCode, "unexpected status code %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return Network{}, err
	}

	logger.Printf("Complete IMDS call response body: %v", body)

	var m Network
	if err := json.Unmarshal(body, &m); err != nil { // .NewDecoder(resp.Body).Decode(&m); err != nil {
		return Network{}, errors.Wrap(err, "error decoding IMDS response as json")
	}

	return m, nil
}
