 # ./cleanupnetwork.ps1 -CniDirectory c:\k -NetworkName azure
 param (
    [string]$CniDirectory = "c:\windows\system32",
 )

Invoke-WebRequest -Uri https://raw.githubusercontent.com/microsoft/SDN/master/Kubernetes/windows/hns.psm1 -OutFile "c:\hns.psm1" -UseBasicParsing

$global:HNSModule = "c:\hns.psm1"
ipmo $global:HNSModule

foreach($net in Get-HnsNetwork) { 
    Get-HnsPolicyList | Remove-HnsPolicyList
    if ($net.Name.StartsWith("azure")) { 
        Write-Host "Cleaning up old HNS network:" $net.Name
        Remove-HnsNetwork $net
        Start-Sleep 10
    }
}

Write-Host "Cleaning stale CNI data"
# Kill all cni instances & stale data left by cni
# Cleanup all files related to cni
taskkill /IM azure-vnet.exe /f
taskkill /IM azure-vnet-ipam.exe /f

# azure-cni logs currently end up in c:\windows\system32 when machines are configured with containerd.
# https://github.com/containerd/containerd/issues/4928
$filesToRemove = @(
    $CniDirectory+"\azure-vnet.json",
    $CniDirectory+"\azure-vnet.json.lock",
    $CniDirectory+"\azure-vnet-ipam.json",
    $CniDirectory+"\azure-vnet-ipam.json.lock"
    $CniDirectory+"\azure-vnet-ipamv6.json",
    $CniDirectory+"\azure-vnet-ipamv6.json.lock"
)

foreach ($file in $filesToRemove) {
    if (Test-Path $file) {
        Write-Host "Deleting stale file at $file"
        Remove-Item $file
    }
}
