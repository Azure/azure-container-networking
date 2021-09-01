Use this Makefile to swiftly provision/deprovision [enhanced pod subnet (aka swift)](https://docs.microsoft.com/en-us/azure/aks/configure-azure-cni#dynamic-allocation-of-ips-and-enhanced-subnet-support-preview) clusters in Azure.

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

SWIFT
  swift-vars       Show the env vars configured for the swift command
  swift-net-up     Create required swift vnet/subnets
  swift-rg-down    Delete the $GROUP in $SUB/$REGION
  swift-up         Brings up a swift cluster $name in $SUB/$REGION
  swift-down       Deletes the swift resources $SUB/$REGION
```
