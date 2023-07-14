Write-Host $env:CONTAINER_SANDBOX_MOUNT_POINT
$sourceCNI = $env:CONTAINER_SANDBOX_MOUNT_POINT + "azure-vnet.exe"
$sourceIpam = $env:CONTAINER_SANDBOX_MOUNT_POINT + "azure-vnet-ipam.exe"
$sourceTelemetry = $env:CONTAINER_SANDBOX_MOUNT_POINT + "azure-vnet-telemetry.exe"
$sourceTelemetryConfig = $env:CONTAINER_SANDBOX_MOUNT_POINT + "azure-vnet-telemetry.config"

$sourceCNIVersion = & "$sourceCNI" -v
$currentVersion = ""
$sourceTelemetryVersion = & "$sourceTelemetry" -v
$currentTelemetryVersion = ""

$cniExists = Test-Path "C:\k\azurecni\bin\azure-vnet.exe"
$telemetryExists = Test-Path "C:\k\azurecni\bin\azure-vnet-telemetry.exe"

Write-Host "Source  $sourceCNIVersion"
Write-Host "Source Telemetry $sourceTelemetryVersion"

if ($cniExists) {
    $currentVersion = & "C:\k\azurecni\bin\azure-vnet.exe" -v
}

if($telemetryExists){
    $currentTelemetryVersion = & "C:\k\azurecni\bin\azure-vnet-telemetry.exe" -v
}


Write-Host "Current Host $currentVersion"
Write-Host "Current Telemetry $currentTelemetryVersion"

## check telemetry was already installed so not to get stuck in a infinite loop of rebooting and killing the process
if ($currentTelemetryVersion -ne $sourceTelemetryVersion){
    $processes = Get-Process -Name azure-vnet-telemetry -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt $processes.Count; $i++) {
        Write-Host "Killing azure-vnet-telemetry process..."
        $processes[$i].Kill()
    }
    Write-Host "copying azure-vnet-telemetry to windows node..."
    Remove-Item "C:\k\azurecni\bin\azure-vnet-telemetry.exe"
    Copy-Item $sourceTelemetry -Destination "C:\k\azurecni\bin"

    Write-Host "copying azure-vnet-telemetry.config to windows node..."
    Remove-Item "C:\k\azurecni\bin\azure-vnet-telemetry.config"
    Copy-Item $sourceTelemetryConfig -Destination "C:\k\azurecni\bin"
}

## check CNI was already installed so not to get stuck in a infinite loop of rebooting
if ($currentVersion -ne $sourceCNIVersion){
    Write-Host "copying azure-vnet to windows node..."
    Remove-Item "C:\k\azurecni\bin\azure-vnet.exe"
    Copy-Item $sourceCNI -Destination "C:\k\azurecni\bin"

    Write-Host "copying azure-vnet-ipam to windows node..."
    Remove-Item "C:\k\azurecni\bin\azure-vnet-ipam.exe"
    Copy-Item $sourceIpam -Destination "C:\k\azurecni\bin"
}

Start-Sleep -s 1000
