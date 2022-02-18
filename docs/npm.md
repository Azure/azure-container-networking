# Microsoft Azure Container Networking

## Azure Network Policy Manager

`azure-npm` Network Policy plugin implements the [Kubernetes Network Policy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

The plugin is available on Linux platform. Windows support is planned.

Azure-NPM serves as a distributed firewall for the Kubernetes cluster, and it can be easily controlled by `kubectl`.

## Install

Running the command below will bring up one azure-npm instance on each Kubernetes node.
```
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/master/npm/deploy/npm/azure-npm.yaml
```
Now you can secure your Kubernetes cluster with Azure-NPM by applying Kubernetes network policies.

## Build

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

## Usage

Microsoft docs has a detailed step by step example on how to use Kubernetes network policy.
1. [Deny all inbound traffic to a pod](https://docs.microsoft.com/en-us/azure/aks/use-network-policies#deny-all-inbound-traffic-to-a-pod)
2. [Allow inbound traffic based on a pod label](https://docs.microsoft.com/en-us/azure/aks/use-network-policies#allow-inbound-traffic-based-on-a-pod-label)
3. [Allow traffic only from within a defined namespace](https://docs.microsoft.com/en-us/azure/aks/use-network-policies#allow-traffic-only-from-within-a-defined-namespace)

## Troubleshooting

`azure-npm` translates Kubernetes network policies into a set of `iptables` rules under the hood.
When `azure-npm` isn't working as expected, try to **delete all networkpolicies and apply them again**.
Also, a good practice is to merge all network policies targeting the same set of pods/labels into one yaml file.
This way, operators can keep the minimum number of network policies and makes it easier for operators to troubleshoot.
