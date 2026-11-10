// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package mellanox

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	// Search string to find adapter having Mellanox in description
	mellanoxSearchString = "*Mellanox*"

	// PriorityVlanTag reg key for adapter
	priorityVLANTagIdentifier = "*PriorityVLANTag"

	// Registry key Path Prefix
	registryKeyPrefix = "System\\CurrentControlSet\\Control\\Class\\"
	networkClassPath  = registryKeyPrefix + "{4d36e972-e325-11ce-bfc1-08002be10318}"
)

var (
	errorMellanoxAdapterNotFound = fmt.Errorf("no network adapter found with %s in description", mellanoxSearchString)
	errorMellanoxDeviceNotFound  = fmt.Errorf("no network device found with %s in description", mellanoxSearchString)
	errorPowershellNotFound      = fmt.Errorf("failed to find powershell executable")
)

type Mellanox struct{}

// GetAdapter returns name of Mellanox adapter if found
// Returns errorMellanoxAdapterNotFound if adapter is not found or adapter name empty
func (m *Mellanox) GetAdapterName() (string, error) {
	// get mellanox adapter name
	cmd := fmt.Sprintf(`Get-NetAdapter | Where-Object { $_.InterfaceDescription -like '%s' } | Select-Object -ExpandProperty Name`, mellanoxSearchString)
	adapterName, err := executePowershellCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("error while executing powershell command to get net adapter list: %w", err)
	}
	if adapterName == "" {
		return "", errorMellanoxAdapterNotFound
	}
	return adapterName, nil
}

// Set Mellanox adapter's PriorityVLANTag value to desired value if adapter exists
// 5/16/23 : right now setting desired reg key value for PriorityVLANTag = 3  --> Packet priority and VLAN enabled
// for more details goto https://docs.nvidia.com/networking/display/winof2v230/Configuring+the+Driver+Registry+Keys#ConfiguringtheDriverRegistryKeys-GeneralRegistryKeysGeneralRegistryKeys
func (m *Mellanox) SetPriorityVLANTag(desiredVal int) error {
	adapterName, err := m.GetAdapterName()
	if err != nil {
		return fmt.Errorf("failed to find mellanox adapter: %w", err)
	}

	// Find if adapter has property PriorityVLANTag (version 4 or up) or not (version 3)
	cmd := fmt.Sprintf(`Get-NetAdapterAdvancedProperty | Where-Object { $_.RegistryKeyword -like '%s' -and $_.Name -eq '%s' } | Select-Object -ExpandProperty Name`,
		priorityVLANTagIdentifier, adapterName)
	adapterNameWithVLANTag, err := executePowershellCommand(cmd)
	if err != nil {
		return fmt.Errorf("error while executing powershell command to get VLAN Tag advance property of %s: %w", adapterName, err)
	}

	if adapterNameWithVLANTag != "" {
		return m.setMellanoxPriorityVLANTagValueForV4(adapterNameWithVLANTag, desiredVal)
	}
	return m.setMellanoxPriorityVLANTagValueForV3(adapterName, desiredVal)
}

// Get PriorityVLANTag returns PriorityVLANTag value for Mellanox Adapter (both version 3 and version 4)
func (m *Mellanox) GetPriorityVLANTag() (int, error) {
	adapterName, err := m.GetAdapterName()
	if err != nil {
		return 0, fmt.Errorf("failed to find mellanox adapter: %w", err)
	}

	// Fall back to the provider when the registry value cannot be mapped unambiguously.
	if value, err := readAdapterVLANTag(adapterName); err == nil {
		return value, nil
	}

	// Find if adapter has property PriorityVLANTag (version 4 or up) or not (version 3)
	cmd := fmt.Sprintf(`Get-NetAdapterAdvancedProperty | Where-Object { $_.RegistryKeyword -like '%s' -and $_.Name -eq '%s' } | Select-Object -ExpandProperty Name`,
		priorityVLANTagIdentifier, adapterName)
	adapterNameWithVLANTag, err := executePowershellCommand(cmd)
	if err != nil {
		return 0, fmt.Errorf("error while executing powershell command to get VLAN Tag advance property of %s: %w", adapterName, err)
	}

	if adapterNameWithVLANTag != "" {
		return m.getMellanoxPriorityVLANTagValueForV4(adapterNameWithVLANTag)
	}

	return m.getMellanoxPriorityVLANTagValueForV3()
}

