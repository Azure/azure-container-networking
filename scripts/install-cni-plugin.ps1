<#
    .SYNOPSIS
        Installs azure-vnet CNI plugins on a Windows node.

    .DESCRIPTION
        Installs azure-vnet CNI plugins on a Windows node.
#>
[CmdletBinding(DefaultParameterSetName="Standard")]
param(
    [string]
    [parameter(Mandatory=$true)]
    [ValidateNotNullOrEmpty()]
    $PluginVersion = "v0.7",

    [string]
    [parameter(Mandatory=$false)]
    [ValidateNotNullOrEmpty()]
    $CniBinDir = "c:\cni\bin",

    [string]
    [parameter(Mandatory=$false)]
    [ValidateNotNullOrEmpty()]
    $CniNetConfDir = "c:\cni\netconf"
)

function
Expand-File($file, $destination)
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
    Write-Host "Creating CNI directories."
    New-Item $CniBinDir -Type directory -Force > $null
    New-Item $CniNetConfDir -Type directory -Force > $null

    # Install azure-vnet CNI plugins.
    Write-Host "Installing azure-vnet CNI plugin version $PluginVersion to $CniBinDir..."
    Invoke-WebRequest -Uri https://github.com/Azure/azure-container-networking/releases/download/$PluginVersion/azure-vnet-cni-windows-amd64-$PluginVersion.zip -OutFile $CniBinDir\azure-vnet.zip
    Expand-File $CniBinDir\azure-vnet.zip $CniBinDir

    # Install azure-vnet CNI network configuration file.
    Write-Host "Installing azure-vnet CNI network configuration file to $CniNetConfDir..."
    Move-Item $CniBinDir\*.conf $CniNetConfDir -Force

    # Windows does not need a loopback plugin.

    # Cleanup.
    Remove-Item $CniBinDir\azure-vnet.zip

    Write-Host "azure-vnet CNI plugin is successfully installed."
}
catch
{
    Write-Error $_
}
