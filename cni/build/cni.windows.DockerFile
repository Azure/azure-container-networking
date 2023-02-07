FROM mcr.microsoft.com/windows/servercore:ltsc2019 as Builder

# Run as admin
USER ContainerAdministrator

SHELL ["powershell", "-command"]

ARG VERSION

RUN Write-Host $($env:VERSION)

## Install Chocolatey
RUN iex ((new-object net.webclient).DownloadString('https://chocolatey.org/install.ps1'));
RUN choco feature disable --name showDownloadProgress

### Install golang
RUN powershell -command choco install golang --version=1.19 -y
WORKDIR /azure-container-networking
COPY . .

SHELL ["cmd", "/S", "/C"]
## build azure cni for windows
WORKDIR /azure-container-networking/cni/network/plugin/
RUN go build -a -o azure-vnet.exe -ldflags "-X main.version=%VERSION%"

WORKDIR /azure-container-networking
COPY /cni/scripts/installcni.ps1 .

ENTRYPOINT ["powershell.exe", ".\\installcni.ps1"]