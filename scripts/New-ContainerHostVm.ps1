<#
    .SYNOPSIS
        Creates an Azure VM with given number of network interfaces and IP addresses.

    .DESCRIPTION
        Useful for testing container networking on Azure.
        Expects a pre-created VNET with the given name and subnets with names "subnetN".
#>
Param
(
    [Parameter(Mandatory=$true)]  [string] $ResourceGroupName,
    [Parameter(Mandatory=$true)]  [string] $StorageAccountName,
    [Parameter(Mandatory=$true)]  [string] $VmName,
    [Parameter(Mandatory=$true)]  [string] $VmOs,
    [Parameter(Mandatory=$false)] [string] $VmSize = "Standard_DS1_v2",
    [Parameter(Mandatory=$false)] [string] $Location = "West US",
    [Parameter(Mandatory=$false)] [string] $VnetName = "vnet1",
    [Parameter(Mandatory=$false)] [int] $InterfaceCount = 2,
    [Parameter(Mandatory=$false)] [int] $AddressCount = 10,

    # Windows OS image defaults.
    [Parameter(Mandatory=$false)] [string] $WindowsImagePublisher = "MicrosoftWindowsServer",
    [Parameter(Mandatory=$false)] [string] $WindowsImageOffer = "WindowsServer",
    [Parameter(Mandatory=$false)] [string] $WindowsImageSku = "2016-Datacenter-with-Containers",
    [Parameter(Mandatory=$false)] [string] $WindowsImageVersion = "latest",

    # Linux OS image defaults.
    [Parameter(Mandatory=$false)] [string] $LinuxImagePublisher = "Canonical",
    [Parameter(Mandatory=$false)] [string] $LinuxImageOffer = "UbuntuServer",
    [Parameter(Mandatory=$false)] [string] $LinuxImageSku = "16.04.0-LTS",
    [Parameter(Mandatory=$false)] [string] $LinuxImageVersion = "latest"
)

try {
    Write-Host "Creating VM $VmName in $Location..."
    $cred = Get-Credential -Message "Enter credentials for the local administrator account."

    # Configure VM size, OS and image.
    $vmConfig = New-AzureRmVMConfig -VMName $VmName -VMSize $VmSize
    if ($VmOs -eq "windows") {
        Set-AzureRmVMOperatingSystem -VM $vmConfig -ComputerName $VmName -Windows -Credential $cred
        $imagePublisher = $WindowsImagePublisher
        $imageOffer = $WindowsImageOffer
        $imageSku = $WindowsImageSku
        $imageVersion = $WindowsImageVersion
    } else {
        Set-AzureRmVMOperatingSystem -VM $vmConfig -ComputerName $VmName -Linux -Credential $cred
        $imagePublisher = $LinuxImagePublisher
        $imageOffer = $LinuxImageOffer
        $imageSku = $LinuxImageSku
        $imageVersion = $LinuxImageVersion
    }

    Set-AzureRmVMSourceImage `
        -VM $vmConfig `
        -PublisherName $imagePublisher `
        -Offer $imageOffer `
        -Skus $imageSku `
        -Version $imageVersion

    # Configure storage.
    Set-AzureRmCurrentStorageAccount -ResourceGroupName $ResourceGroupName -StorageAccountName $StorageAccountName
    $storageAccount = Get-AzureRmStorageAccount -ResourceGroupName $ResourceGroupName -Name $StorageAccountName

    Write-Host "Adding OS disk..."
    $osVhdPath = $storageAccount.PrimaryEndpoints.Blob.ToString() + "vhds/"
    $osDiskName = "$VmName-Disk0"
    $osVhdUri = "$osVhdPath$osDiskName.vhd"
    Set-AzureRmVMOSDisk `
        -VM $vmConfig `
        -Name $osDiskName `
        -VhdUri $osVhdUri `
        -CreateOption fromImage 

    Write-Host "Adding data disk..."
    $dataDiskName = "$VmName-Disk1"
    $data1VhdUri = "$osVhdPath$dataDiskName.vhd"
    Add-AzureRmVMDataDisk `
        -VM $vmConfig `
        -Name $dataDiskName `
        -DiskSizeInGB 100 `
        -VhdUri $data1VhdUri `
        -CreateOption empty `
        -Lun 0

    # Configure networking.
    $vnet = Get-AzureRmVirtualNetwork -Name $VnetName -ResourceGroupName $ResourceGroupName

    for ($ifIndex = 1; $ifIndex -le $InterfaceCount; $ifIndex++) {
        # Add network interfaces.
        $ifName = "$VmName-if$ifIndex"
        $ifPrimary = ($ifIndex -eq 1)
        $subnetName = "subnet$ifIndex"
        Write-Host "Creating network interface $ifName on subnet $subnetName..."

        $subnet = Get-AzureRmVirtualNetworkSubnetConfig -Name $subnetName -VirtualNetwork $vnet

        if ($ifPrimary) {
            $pip = New-AzureRmPublicIpAddress `
                -Name "$ifName-pip" `
                -ResourceGroupName $ResourceGroupName `
                -Location $Location `
                -AllocationMethod Dynamic `
                -DomainNameLabel $VmName
        } else {
            $pip = $null
        }

        $if = New-AzureRmNetworkInterface `
            -Name $ifName `
            -ResourceGroupName $ResourceGroupName `
            -Location $Location `
            -Subnet $subnet `
            -PublicIpAddress $pip

        # Add secondary IP addresses.
        for ($addrIndex = 2; $addrIndex -le $AddressCount; $addrIndex++) {
            Add-AzureRmNetworkInterfaceIpConfig -Name "ipconfig$addrIndex" -NetworkInterface $if -Subnet $subnet
        }
        $if | Set-AzureRmNetworkInterface

        $vmConfig = Add-AzureRmVMNetworkInterface -VM $vmConfig -Id $if.Id -Primary:$ifPrimary
    }

    # Create the VM.
    Write-Host "Creating the VM..."
    New-AzureRmVM -VM $vmConfig -ResourceGroupName $ResourceGroupName -Location $Location

    Write-Host "Done."
}
catch
{
    Write-Error $_
}
