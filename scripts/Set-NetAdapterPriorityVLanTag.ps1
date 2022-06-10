# This script will set the PriorityVLANTag registry key on a multitenant windows VM
# This is needed for VLAN tagging to be honored so that SWIFT packets flow out properly to the Azure Host

function Set-NetAdapterPriorityVLanTag
{
    New-Variable -Name PriorityVLANTagIdentifier -Value "*PriorityVLANTag" -Option ReadOnly
    New-Variable -Name MellanoxSearchString -Value "*Mellanox*" -Option ReadOnly
    New-Variable -Name RegistryKeyPrefix -Value "HKLM:\System\CurrentControlSet\Control\Class\" -Option ReadOnly

    Write-Host "Searching for a network adapter with '$MellanoxSearchString' in description"
    $ethernetName = Get-NetAdapter | Where-Object { $_.InterfaceDescription -like $MellanoxSearchString } | Select-Object -ExpandProperty Name

    if ($ethernetName)
    {
        Write-Host "Network adapter found: '$ethernetName'"
        $ethernetNameIfInProperty = Get-NetAdapterAdvancedProperty | Where-Object { $_.RegistryKeyword -like $PriorityVLANTagIdentifier -and $_.Name -eq $ethernetName } | Select-Object -ExpandProperty Name

        Write-Host "Searching network adapter properties for '$PriorityVLANTagIdentifier'"
        if ($ethernetNameIfInProperty)
        {
            Write-Host "Found '$PriorityVLANTagIdentifier' in adapter's advanced properties"
            Set-NetAdapterAdvancedProperty -Name $ethernetName -RegistryKeyword $PriorityVLANTagIdentifier -RegistryValue 3
            Write-Host "Successfully set Mellanox Network Adapter: '$ethernetName' with '$PriorityVLANTagIdentifier' property value as 3"
            return;
        }

        Write-Host "Could not find '$PriorityVLANTagIdentifier' in adapter's advanced properties"

        Write-Host "Proceeding in a different way"
        Write-Host "Searching through CIM instances for Network devices with '$MellanoxSearchString' in the name"
        $deviceID = Get-CimInstance -Namespace root/cimv2 -ClassName Win32_PNPEntity | Where-Object PNPClass -EQ "Net" | Where-Object { $_.Name -like $MellanoxSearchString } | Select-Object -ExpandProperty DeviceID
        if ($deviceID)
        {
            Write-Host "Device ID found: '$deviceID'"
            Write-Host "Getting Pnp Device properites for '$deviceID'"
            $registryKeySuffix = Get-PnpDeviceProperty -InstanceId $deviceID | Where-Object KeyName -EQ "DEVPKEY_Device_Driver" | Select-Object -ExpandProperty Data
            Write-Host "Registry key suffix found: '$registryKeySuffix'"
            $registryKeyFullPath = $RegistryKeyPrefix + $registryKeySuffix
            Write-Host "Registry key full path: '$registryKeyFullPath'"
            Write-Host "Updating '$PriorityVLANTagIdentifier' to be 3"
            New-ItemProperty -Path $registryKeyFullPath -Name $PriorityVLANTagIdentifier -Value 3 -PropertyType String -Force
            Write-Host "Updated successfully"

            Write-Host "Restarting Mellanox network adapter for regkey change to take effect"
            Restart-NetAdapter -Name $ethernetName
            Write-Host "Successfully restarted Mellanox network adapter"
            Write-Host "For Mellanox CX-3 adapters, if the problem persists please restart the VM"
            return;
        }

        Write-Host "No network device found with '$MellanoxSearchString' in name, exiting."
        return;
    }

    Write-Host "No Network adapter found with '$MellanoxSearchString' in description"
    return;
}

Set-NetAdapterPriorityVLanTag
