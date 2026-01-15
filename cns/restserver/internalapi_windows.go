package restserver

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Microsoft/hcsshim"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
)

const (
	// timeout for powershell command to return the interfaces list
	pwshTimeout             = 120 * time.Second
	hnsRegistryPath         = `SYSTEM\CurrentControlSet\Services\HNS\wcna_state\config`
	prefixOnNicRegistryPath = `SYSTEM\CurrentControlSet\Services\HNS\wcna_state\config\PrefixOnNic`
)

var (
	errUnsupportedAPI       = errors.New("unsupported api")
	errIntOverflow          = errors.New("int value overflows uint32")
	errUnsupportedValueType = errors.New("unsupported value type for registry key")
	errEmptyMACAddress      = errors.New("empty MAC address")
)

type IPtablesProvider struct{}

func (*IPtablesProvider) GetIPTables() (iptablesClient, error) {
	return nil, errUnsupportedAPI
}

func (*IPtablesProvider) GetIPTablesLegacy() (iptablesLegacyClient, error) {
	return nil, errUnsupportedAPI
}

// nolint
func (service *HTTPRestService) programSNATRules(req *cns.CreateNetworkContainerRequest) (types.ResponseCode, string) {
	return types.Success, ""
}

// setVFForAccelnetNICs is used in SWIFTV2 mode to set VF on accelnet nics
func (service *HTTPRestService) setVFForAccelnetNICs() error {
	// supply the primary MAC address to HNS api
	macAddress, err := service.getPrimaryNICMACAddress()
	if err != nil {
		return err
	}
	macAddresses := []string{macAddress}
	if _, err := hcsshim.SetNnvManagementMacAddresses(macAddresses); err != nil {
		return errors.Wrap(err, "Failed to set primary NIC MAC address")
	}
	return nil
}

