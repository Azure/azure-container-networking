 # ./cleanupnetwork.ps1 -CniDirectory c:\k -NetworkName azure
 param (
    [string]$CniDirectory = "c:\k",
    [Parameter(Mandatory=$true)][string]$NetworkName
 )

Invoke-WebRequest -Uri https://raw.githubusercontent.com/microsoft/SDN/master/Kubernetes/windows/hns.psm1 -OutFile "c:\hns.psm1" -UseBasicParsing

$global:NetworkMode = "L2Bridge"
$global:HNSModule = "c:\hns.psm1"

ipmo $global:HNSModule

$networkname = $global:NetworkMode.ToLower()
if ($global:NetworkPlugin -eq "azure") {
    $networkname = "azure"
}

$hnsNetwork = Get-HnsNetwork | ? Name -EQ $networkname
if ($hnsNetwork) {   
    Write-Host "Cleaning up persisted HNS policy lists"
    # Initially a workaround for https://github.com/kubernetes/kubernetes/pull/68923 in < 1.14,
    # and https://github.com/kubernetes/kubernetes/pull/78612 for <= 1.15
    #
    # October patch 10.0.17763.1554 introduced a breaking change 
    # which requires the hns policy list to be removed before network if it gets into a bad state
    # See https://github.com/Azure/aks-engine/pull/3956#issuecomment-720797433 for more info
    # Kubeproxy doesn't fail becuase errors are not handled: 
    # https://github.com/delulu/kubernetes/blob/524de768bb64b7adff76792ca3bf0f0ece1e849f/pkg/proxy/winkernel/proxier.go#L532
    Get-HnsPolicyList | Remove-HnsPolicyList

    Write-Host "Cleaning up old HNS network found"
    Remove-HnsNetwork $hnsNetwork
    Start-Sleep 10
} else {
    Write-Host "no hns network found with name" $networkname
}


if ($global:NetworkPlugin -eq "azure") {
    Write-Host "NetworkPlugin azure, starting kubelet."

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
} else {
    Write-Host "network plugin name not recognized, default is \azure" $networkname
}
