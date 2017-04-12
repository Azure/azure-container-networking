<#
    .SYNOPSIS
        Installs azure-vnet CNI plugins on a Windows node.

    .DESCRIPTION
        Installs azure-vnet CNI plugins on a Windows node.
#>
[CmdletBinding(DefaultParameterSetName="Standard")]
param(
    [string]
    [ValidateNotNullOrEmpty()]
    $PluginVersion = "v0.7",

    [parameter(Mandatory=$false)]
    [ValidateNotNullOrEmpty()]
    $CniBinDir = "c:\cni\bin",

    [parameter(Mandatory=$false)]
    [ValidateNotNullOrEmpty()]
    $CniConfDir = "c:\cni\netconf"
)

function
Write-Log($message)
{
    Write-Host $message
}

function
Download-File($uri, $destination)
{
    $temp = "c:\k.zip"
    Invoke-WebRequest -Uri $uri -OutFile $temp
    Extract-ZipFile -File $temp -Destination $destination
}

function
Extract-ZipFile($file, $destination)
{
    $shell = new-object -com shell.application
    $zip = $shell.NameSpace($file)
    foreach($item in $zip.items())
    {
        $shell.Namespace($destination).copyhere($item)
    }
}

try {
    # Create CNI directories.
    mkdir $CniBinDir
    mkdir $CniConfDir

    # Install Azure CNI plugins.
    Download-File https://github.com/Azure/azure-container-networking/releases/download/$PluginVersion/azure-vnet-cni-windows-amd64-$PluginVersion.zip > $CniBinDir/azure.zip
    Extract-ZipFile $CniBinDir/azure.zip $CniBinDir

    # Windows does not need a loopback plugin.

    # Cleanup.
    del $CniBinDir/*.zip
    #chown root:root $CNI_BIN_DIR/*
}
catch
{
    Write-Error $_
}
