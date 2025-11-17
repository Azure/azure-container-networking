# Swiftv2 Managed Cilium Setup Guide

## Steps
### Clone repo + checkout branch for *.yamls
```
git clone https://github.com/Azure/azure-container-networking.git
git checkout master
```

### Update Conflist

```
export CONFLIST=azure-chained-cilium.conflist
export CONFLIST_PRIORITY=05
export CNI_IMAGE=acnpublic.azurecr.io/public/containernetworking/azure-cni:v1.7.5-3
envsubst '${CONFLIST},${CONFLIST_PRIORITY},${CNI_IMAGE}' < test/integration/manifests/cni/conflist-installer-byon.yaml | kubectl apply -f -
```


### Apply Watcher
```
kubectl apply -f test/integration/manifests/cilium/watcher/deployment.yaml
```

- Watcher obtains existing Cilium RBAC and Daemonset from managed node
  - We overwrite Cilium Configmap values through the use of args on the `cilium-agent` container within the watcher deployment.



### Quick Summary
- Apply conflist installer to update conflist on BYON
- Apply Watcher and Overwrite existing CM values through `cilium-agent` container

## Quick Vaildation testing
Check Cilium Management with
- `kubectl get cep -A`
