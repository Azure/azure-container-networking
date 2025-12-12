// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package imds

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

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
	vmUniqueIDProperty         = "vmId"
	imdsComputePath            = "/metadata/instance/compute"
	imdsNetworkPath            = "/metadata/instance/network"
	imdsVersionsPath           = "/metadata/versions"
	imdsDefaultAPIVersion      = "api-version=2021-01-01"
	imdsNCDetailsVersion       = "api-version=2025-07-24"
	imdsMACAddressStringLength = 12 // 6 bytes in hex equals 12 characters
	imdsFormatJSON             = "format=json"
	metadataHeaderKey          = "Metadata"
	metadataHeaderValue        = "true"
	defaultRetryAttempts       = 3
	defaultIMDSEndpoint        = "http://169.254.169.254"
)

var (
	ErrVMUniqueIDNotFound   = errors.New("vm unique ID not found")
	ErrUnexpectedStatusCode = errors.New("imds returned an unexpected status code")
)

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
		computeDoc, err := c.getInstanceMetadata(ctx, imdsComputePath, imdsDefaultAPIVersion)
		if err != nil {
			return errors.Wrap(err, "error getting IMDS compute metadata")
		}
		vmUniqueIDUntyped := computeDoc[vmUniqueIDProperty]
		var ok bool
		vmUniqueID, ok = vmUniqueIDUntyped.(string)
		if !ok {
			return errors.New("unable to parse IMDS compute metadata, vmId property is not a string")
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

func (c *Client) GetNetworkInterfaces(ctx context.Context) ([]NetworkInterface, error) {
	var networkData NetworkInterfaces
	err := retry.Do(func() error {
		networkInterfaces, err := c.getInstanceMetadata(ctx, imdsNetworkPath, imdsNCDetailsVersion)
		if err != nil {
			return errors.Wrap(err, "error getting IMDS network metadata")
		}

		// Parse the network metadata to the expected structure
		jsonData, err := json.Marshal(networkInterfaces)
		if err != nil {
			return errors.Wrap(err, "error marshaling network metadata")
		}

		if err := json.Unmarshal(jsonData, &networkData); err != nil {
			return errors.Wrap(err, "error unmarshaling network metadata")
		}
		return nil
	}, retry.Context(ctx), retry.Attempts(c.config.retryAttempts), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return nil, errors.Wrap(err, "external call failed")
	}

	return networkData.Interface, nil
}

func (c *Client) getInstanceMetadata(ctx context.Context, imdsMetadataPath, imdsAPIVersion string) (map[string]any, error) {
	imdsRequestURL, err := url.JoinPath(c.config.endpoint, imdsMetadataPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to build path to IMDS metadata for path"+imdsMetadataPath)
	}
	imdsRequestURL = imdsRequestURL + "?" + imdsAPIVersion + "&" + imdsFormatJSON

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imdsRequestURL, http.NoBody)
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

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, errors.Wrap(err, "error decoding IMDS response as json")
	}

	return m, nil
}

func (c *Client) GetIMDSVersions(ctx context.Context) (*APIVersionsResponse, error) {
	var versionsResp APIVersionsResponse
	err := retry.Do(func() error {
		// Build the URL for the versions endpoint
		imdsRequestURL, err := url.JoinPath(c.config.endpoint, imdsVersionsPath)
		if err != nil {
			return errors.Wrap(err, "unable to build path to IMDS versions endpoint")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, imdsRequestURL, http.NoBody)
		if err != nil {
			return errors.Wrap(err, "error building IMDS versions http request")
		}

		req.Header.Add(metadataHeaderKey, metadataHeaderValue)
		resp, err := c.cli.Do(req)
		if err != nil {
			return errors.Wrap(err, "error querying IMDS versions API")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errors.Wrapf(ErrUnexpectedStatusCode, "unexpected status code %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&versionsResp); err != nil {
			return errors.Wrap(err, "error decoding IMDS versions response as json")
		}

		return nil
	}, retry.Context(ctx), retry.Attempts(c.config.retryAttempts), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return nil, errors.Wrap(err, "exhausted retries querying IMDS versions")
	}

	return &versionsResp, nil
}

// Required for  marshaling/unmarshaling of mac address
type HardwareAddr net.HardwareAddr

func (h *HardwareAddr) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(net.HardwareAddr(*h).String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal hardware address")
	}
	return data, nil
}

func (h *HardwareAddr) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return errors.Wrap(err, "failed to unmarshal JSON data")
	}

	mac, err := parseMacAddress(s)
	if err != nil {
		return errors.Wrap(err, "failed to parse MAC address")
	}
	*h = HardwareAddr(mac)
	return nil
}

// parseMacAddress is a wrapper around net.ParseMAC to handle Windows MAC address. Windows MAC address is a pure hex
// dump without delimiter, so we need to add delimiters. This happens when CNS gets MAC address from IMDS.
func parseMacAddress(s string) (net.HardwareAddr, error) {
	if !strings.ContainsAny(s, ":-.") && len(s) == imdsMACAddressStringLength {
		var sb strings.Builder
		for i := 0; i < len(s); i += 2 {
			if i > 0 {
				sb.WriteByte(':')
			}
			sb.WriteString(s[i : i+2])
		}
		s = sb.String()
	}

	mac, err := net.ParseMAC(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse MAC address")
	}
	return mac, nil
}

func (h *HardwareAddr) String() string {
	return net.HardwareAddr(*h).String()
}

// NetworkInterface represents a network interface from IMDS
type NetworkInterface struct {
	// IMDS returns compartment fields - these are mapped to NC ID and NC version
	MacAddress             HardwareAddr `json:"macAddress"`
	InterfaceCompartmentID string       `json:"interfaceCompartmentID,omitempty"`
}

// NetworkInterfaces represents the network interfaces from IMDS
type NetworkInterfaces struct {
	Interface []NetworkInterface `json:"interface"`
}

// APIVersionsResponse represents versions form IMDS
type APIVersionsResponse struct {
	APIVersions []string `json:"apiVersions"`
}