func readAdapterVLANTag(adapterName string) (int, error) {
	iface, err := net.InterfaceByName(adapterName)
	if err != nil {
		return 0, err
	}
	row := windows.MibIfRow2{InterfaceIndex: uint32(iface.Index)}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, &row); err != nil {
		return 0, err
	}
	if !strings.EqualFold(windows.UTF16ToString(row.Alias[:]), adapterName) {
		return 0, fmt.Errorf("adapter name changed")
	}

	path, err := adapterRegistryPath(registry.LOCAL_MACHINE, networkClassPath, row.InterfaceGuid.String())
	if err != nil {
		return 0, err
	}
	if err := checkVLANTagKeyword(registry.LOCAL_MACHINE, path); err != nil {
		return 0, err
	}
	return readRegistryVLANTag(registry.LOCAL_MACHINE, path)
}

func adapterRegistryPath(root registry.Key, classPath, guid string) (string, error) {
	class, err := registry.OpenKey(root, classPath, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return "", err
	}
	defer class.Close()
	names, err := class.ReadSubKeyNames(-1)
	if err != nil {
		return "", err
	}

	var path string
	for _, name := range names {
		if len(name) != 4 || strings.Trim(name, "0123456789") != "" {
			continue
		}
		key, err := registry.OpenKey(class, name, registry.QUERY_VALUE)
		if err != nil {
			return "", err
		}
		instanceID, _, err := key.GetStringValue("NetCfgInstanceId")
		key.Close()
		if err == registry.ErrNotExist {
			continue
		}
		if err != nil {
			return "", err
		}
		if strings.EqualFold(instanceID, guid) {
			if path != "" {
				return "", fmt.Errorf("multiple registry keys for adapter %s", guid)
			}
			path = classPath + `\` + name
		}
	}
	if path == "" {
		return "", fmt.Errorf("registry key not found for adapter %s", guid)
	}
	return path, nil
}

func checkVLANTagKeyword(root registry.Key, path string) error {
	key, err := registry.OpenKey(root, path+`\Ndi\params`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return err
	}
	defer key.Close()
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if strings.HasSuffix(strings.ToLower(name), "priorityvlantag") && !strings.EqualFold(name, priorityVLANTagIdentifier) {
			return fmt.Errorf("multiple PriorityVLANTag keywords")
		}
	}
	return nil
}

// Checks if a Mellanox adapter's PriorityVLANTag value
// for version 4 and up is set to the given expected value
func (m *Mellanox) getMellanoxPriorityVLANTagValueForV4(adapterName string) (int, error) {
	cmd := fmt.Sprintf(
		`Get-NetAdapterAdvancedProperty | Where-Object { $_.RegistryKeyword -like '%s' -and $_.Name -eq '%s' } | Select-Object -ExpandProperty RegistryValue`,
		priorityVLANTagIdentifier, adapterName)

	regvalue, err := executePowershellCommand(cmd)
	if err != nil {
		return 0, err
	}

	intValue, err := strconv.Atoi(regvalue)
	if err != nil {
		return 0, fmt.Errorf("failed to convert PriorityVLANTag value to integer: %w", err)
	}

	return intValue, nil
}

// Checks if a Mellanox adapter's PriorityVLANTag value
// for version 3 and below is set to the given expected value
func (m *Mellanox) getMellanoxPriorityVLANTagValueForV3() (int, error) {
	registryKeyFullPath, err := m.getRegistryFullPath()
	if err != nil {
		return 0, err
	}

	intValue, err := readRegistryVLANTag(registry.LOCAL_MACHINE, registryKeyFullPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PriorityVLANTag: %w", err)
	}

	return intValue, nil
}

func readRegistryVLANTag(root registry.Key, path string) (int, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer key.Close()

	value, valueType, err := key.GetStringValue(priorityVLANTagIdentifier)
	if err == registry.ErrUnexpectedType && (valueType == registry.DWORD || valueType == registry.QWORD) {
		number, _, numberErr := key.GetIntegerValue(priorityVLANTagIdentifier)
		if numberErr != nil {
			return 0, numberErr
		}
		value = strconv.FormatUint(number, 10)
	} else if err == registry.ErrUnexpectedType && valueType == registry.MULTI_SZ {
		values, _, valuesErr := key.GetStringsValue(priorityVLANTagIdentifier)
		if valuesErr != nil {
			return 0, valuesErr
		}
		value = strings.Join(values, "\r\n")
	} else if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(value))
}

func writeRegistryVLANTag(root registry.Key, path string, value int) error {
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	return key.SetStringValue(priorityVLANTagIdentifier, strconv.Itoa(value))
}

// adapter is version 4 and up since adapter's advance property consists of reg key : PriorityVLANTag
// set reg value for Priorityvlantag of adapter to 3 if not set already
func (m *Mellanox) setMellanoxPriorityVLANTagValueForV4(adapterName string, desiredVal int) error {
	cmd := fmt.Sprintf(
		`Set-NetAdapterAdvancedProperty -Name '%s' -RegistryKeyword '%s' -RegistryValue %d`,
		adapterName, priorityVLANTagIdentifier, desiredVal)
	_, err := executePowershellCommand(cmd)
	if err != nil {
		return fmt.Errorf("error while setting up registry value for PriorityVLANTag for adapter: %w", err)
	}

	log.Printf("Successfully set Mellanox Network Adapter: %s with %s property value as %d",
		adapterName, priorityVLANTagIdentifier, desiredVal)
	return nil
}

// Adapter is version 3 or less as PriorityVLANTag was not found in advanced properties of mellanox adapter
func (m *Mellanox) setMellanoxPriorityVLANTagValueForV3(adapterName string, desiredVal int) error {
	registryKeyFullPath, err := m.getRegistryFullPath()
	if err != nil {
		return err
	}

	if err := writeRegistryVLANTag(registry.LOCAL_MACHINE, registryKeyFullPath, desiredVal); err != nil {
		return fmt.Errorf("failed to set PriorityVLANTag for adapter %s: %w", adapterName, err)
	}

	log.Printf("Restarting Mellanox network adapter for regkey change to take effect")
	cmd := fmt.Sprintf(`Restart-NetAdapter -Name '%s'`, adapterName)
	_, err = executePowershellCommand(cmd)
	if err != nil {
		return fmt.Errorf("error while executing powershell command to restart net adapter  %s: %w", adapterName, err)
	}
	log.Printf("For Mellanox CX-3 adapters, the reg key set to %d", desiredVal)
	return nil
}

// Get registry full path for Mellanox Adapter
func (m *Mellanox) getRegistryFullPath() (string, error) {
	log.Printf("Searching through CIM instances for Network devices with %s in the name", mellanoxSearchString)
	cmd := fmt.Sprintf(
		`Get-CimInstance -Namespace root/cimv2 -ClassName Win32_PNPEntity | Where-Object PNPClass -EQ "Net" | Where-Object { $_.Name -like '%s' } | Select-Object -ExpandProperty DeviceID`,
		mellanoxSearchString)
	deviceid, err := executePowershellCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("error while executing powershell command to get device id for Mellanox: %w", err)
	}
	if deviceid == "" {
		return "", errorMellanoxDeviceNotFound
	}

	cmd = fmt.Sprintf(`Get-PnpDeviceProperty -InstanceId '%s' | Where-Object KeyName -EQ "DEVPKEY_Device_Driver" | Select-Object -ExpandProperty Data`, deviceid)
	registryKeySuffix, err := executePowershellCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("error while executing powershell command to get registry suffix of device id %s: %w", deviceid, err)
	}

	return registryKeyPrefix + registryKeySuffix, nil
}

// executePowershellCommand executes powershell command
func executePowershellCommand(command string) (string, error) {
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return "", errorPowershellNotFound
	}

	log.Printf("[Azure-Utils] %s", command)

	cmd := exec.Command(ps, command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s:%w", stderr.String(), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}
