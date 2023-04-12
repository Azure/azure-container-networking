# Microsoft Azure Container Networking

## Azure Network Policy Manager

`azure-npm` Network Policy plugin implements the [Kubernetes Network Policy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

The plugin is available on Linux and (preview) Windows Server 2022.

Azure-NPM serves as a distributed firewall for the Kubernetes cluster, and it can be easily controlled by `kubectl`.

## Documentation
1. [Secure traffic between pods using network policies in Azure Kubernetes Service (AKS)](https://learn.microsoft.com/en-us/azure/aks/use-network-policies)
2. [Monitor and Visualize Network Configurations with Azure NPM](https://learn.microsoft.com/en-us/azure/virtual-network/kubernetes-network-policies#monitor-and-visualize-network-configurations-with-azure-npm)

## Install
Specify `--network-policy=azure` when creating an AKS cluster. For more information, see the [Microsoft Docs](https://learn.microsoft.com/en-us/azure/aks/use-network-policies#create-an-aks-cluster-and-enable-network-policy).

### Manual Installation
Running the command below will bring up one azure-npm instance on each Kubernetes node.
```
# linux
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/azure-npm.yaml
# windows
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/examples/windows/azure-npm.yaml
```
Now you can secure your Kubernetes cluster with Azure-NPM by applying Kubernetes network policies.

## Build
### Linux
`azure-npm` can be built directly from the source code in this repository.
```
make azure-npm
make npm-image
make azure-npm-archive
```
The first command builds the `azure-npm` executable. 
The second command builds the `azure-npm` docker image.
The third command builds the `azure-npm` binary and place it in a tar archive. 
The binaries are placed in the `output` directory.

### Windows
```
$env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
$env:NPM_AI_PATH = "$env:ACN_PACKAGE_PATH/npm.aiMetadata"
$env:NPM_AI_ID = "1234abcd-1234-abcd-1234-12345678abcd"
$env:VERSION = "0.0.0"
$env:REPO = "mcr.microsoft.com/azure-npm:" # include colon at end
$env:IMAGE = "$env:REPO$env:VERSION"
docker build `
	-f npm/windows.Dockerfile `
	-t $env:IMAGE `
	--build-arg VERSION=$env:VERSION `
	--build-arg NPM_AI_PATH=$env:NPM_AI_PATH `
	--build-arg NPM_AI_ID=$env:NPM_AI_ID `
	.
docker push $env:IMAGE
echo $env:IMAGE
```

## Usage
[Microsoft Docs](https://learn.microsoft.com/en-us/azure/aks/use-network-policies#verify-network-policy-setup) has a detailed step by step example on how to use Kubernetes network policy.

## Troubleshooting
When `azure-npm` isn't working as expected, try to **delete all networkpolicies and apply them again**.
Also, a good practice is to merge all network policies targeting the same set of pods/labels into one yaml file.
This way, operators can keep the minimum number of network policies and makes it easier for operators to troubleshoot.

### Linux
NPM adds firewall rules via `iptables` and `ipset`. You can examine the configuration on a given node with:
- `kubectl exec -it -n kube-system $npmPod -- iptables -vnL`
- `kubectl exec -it -n kube-system $npmPod -- ipset -L`

### Windows
NPM adds firewall rules via HNS. You can examine the configuration on a given node with:
- ACLs applied on Pod Endpoints: `kubectl exec -n kube-system $npmWinPod -- Get-HNSEndpoint`
- SetPolicies are like ipsets: `(Get-HNSNetwork | ? Name -Like Azure).Policies`
