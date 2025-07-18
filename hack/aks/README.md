Use this Makefile to swiftly provision/deprovision AKS clusters of different Networking flavors in Azure.

---
```bash
➜  make help
Usage:
  make <target>

Help
  help             Display this help

Utilities
  set-kubeconf     Adds the kubeconf for $CLUSTER
  unset-kubeconf   Deletes the kubeconf for $CLUSTER
  shell            print $AZCLI so it can be used outside of make

SWIFT Infra
  vars             Show the env vars configured for the swift command
  rg-up            Create resource group $GROUP in $SUB/$REGION
  rg-down          Delete the $GROUP in $SUB/$REGION
  net-up           Create required swift vnet/subnets

AKS Clusters
  byocni-up                                 Alias to swift-byocni-up
  cilium-up                                 Alias to swift-cilium-up
  up                                        Alias to swift-up
  nodesubnet-byocni-nokubeproxy-up          Bring up a Nodesubnet BYO CNI cluster. Does not include secondary IP configs.
  overlay-byocni-up                         Bring up a Overlay BYO CNI cluster
  overlay-byocni-nokubeproxy-up             Bring up a Overlay BYO CNI cluster without kube-proxy
  overlay-cilium-up                         Bring up a Overlay Cilium cluster
  overlay-up                                Bring up a Overlay AzCNI cluster
  swift-byocni-up                           Bring up a SWIFT BYO CNI cluster
  swift-byocni-nokubeproxy-up               Bring up a SWIFT BYO CNI cluster without kube-proxy
  swift-cilium-up                           Bring up a SWIFT Cilium cluster
  swift-up                                  Bring up a SWIFT AzCNI cluster
  vnetscale-swift-byocni-up                 Bring up a Vnet Scale SWIFT BYO CNI cluster
  vnetscale-swift-byocni-nokubeproxy-up     Bring up a Vnet Scale SWIFT BYO CNI cluster without kube-proxy
  vnetscale-swift-cilium-up                 Bring up a Vnet Scale SWIFT Cilium cluster
  vnetscale-swift-up                        Bring up a Vnet Scale SWIFT AzCNI cluster
  nodesubnet-cilium-up                      Bring up a Nodesubnet Cilium cluster
  cniv1-up                                  Bring up a AzCNIv1 cluster
  dualstack-overlay-byocni-up               Bring up an dualstack overlay cluster without CNS and CNI installed
  cilium-dualstack-up                       Brings up a Cilium Dualstack Overlay cluster with Linux node only
  dualstack-byocni-nokubeproxy-up           Brings up a Dualstack overlay BYOCNI cluster with Linux node only and no kube-proxy
  windows-nodepool-up                       Add windows node pool
  down                                      Delete the cluster
  vmss-restart                              Restart the nodes of the cluster

BYO CNI Automation
  byocni-cluster-up                         Create complete BYO CNI cluster with CNS and CNI (default: Cilium)
  deploy-cns                               Deploy CNS to the cluster  
  deploy-cilium                            Deploy Cilium to the cluster
  byocni-cluster-vars                      Show variables for BYO CNI cluster setup
  validate-cni-type                        Validate the CNI type
```

## BYO CNI Cluster Automation

The Makefile now includes automated setup for complete BYO CNI clusters with CNS and CNI deployment.

### Quick Start

Create a BYO CNI cluster with Cilium (default):
```bash
make byocni-cluster-up CLUSTER=my-cluster SUB=<subscription-id>
```

### Customization

All parameters are configurable:
```bash
make byocni-cluster-up \
    CLUSTER=my-cilium-cluster \
    SUB=<subscription-id> \
    CNS_VERSION=v1.6.0 \
    CILIUM_DIR=1.16 \
    CILIUM_VERSION_TAG=v1.16.5 \
    CILIUM_IMAGE_REGISTRY=mcr.microsoft.com/containernetworking
```

### Available Configuration

- `CNI_TYPE`: cilium (default) - Future CNI types can be added
- `CNS_VERSION`: CNS version to deploy (default: v1.5.38)
- `AZURE_IPAM_VERSION`: Azure IPAM version (default: v0.3.0)
- `CNS_IMAGE_REPO`: CNS image repository - MCR or ACR (default: MCR)
- `CILIUM_DIR`: Cilium version directory - 1.12, 1.13, 1.14, 1.16, 1.17 (default: 1.14)
- `CILIUM_VERSION_TAG`: Cilium image tag (default: v1.14.8)
- `CILIUM_IMAGE_REGISTRY`: Cilium image registry (default: acnpublic.azurecr.io)
- `IPV6_HP_BPF_VERSION`: IPv6 HP BPF version for dual stack (default: v0.0.3)

View all configuration variables:
```bash
make byocni-cluster-vars
```

### Workflow

The `byocni-cluster-up` target orchestrates three main steps:

1. **Cluster Creation**: Uses `overlay-byocni-nokubeproxy-up` to create AKS cluster
2. **CNS Deployment**: Uses root makefile `test-load` target with CNS-specific parameters  
3. **CNI Deployment**: Deploys Cilium using manifests from `test/integration/manifests/cilium/`

Individual steps can also be run separately:
```bash
make deploy-cns
make deploy-cilium
```
