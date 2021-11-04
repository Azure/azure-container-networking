function azure-npm-image {
    docker build -f npm/Dockerfile.windows -t acnpublic.azurecr.io/azure-npm:$env:tag-windows-amd64 .
}