// getPrimaryNICMacAddress fetches the MAC address of the primary NIC on the node.
func (service *HTTPRestService) getPrimaryNICMACAddress() (string, error) {
	// Create a new context and add a timeout to it
	ctx, cancel := context.WithTimeout(context.Background(), pwshTimeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	res, err := service.wscli.GetInterfaces(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to find primary interface info: %w", err)
	}
	var macAddress string
	for _, i := range res.Interface {
		// skip if not primary
		if !i.IsPrimary {
			continue
		}
		// skip if no subnets
		if len(i.IPSubnet) == 0 {
			continue
		}
		macAddress = i.MacAddress
	}

	if macAddress == "" {
		return "", errors.New("MAC address not found(empty) from wireserver")
	}
	return macAddress, nil
}

// isSwiftV2PrefixOnNicEnabled checks if the specified NC has SwiftV2PrefixOnNic enabled.
func (service *HTTPRestService) isSwiftV2PrefixOnNicEnabled(ncID string) bool {
	if service.state == nil {
		return false
	}
	if ncStatus, exists := service.state.ContainerStatus[ncID]; exists {
		return ncStatus.CreateNetworkContainerRequest.SwiftV2PrefixOnNic
	}
	return false
}

func (service *HTTPRestService) processIMDSData(networkInterfaces []imds.NetworkInterface) map[string]string {
	ncs := make(map[string]string)
	var infraNicMacAddress string
	var isSwiftv2PrefixOnNic bool
	// IMDS returns networkInterfaces that has delegated nic {nc id, mac address}, infra nic {empty nc id, mac address}.
	for _, iface := range networkInterfaces {
		ncID := iface.InterfaceCompartmentID
		if ncID != "" {
			ncs[ncID] = PrefixOnNicNCVersion
			// Check if this NC has SwiftV2PrefixOnNic enabled
			if service.isSwiftV2PrefixOnNicEnabled(ncID) {
				isSwiftv2PrefixOnNic = true
			}
		} else {
			infraNicMacAddress = iface.MacAddress.String()
		}
	}
	if infraNicMacAddress != "" {
		go func() {
			// Get the interface name from the MAC address
			infraNicIfName, err := service.getInterfaceNameFromMAC(infraNicMacAddress)
			if err != nil {
				//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
				logger.Errorf("[Windows] Failed to get interface name from MAC address to set for windows registry: %v", err)
				return
			}
			// Process Windows registry keys with the retrieved MAC address and interface name. It is required for HNS team to configure cilium routes specific to windows nodes
			if err := service.setRegistryKeysForPrefixOnNic(isSwiftv2PrefixOnNic, infraNicMacAddress, infraNicIfName); err != nil {
				//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
				logger.Errorf("[Windows] Failed to set registry keys: %v", err)
				return
			}
		}()
	}
	return ncs
}

// getInterfaceNameFromMAC retrieves the network interface name given its MAC address.
func (service *HTTPRestService) getInterfaceNameFromMAC(macAddress string) (string, error) {
	macFormatted := strings.ReplaceAll(strings.ReplaceAll(macAddress, ":", ""), "-", "")
	if macFormatted == "" {
		return "", fmt.Errorf("failed to parse MAC address: %w", errEmptyMACAddress)
	}

	macBytes, err := hex.DecodeString(macFormatted)
	if err != nil {
		return "", fmt.Errorf("failed to parse MAC address %s: %w", macAddress, err)
	}

	iface, err := (&netio.NetIO{}).GetNetworkInterfaceByMac(net.HardwareAddr(macBytes))
	if err != nil {
		return "", fmt.Errorf("failed to find interface with MAC address %s: %w", macAddress, err)
	}
	return iface.Name, nil
}

// setRegistryKeysForPrefixOnNic configures Windows registry keys for prefix-on-NIC scenarios.
func (service *HTTPRestService) setRegistryKeysForPrefixOnNic(enablePrefixOnNic bool, infraNicMacAddress, infraNicIfName string) error {
	// HNS looks for specific keywords in registry, setting them here
	if err := setRegistryValue(prefixOnNicRegistryPath, "enabled", enablePrefixOnNic); err != nil {
		return fmt.Errorf("failed to set enablePrefixOnNic key to windows registry: %w", err)
	}

	if err := setRegistryValue(prefixOnNicRegistryPath, "infra_nic_mac_address", infraNicMacAddress); err != nil {
		return fmt.Errorf("failed to set InfraNicMacAddress key to windows registry: %w", err)
	}

	if err := setRegistryValue(prefixOnNicRegistryPath, "infra_nic_ifname", infraNicIfName); err != nil {
		return fmt.Errorf("failed to set InfraNicIfName key to windows registry: %w", err)
	}

	if err := setRegistryValue(hnsRegistryPath, "EnableSNAT", !enablePrefixOnNic); err != nil { // for prefix on nic,  snat should be disabled
		return fmt.Errorf("failed to set EnableSNAT key to windows registry: %w", err)
	}

	return nil
}

// setRegistryValue writes a value to the Windows registry.
func setRegistryValue(registryPath, keyName string, value interface{}) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, registryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to create/open registry key %s: %w", registryPath, err)
	}
	defer key.Close()

	switch v := value.(type) {
	case string:
		err = key.SetStringValue(keyName, v)
	case bool:
		dwordValue := uint32(0)
		if v {
			dwordValue = 1
		}
		err = key.SetDWordValue(keyName, dwordValue)
	case uint32:
		err = key.SetDWordValue(keyName, v)
	case int:
		if v < 0 || v > math.MaxUint32 {
			return fmt.Errorf("%w: %d for registry key %s", errIntOverflow, v, keyName)
		}
		err = key.SetDWordValue(keyName, uint32(v))
	default:
		return fmt.Errorf("%w %s: %T", errUnsupportedValueType, keyName, value)
	}
	if err != nil {
		return fmt.Errorf("failed to set registry value '%s': %w", keyName, err)
	}
	return nil
}
