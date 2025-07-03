# BYO Cilium Cluster Setup Script

The `create-byocilium-cluster.sh` script automates the creation of a BYO (Bring Your Own) Cilium cluster on Azure Kubernetes Service (AKS). It orchestrates the following steps:

1. **Cluster Creation**: Creates an AKS cluster with overlay networking and no kube-proxy using the `overlay-byocni-nokubeproxy-up` make target
2. **CNS Deployment**: Deploys Azure Container Networking Service (CNS) to the cluster using the `test-load` make command
3. **Cilium Installation**: Installs Cilium networking components using manifests from the `test/integration/manifests/cilium/` directory

## Prerequisites

Before running the script, ensure you have:

- Azure CLI installed and logged in (`az login`)
- kubectl installed and configured
- make utility installed
- gettext package installed (for `envsubst` command)
- Proper Azure subscription permissions to create AKS clusters

## Usage

### Basic Usage

Create a cluster with default settings:
```bash
./create-byocilium-cluster.sh --subscription YOUR_SUBSCRIPTION_ID
```

### Advanced Usage

Create a cluster with custom configuration:
```bash
./create-byocilium-cluster.sh \
    --cluster my-cilium-cluster \
    --subscription YOUR_SUBSCRIPTION_ID \
    --cns-version v1.6.0 \
    --cilium-dir 1.16 \
    --cilium-version-tag v1.16.5
```

### Dry Run

Preview the commands that would be executed without actually running them:
```bash
./create-byocilium-cluster.sh --subscription YOUR_SUBSCRIPTION_ID --dry-run
```

## Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `--cluster` | Name of the AKS cluster | `byocni-cluster` |
| `--subscription` | Azure subscription ID | *Required* |
| `--azcli` | Azure CLI command | `az` |
| `--cns-version` | CNS version to deploy | `v1.5.38` |
| `--azure-ipam-version` | Azure IPAM version | `v0.3.0` |
| `--cilium-dir` | Cilium version directory | `1.14` |
| `--cilium-registry` | Cilium image registry | `acnpublic.azurecr.io` |
| `--cilium-version-tag` | Cilium version tag | Auto-detected |
| `--ipv6-hp-bpf-version` | IPv6 HP BPF version | Auto-detected |
| `--cns-image-repo` | CNS image repository | `MCR` |
| `--dry-run` | Show commands without executing | `false` |

## Supported Cilium Versions

The script supports the following Cilium versions based on available manifests:
- v1.12
- v1.13
- v1.14 (default)
- v1.16
- v1.17

## Examples

### Example 1: Basic cluster creation
```bash
./create-byocilium-cluster.sh --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f
```

### Example 2: Custom cluster with specific CNS version
```bash
./create-byocilium-cluster.sh \
    --cluster production-cilium \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --cns-version v1.6.0 \
    --azure-ipam-version v0.4.0
```

### Example 3: Latest Cilium version
```bash
./create-byocilium-cluster.sh \
    --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f \
    --cilium-dir 1.17 \
    --cilium-version-tag v1.17.0
```

### Example 4: Using different image registry
```bash
./create-byocilium-cluster.sh \
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
./create-byocilium-cluster.sh --subscription YOUR_SUBSCRIPTION_ID --dry-run
```

Check the logs for detailed information about each step of the process.

## Contributing

When modifying the script, please:
1. Test with `--dry-run` first
2. Ensure all error handling works correctly
3. Update this documentation for any new features
4. Test with different Cilium versions to ensure compatibility