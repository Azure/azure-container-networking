// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipamclient

import (
	"bytes"
	"encoding/json"
	"fmt"

	ipam "github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/log"
)

// old api from cnm
const (
	// Libnetwork IPAM plugin endpoint type
	EndpointType = "IpamDriver"

	// Libnetwork IPAM plugin remote API paths
	GetCapabilitiesPath  = "/IpamDriver.GetCapabilities"
	GetAddressSpacesPath = "/IpamDriver.GetDefaultAddressSpaces"
	RequestPoolPath      = "/IpamDrive r.RequestPool"
	ReleasePoolPath      = "/IpamDriver.ReleasePool"
	GetPoolInfoPath      = "/IpamDriver.GetPoolInfo"
	RequestAddressPath   = "/IpamDriver.RequestAddress"
	ReleaseAddressPath   = "/IpamDriver.ReleaseAddress"

	// Libnetwork IPAM plugin options
	OptAddressType        = "RequestAddressType"
	OptAddressTypeGateway = "com.docker.network.gateway"
)

// Request sent by libnetwork when querying plugin capabilities.
type GetCapabilitiesRequest struct{}

// Response sent by plugin when registering its capabilities with libnetwork.
type GetCapabilitiesResponse struct {
	Err                   string
	RequiresMACAddress    bool
	RequiresRequestReplay bool
}

// Request sent by libnetwork when querying the default address space names.
type GetDefaultAddressSpacesRequest struct{}

// Response sent by plugin when returning the default address space names.
type GetDefaultAddressSpacesResponse struct {
	Err                       string
	LocalDefaultAddressSpace  string
	GlobalDefaultAddressSpace string
}

// Request sent by libnetwork when acquiring a reference to an address pool.
type RequestPoolRequest struct {
	AddressSpace string
	Pool         string
	SubPool      string
	Options      map[string]string
	V6           bool
}

// Response sent by plugin when an address pool is successfully referenced.
type RequestPoolResponse struct {
	Err    string
	PoolID string
	Pool   string
	Data   map[string]string
}

// Request sent by libnetwork when releasing a previously registered address pool.
type ReleasePoolRequest struct {
	PoolID string
}

// Response sent by plugin when an address pool is successfully released.
type ReleasePoolResponse struct {
	Err string
}

// Request sent when querying address pool information.
type GetPoolInfoRequest struct {
	PoolID string
}

// Response sent by plugin when returning address pool information.
type GetPoolInfoResponse struct {
	Err                string
	Capacity           int
	Available          int
	UnhealthyAddresses []string
}

// Request sent by libnetwork when reserving an address from a pool.
type RequestAddressRequest struct {
	PoolID  string
	Address string
	Options map[string]string
}

// Response sent by plugin when an address is successfully reserved.
type RequestAddressResponse struct {
	Err     string
	Address string
	Data    map[string]string
}

// Request sent by libnetwork when releasing an address back to the pool.
type ReleaseAddressRequest struct {
	PoolID  string
	Address string
	Options map[string]string
}

// Response sent by plugin when an address is successfully released.
type ReleaseAddressResponse struct {
	Err string
}

// IpamClient specifies a client to connect to Ipam Plugin.
type IpamClient struct {
	connectionURL string
}

// NewIpamClient create a new ipam client.
func NewIpamClient(url string) (*IpamClient, error) {
	if url == "" {
		url = defaultIpamPluginURL
	}
	return &IpamClient{
		connectionURL: url,
	}, nil
}

// GetAddressSpace request to get address space ID.
func (ic *IpamClient) GetAddressSpace() (string, error) {
	log.Printf("[Azure CNS] GetAddressSpace Request")

	client, err := getClient(ic.connectionURL)
	if err != nil {
		return "", err
	}

	url := ic.connectionURL + GetAddressSpacesPath

	res, err := client.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode == 200 {
		var resp GetDefaultAddressSpacesResponse
		err := json.NewDecoder(res.Body).Decode(&resp)
		if err != nil {
			log.Printf("[Azure CNS] Error received while parsing GetAddressSpace response resp:%v err:%v", res.Body, err.Error())
			return "", err
		}

		if resp.Err != "" {
			log.Printf("[Azure CNS] GetAddressSpace received error response :%v", resp.Err)
			return "", fmt.Errorf(resp.Err)
		}

		return resp.LocalDefaultAddressSpace, nil
	}
	log.Printf("[Azure CNS] GetAddressSpace invalid http status code: %v err:%v", res.StatusCode, err.Error())
	return "", err
}

