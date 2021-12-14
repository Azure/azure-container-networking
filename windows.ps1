# example usage:
# powershell.exe -command "& { . .\windows.ps1; azure-npm-image <imagetag> }"
# Retry({azure-npm-image $(tag)-windows-amd64})

function Retry([Action]$action) {
    $attempts = 3    
    $sleepInSeconds = 5
    do {
        try {
            $action.Invoke();
            break;
        }
        catch [Exception] {
            Write-Host $_.Exception.Message
        }            
        $attempts--
        if ($attempts -gt 0) { 
            sleep $sleepInSeconds 
        }
    } while ($attempts -gt 0)    
}

function azure-npm-image {
    $env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
    $env:NPM_AI_ID = "014c22bd-4107-459e-8475-67909e96edcb"
    $env:NPM_AI_PATH = "$env:ACN_PACKAGE_PATH/npm.aiMetadata"

    if ($null -eq $env:VERSION) { $env:VERSION = $args[0] } 
    docker build `
        -f npm/Dockerfile.windows `
        -t acnpublic.azurecr.io/azure-npm:$env:VERSION `
        --build-arg VERSION=$env:VERSION `
        --build-arg NPM_AI_PATH=$env:NPM_AI_PATH `
        --build-arg NPM_AI_ID=$env:NPM_AI_ID `
        .
}
