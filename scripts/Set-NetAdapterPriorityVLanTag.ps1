function Set-NetAdapterPriorityVLanTag
{
    Write-Host "Searching for a network adapter with 'Mellanox' in description"
    $ethernetName = Get-NetAdapter | Where-Object { $_.InterfaceDescription -like "*Mellanox*" } | Select-Object -ExpandProperty Name

    if ($ethernetName)
    {
        Write-Host "Network adapter found: '$ethernetName'"
        $ethernetNameIfInProperty = Get-NetAdapterAdvancedProperty | Where-Object { $_.RegistryKeyword -like "*PriorityVLANTag" -and $_.Name -eq $ethernetName } | Select-Object -ExpandProperty Name

        Write-Host "Searching network adapter properties for '*PriorityVLANTag'"
        if ($ethernetNameIfInProperty)
        {
            Write-Host "Found 'PriorityVLANTag' in adapter's advanced properties"
            Set-NetAdapterAdvancedProperty -Name $ethernetName -RegistryKeyword "*PriorityVLANTag" -RegistryValue 3
            Write-Host "Successfully set Mellanox Network Adapter: '$ethernetName' with '*PriorityVLANTag' property value as 3"
            return;
        }

        Write-Host "Could not find 'PriorityVLANTag' in adapter's advanced properties"
        Write-Host "Proceeding to set in a different way"

    }
}