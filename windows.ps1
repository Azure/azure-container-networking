
function azure-npm-image {
    if ($null -eq $env:imagetag) { $env:imagetag = $args[0] } 
    docker build -f npm/Dockerfile.windows -t acnpublic.azurecr.io/azure-npm:$env:imagetag .
}
