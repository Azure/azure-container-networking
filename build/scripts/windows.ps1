# example usage:
# powershell.exe -command "& { . .\windows.ps1; npm-image <imagetag> }"
# Retry({npm-image $(tag)-windows-amd64})

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

function npm-image {
    $env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
    $env:NPM_AI_ID = "014c22bd-4107-459e-8475-67909e96edcb"
    $env:NPM_AI_PATH = "$env:ACN_PACKAGE_PATH/npm.aiMetadata"

    if ($null -eq $env:VERSION) { $env:VERSION = $args[0] } 
    docker build `
        -f npm/windows.Dockerfile `
        -t acnpublic.azurecr.io/azure-npm:$env:VERSION `
        --build-arg VERSION=$env:VERSION `
        --build-arg NPM_AI_PATH=$env:NPM_AI_PATH `
        --build-arg NPM_AI_ID=$env:NPM_AI_ID `
        .
}

function cns-image {
    $env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
    $env:CNS_AI_ID = "ce672799-8f08-4235-8c12-08563dc2acef"
    $env:CNS_AI_PATH = "$env:ACN_PACKAGE_PATH/cns/logger.aiMetadata"

    if ($null -eq $env:VERSION) { $env:VERSION = $args[0] } 
    docker build `
        -f cns/windows.Dockerfile `
        -t acnpublic.azurecr.io/azure-cns:$env:VERSION `
        --build-arg VERSION=$env:VERSION `
        --build-arg CNS_AI_PATH=$env:CNS_AI_PATH `
        --build-arg CNS_AI_ID=$env:CNS_AI_ID `
        .
}