// GetPoolID Request to get poolID.
func (ic *IpamClient) GetPoolID(asID, subnet string) (string, error) {
	var body bytes.Buffer
	log.Printf("[Azure CNS] GetPoolID Request")

	client, err := getClient(ic.connectionURL)
	if err != nil {
		return "", err
	}

	url := ic.connectionURL + RequestPoolPath

	payload := &RequestPoolRequest{
		AddressSpace: asID,
		Pool:         subnet,
	}

	json.NewEncoder(&body).Encode(payload)

	res, err := client.Post(url, "application/json", &body)
	if err != nil {
		log.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode == 200 {
		var resp RequestPoolResponse
		err := json.NewDecoder(res.Body).Decode(&resp)
		if err != nil {
			log.Printf("[Azure CNS] Error received while parsing GetPoolID response resp:%v err:%v", res.Body, err.Error())
			return "", err
		}

		if resp.Err != "" {
			log.Printf("[Azure CNS] GetPoolID received error response :%v", resp.Err)
			return "", fmt.Errorf(resp.Err)
		}

		return resp.PoolID, nil
	}
	log.Printf("[Azure CNS] GetPoolID invalid http status code: %v err:%v", res.StatusCode, err.Error())
	return "", err
}

// ReserveIPAddress request an Ip address for the reservation id.
func (ic *IpamClient) ReserveIPAddress(poolID string, reservationID string) (string, error) {
	var body bytes.Buffer
	log.Printf("[Azure CNS] ReserveIpAddress")

	client, err := getClient(ic.connectionURL)
	if err != nil {
		return "", err
	}

	url := ic.connectionURL + RequestAddressPath

	payload := &RequestAddressRequest{
		PoolID:  poolID,
		Address: "",
		Options: make(map[string]string),
	}
	payload.Options[ipam.OptAddressID] = reservationID
	json.NewEncoder(&body).Encode(payload)

	res, err := client.Post(url, "application/json", &body)
	if err != nil {
		log.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode == 200 {
		var reserveResp RequestAddressResponse

		err = json.NewDecoder(res.Body).Decode(&reserveResp)
		if err != nil {
			log.Printf("[Azure CNS] Error received while parsing reserve response resp:%v err:%v", res.Body, err.Error())
			return "", err
		}

		if reserveResp.Err != "" {
			log.Printf("[Azure CNS] ReserveIP received error response :%v", reserveResp.Err)
			return "", fmt.Errorf(reserveResp.Err)
		}

		return reserveResp.Address, nil
	}

	log.Printf("[Azure CNS] ReserveIp invalid http status code: %v err:%v", res.StatusCode, err.Error())
	return "", err
}

// ReleaseIPAddress release an IP address for the reservation id.
func (ic *IpamClient) ReleaseIPAddress(poolID string, reservationID string) error {
	var body bytes.Buffer
	log.Printf("[Azure CNS] ReleaseIPAddress")

	client, err := getClient(ic.connectionURL)
	if err != nil {
		return err
	}

	url := ic.connectionURL + ReleaseAddressPath

	payload := &ReleaseAddressRequest{
		PoolID:  poolID,
		Address: "",
		Options: make(map[string]string),
	}

	payload.Options[ipam.OptAddressID] = reservationID

	json.NewEncoder(&body).Encode(payload)

	res, err := client.Post(url, "application/json", &body)
	if err != nil {
		log.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return err
	}

	defer res.Body.Close()

	if res.StatusCode == 200 {
		var releaseResp ReleaseAddressResponse
		err := json.NewDecoder(res.Body).Decode(&releaseResp)
		if err != nil {
			log.Printf("[Azure CNS] Error received while parsing release response :%v err:%v", res.Body, err.Error())
			return err
		}

		if releaseResp.Err != "" {
			log.Printf("[Azure CNS] ReleaseIP received error response :%v", releaseResp.Err)
			return fmt.Errorf(releaseResp.Err)
		}

		return nil
	}
	log.Printf("[Azure CNS] ReleaseIP invalid http status code: %v", res.StatusCode)
	return err
}

// GetIPAddressUtilization - returns number of available, reserved and unhealthy addresses list.
func (ic *IpamClient) GetIPAddressUtilization(poolID string) (int, int, []string, error) {
	var body bytes.Buffer
	log.Printf("[Azure CNS] GetIPAddressUtilization")

	client, err := getClient(ic.connectionURL)
	if err != nil {
		return 0, 0, nil, err
	}
	url := ic.connectionURL + GetPoolInfoPath

	payload := &GetPoolInfoRequest{
		PoolID: poolID,
	}

	json.NewEncoder(&body).Encode(payload)

	res, err := client.Post(url, "application/json", &body)
	if err != nil {
		log.Printf("[Azure CNS] HTTP Post returned error %v", err.Error())
		return 0, 0, nil, err
	}

	defer res.Body.Close()

	if res.StatusCode == 200 {
		var poolInfoResp GetPoolInfoResponse
		err := json.NewDecoder(res.Body).Decode(&poolInfoResp)
		if err != nil {
			log.Printf("[Azure CNS] Error received while parsing GetIPUtilization response :%v err:%v", res.Body, err.Error())
			return 0, 0, nil, err
		}

		if poolInfoResp.Err != "" {
			log.Printf("[Azure CNS] GetIPUtilization received error response :%v", poolInfoResp.Err)
			return 0, 0, nil, fmt.Errorf(poolInfoResp.Err)
		}

		return poolInfoResp.Capacity, poolInfoResp.Available, poolInfoResp.UnhealthyAddresses, nil
	}
	log.Printf("[Azure CNS] GetIPUtilization invalid http status code: %v err:%v", res.StatusCode, err.Error())
	return 0, 0, nil, err
}
