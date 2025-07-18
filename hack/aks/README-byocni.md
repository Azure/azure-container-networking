# BYO CNI Cluster Setup Script

The `create-byocni-cluster.sh` script automates the creation of a BYO (Bring Your Own) CNI cluster on Azure Kubernetes Service (AKS). It orchestrates the following steps:

1. **Cluster Creation**: Creates an AKS cluster with configurable networking modes (overlay, swift, nodesubnet, dualstack-overlay, vnetscale-swift)
2. **CNS Deployment**: Deploys Azure Container Networking Service (CNS) to the cluster using the `test-load` make command  
3. **CNI Installation**: Installs the specified CNI networking components (Cilium by default, Azure CNI Manager, or none)

## Prerequisites

Before running the script, ensure you have:

- Azure CLI installed and logged in (`az login`)
- kubectl installed and configured
- make utility installed
- gettext package installed (for `envsubst` command)
- Proper Azure subscription permissions to create AKS clusters

## Usage

### Basic Usage

Create a cluster with default settings (overlay networking with Cilium):

**Note**: The script must be run from the root directory of the azure-container-networking repository.

```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh --subscription YOUR_SUBSCRIPTION_ID
```

### Advanced Usage

Create a cluster with custom configuration:
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --cluster my-cluster \
    --subscription YOUR_SUBSCRIPTION_ID \
    --networking-mode swift \
    --vm-size Standard_D2s_v3 \
    --cni-plugin cilium \
    --cns-version v1.6.0 \
    --cilium-dir 1.16 \
    --cilium-version-tag v1.16.5
```

### Dry Run

Preview the commands that would be executed without actually running them:
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh --subscription YOUR_SUBSCRIPTION_ID --dry-run
```

## Configuration Options

All parameters have sensible defaults as specified below. Only the `--subscription` parameter is required.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `--cluster` | Name of the AKS cluster | `byocni-cluster` |
| `--subscription` | Azure subscription ID | *Required* |
| `--resource-group` | Resource group name | Same as cluster name |
| `--azcli` | Azure CLI command | `az` |
| `--kubernetes-version` | Kubernetes version for the cluster | `1.33` |
| `--vm-size` | Azure VM size for cluster nodes | `Standard_B2s` |
| `--networking-mode` | Networking mode (overlay, swift, nodesubnet, dualstack-overlay, vnetscale-swift) | `overlay` |
| `--no-kube-proxy` | Create cluster without kube-proxy | `true` |
| `--with-kube-proxy` | Create cluster with kube-proxy | Overrides --no-kube-proxy |
| `--cni-plugin` | CNI plugin (cilium, azure-cni, none) | `cilium` |
| `--cns-version` | CNS version to deploy | `v1.5.38` |
| `--azure-ipam-version` | Azure IPAM version | `v0.3.0` |
| `--cilium-dir` | Cilium version directory | `1.17` |
| `--cilium-registry` | Cilium image registry | `acnpublic.azurecr.io` |
| `--cilium-version-tag` | Cilium version tag | Auto-detected |
| `--ipv6-hp-bpf-version` | IPv6 HP BPF version | Auto-detected |
| `--cns-image-repo` | CNS image repository | `MCR` |
| `--dry-run` | Show commands without executing | `false` |

## Networking Modes

- **overlay**: Standard overlay networking mode (supports kube-proxy and no-kube-proxy)
- **swift**: SWIFT networking mode (supports kube-proxy and no-kube-proxy)
- **nodesubnet**: NodeSubnet networking mode (only supports no-kube-proxy)
- **dualstack-overlay**: Dualstack overlay networking mode (supports kube-proxy and no-kube-proxy)
- **vnetscale-swift**: VNet Scale SWIFT networking mode (supports kube-proxy and no-kube-proxy)

## CNI Plugins

- **cilium**: Deploy Cilium CNI with configurable versions (default)
- **azure-cni**: Deploy Azure CNI Manager
- **none**: Deploy only cluster and CNS, no CNI plugin

## Supported Cilium Versions

The script supports the following Cilium versions based on available manifests (when using --cni-plugin cilium):
- v1.12
- v1.13
- v1.14 
- v1.16
- v1.17 (default)

## Examples

### Example 1: Basic cluster creation with Cilium (default)
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f
```

### Example 2: Swift networking with Azure CNI Manager
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --networking-mode swift \
    --cni-plugin azure-cni
```

### Example 3: Custom cluster with specific CNS version and Cilium
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --cluster production-cluster \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --networking-mode overlay \
    --cns-version v1.6.0 \
    --azure-ipam-version v0.4.0 \
    --cni-plugin cilium \
    --cilium-dir 1.16
```

### Example 4: Custom cluster and resource group
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --cluster my-aks-cluster \
    --resource-group my-resource-group \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f
```

### Example 5: Cluster with kube-proxy enabled
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --networking-mode overlay \
    --with-kube-proxy
```

### Example 6: Dualstack cluster with Cilium
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --networking-mode dualstack-overlay \
    --cilium-dir 1.17 \
    --cilium-version-tag v1.17.0
```

### Example 7: Cluster with specific Kubernetes version
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --kubernetes-version 1.30 \
    --networking-mode overlay \
    --cni-plugin cilium
```

### Example 8: Only cluster and CNS, no CNI plugin
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --cni-plugin none
```

### Example 9: Using different image registry
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --cilium-registry mcr.microsoft.com/containernetworking \
    --cilium-version-tag v1.14.8
```

## Post-Installation

After successful cluster creation, you can:

1. Get the kubeconfig:
   ```bash
   az aks get-credentials --resource-group CLUSTER_NAME --name CLUSTER_NAME
   ```

2. Verify the installation:
   ```bash
   kubectl get pods -n kube-system
   kubectl get nodes
   ```

3. Check Cilium status:
   ```bash
   kubectl get pods -n kube-system | grep cilium
   ```

## Troubleshooting

### Common Issues

1. **Azure CLI not logged in**: Run `az login` before executing the script
2. **Missing permissions**: Ensure your Azure account has permissions to create AKS clusters
3. **Invalid Cilium version**: Use `--dry-run` to test configuration before running
4. **kubectl not found**: Install kubectl and ensure it's in your PATH

### Debugging

Use the `--dry-run` flag to see exactly what commands would be executed:
```bash
cd /path/to/azure-container-networking
./hack/aks/create-byocni-cluster.sh --subscription YOUR_SUBSCRIPTION_ID --dry-run
```

Check the logs for detailed information about each step of the process.

## Contributing

When modifying the script, please:
1. Test with `--dry-run` first
2. Ensure all error handling works correctly
3. Update this documentation for any new features
4. Test with different Cilium versions to ensure compatibility