Write-Host $env:CONTAINER_SANDBOX_MOUNT_POINT 

$sourceCNI = $env:CONTAINER_SANDBOX_MOUNT_POINT + "\azure-container-networking\cni\network\plugin\azure-vnet.exe"
$sourceConflist = $env:CONTAINER_SANDBOX_MOUNT_POINT + "\azure-container-networking\cni\azure-windows-multitenancy.conflist"
$sourceCNIVersion = & "$sourceCNI" -v
$currentVersion = ""

$cniExists = Test-Path "C:\k\azurecni\bin\azure-vnet.exe"

Write-Host "Source  $sourceCNIVersion"

if ($cniExists) {
    $currentVersion = & "C:\k\azurecni\bin\azure-vnet.exe" -v
}

Write-Host "Current Host $currentVersion"

## check CNI was already installed so not to get stuck in a infinite loop of rebooting
if ($currentVersion -ne $sourceCNIVersion){
    Write-Host "about copying azure-vnet to windows node..."
    Rename-Item -Path "C:\k\azurecni\bin\azure-vnet.exe" -NewName "azure-vnet-old.exe"
    Copy-Item $sourceCNI -Destination "C:\k\azurecni\bin"
    Write-Host "about copying 10-azure.conflist file to windows node..."
    Remove-Item "C:\k\azurecni\netconf\10-azure.conflist"
    Copy-Item $sourceConflist -Destination "C:\k\azurecni\netconf"
    Rename-Item -Path "C:\k\azurecni\netconf\azure-windows-multitenancy.conflist" -NewName "10-azure.conflist"

    shutdown /r /t 0
}