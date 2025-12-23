package restserver

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Microsoft/hcsshim"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
)

const (
	// timeout for powershell command to return the interfaces list
	pwshTimeout             = 120 * time.Second
	hnsRegistryPath         = `SYSTEM\CurrentControlSet\Services\HNS\wcna_state\config`
	prefixOnNicRegistryPath = `SYSTEM\CurrentControlSet\Services\HNS\wcna_state\config\PrefixOnNic`
	infraNicIfName          = "eth0"
)

var (
	errUnsupportedAPI       = errors.New("unsupported api")
	errIntOverflow          = errors.New("int value overflows uint32")
	errUnsupportedValueType = errors.New("unsupported value type for registry key")
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

func (service *HTTPRestService) enablePrefixOnNic(isEnabled bool) error {
	return service.setRegistryValue(prefixOnNicRegistryPath, "enabled", isEnabled)
}

func (service *HTTPRestService) setInfraNicMacAddress(macAddress string) error {
	return service.setRegistryValue(prefixOnNicRegistryPath, "infra_nic_mac_address", macAddress)
}

func (service *HTTPRestService) setInfraNicIfName(ifName string) error {
	return service.setRegistryValue(prefixOnNicRegistryPath, "infra_nic_ifname", ifName)
}

func (service *HTTPRestService) setEnableSNAT(isEnabled bool) error {
	return service.setRegistryValue(hnsRegistryPath, "EnableSNAT", isEnabled)
}

func (service *HTTPRestService) setPrefixOnNICRegistry(enablePrefixOnNic bool, infraNicMacAddress string) error {
	if err := service.enablePrefixOnNic(enablePrefixOnNic); err != nil {
		return fmt.Errorf("failed to set enablePrefixOnNic key to windows registry: %w", err)
	}

	if err := service.setInfraNicMacAddress(infraNicMacAddress); err != nil {
		return fmt.Errorf("failed to set InfraNicMacAddress key to windows registry: %w", err)
	}

	if err := service.setInfraNicIfName(infraNicIfName); err != nil {
		return fmt.Errorf("failed to set InfraNicIfName key to windows registry: %w", err)
	}

	if err := service.setEnableSNAT(!enablePrefixOnNic); err != nil { // for prefix on nic,  snat should be disabled
		return fmt.Errorf("failed to set EnableSNAT key to windows registry: %w", err)
	}

	return nil
}

func (service *HTTPRestService) setRegistryValue(registryPath, keyName string, value interface{}) error {
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
	fmt.Printf("[setRegistryValue] Set %s\\%s = %v\n", registryPath, keyName, value)
	return nil
}
