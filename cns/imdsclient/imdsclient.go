package imdsclient

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/pkg/errors"
)

var (
	// ErrNoPrimaryInterface indicates the imds respnose does not have a primary interface indicated.
	ErrNoPrimaryInterface = errors.New("no primary interface found")
	// ErrInsufficientAddressSpace indicates that the CIDR space is too small to include a gateway IP; it is 1 IP.
	ErrInsufficientAddressSpace = errors.New("insufficient address space to generate gateway IP")
)

// GetNetworkContainerInfoFromHost retrieves the programmed version of network container from Host.
func (imdsClient *ImdsClient) GetNetworkContainerInfoFromHost(networkContainerID string, primaryAddress string, authToken string, apiVersion string) (*ContainerVersion, error) {
	logger.Printf("[Azure CNS] GetNetworkContainerInfoFromHost")
	queryURL := fmt.Sprintf(hostQueryURLForProgrammedVersion,
		primaryAddress, networkContainerID, authToken, apiVersion)

	logger.Printf("[Azure CNS] Going to query Azure Host for container version @\n %v\n", queryURL)
	jsonResponse, err := http.Get(queryURL)
	if err != nil {
		return nil, err
	}

	defer jsonResponse.Body.Close()

	logger.Printf("[Azure CNS] Response received from Azure Host for NetworkManagement/interfaces: %v", jsonResponse.Body)

	var response containerVersionJsonResponse
	err = json.NewDecoder(jsonResponse.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	ret := &ContainerVersion{
		NetworkContainerID: response.NetworkContainerID,
		ProgrammedVersion:  response.ProgrammedVersion,
	}

	return ret, nil
}

// GetPrimaryInterfaceInfoFromHost retrieves subnet and gateway of primary NIC from Host.
// TODO(rbtr): this is not a good client contract, we should return the resp.
func (imdsClient *ImdsClient) GetPrimaryInterfaceInfoFromHost() (*InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromHost")

	interfaceInfo := &InterfaceInfo{}
	resp, err := http.Get(hostQueryURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	logger.Printf("[Azure CNS] Response received from NMAgent for get interface details: %s", string(b))

	var doc xmlDocument
	if err := xml.NewDecoder(bytes.NewReader(b)).Decode(&doc); err != nil {
		return nil, errors.Wrap(err, "failed to decode response body")
	}

	foundPrimaryInterface := false

	// For each interface.
	for _, i := range doc.Interface {
		// skip if not primary
		if !i.IsPrimary {
			continue
		}
		interfaceInfo.IsPrimary = true

		// Get the first subnet.
		for _, s := range i.IPSubnet {
			interfaceInfo.Subnet = s.Prefix
			gw, err := calculateGatewayIP(s.Prefix)
			if err != nil {
				return nil, err
			}
			interfaceInfo.Gateway = gw.String()
			for _, ip := range s.IPAddress {
				if ip.IsPrimary {
					interfaceInfo.PrimaryIP = ip.Address
				}
			}

			imdsClient.primaryInterface = interfaceInfo
			break
		}

		foundPrimaryInterface = true
		break
	}

	if !foundPrimaryInterface {
		return nil, ErrNoPrimaryInterface
	}

	return interfaceInfo, nil
}

// calculateGatewayIP parses the passed CIDR string and returns the first IP in the range.
func calculateGatewayIP(cidr string) (net.IP, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, errors.Wrap(err, "received malformed subnet from host")
	}

	// check if we have enough address space to calculate a gateway IP
	// we need at least 2 IPs (eg the IPv4 mask cannot be greater than 31)
	// since the zeroth is reserved and the gateway is the first.
	mask, bits := subnet.Mask.Size()
	if mask == bits {
		return nil, ErrInsufficientAddressSpace
	}

	// the subnet IP is the zero base address, so we need to increment it by one to get the gateway.
	gw := make([]byte, len(subnet.IP))
	copy(gw, subnet.IP)
	for idx := len(gw) - 1; idx >= 0; idx-- {
		gw[idx]++
		// net.IP is a binary byte array, check if we have overflowed and need to continue incrementing to the left
		// along the arary or if we're done.
		// it's like if we have a 9 in base 10, and add 1, it rolls over to 0 so we're not done - we need to move
		// left and increment that digit also.
		if gw[idx] != 0 {
			break
		}
	}
	return gw, nil
}

// GetPrimaryInterfaceInfoFromMemory retrieves subnet and gateway of primary NIC that is saved in memory.
func (imdsClient *ImdsClient) GetPrimaryInterfaceInfoFromMemory() (*InterfaceInfo, error) {
	logger.Printf("[Azure CNS] GetPrimaryInterfaceInfoFromMemory")

	var iface *InterfaceInfo
	var err error
	if imdsClient.primaryInterface == nil {
		logger.Debugf("Azure-CNS] Primary interface in memory does not exist. Will get it from Host.")
		iface, err = imdsClient.GetPrimaryInterfaceInfoFromHost()
		if err != nil {
			logger.Errorf("[Azure-CNS] Unable to retrive primary interface info.")
		} else {
			logger.Debugf("Azure-CNS] Primary interface received from HOST: %+v.", iface)
		}
	} else {
		iface = imdsClient.primaryInterface
	}

	return iface, err
}

// GetNetworkContainerInfoFromHostWithoutToken is a temp implementation which will be removed once background thread
// updating host version is ready. Return max integer value to regress current AKS scenario
func (imdsClient *ImdsClient) GetNetworkContainerInfoFromHostWithoutToken() int {
	logger.Printf("[Azure CNS] GetNMagentVersionFromNMAgent")

	return math.MaxInt64
}
