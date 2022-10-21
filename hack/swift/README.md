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
  byocni-up        Alias to swift-byocni-up
  cilium-up        Alias to swift-cilium-up
  up               Alias to swift-up
  overlay-up       Brings up an Overlay AzCNI cluster
  swift-byocni-up  Bring up a SWIFT BYO CNI cluster
  swift-cilium-up  Bring up a SWIFT Cilium cluster
  swift-up         Bring up a SWIFT AzCNI cluster
  down             Delete the cluster
```
