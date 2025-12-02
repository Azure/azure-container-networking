# Swiftv2 Cilium In-place Upgrade Guide

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
envsubst '${CONFLIST},${CONFLIST_PRIORITY},${CNI_IMAGE}' < test/integration/manifests/cni/conflist-installer.yaml | kubectl apply -f -
```


### Apply Cilium config
```
export DIR=1.17
export CILIUM_VERSION_TAG=v1.17.7-250927
export CILIUM_IMAGE_REGISTRY=mcr.microsoft.com/containernetworking
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-config/cilium-chained-config.yaml
```


### Apply Cilium Agent + Operator + RBAC
```
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-operator/files
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-agent/files
envsubst '${CILIUM_VERSION_TAG},${CILIUM_IMAGE_REGISTRY}' < test/integration/manifests/cilium/v${DIR}/cilium-agent/templates/daemonset.yaml | kubectl apply -f -
envsubst '${CILIUM_VERSION_TAG},${CILIUM_IMAGE_REGISTRY}' < test/integration/manifests/cilium/v${DIR}/cilium-operator/templates/deployment.yaml | kubectl apply -f -
```


### Quick Summary
- Apply conflist installer to update conflist on all nodes
- Apply Cilium Config
- Apply Agent + Operator + RBAC


## Quick Vaildation testing
- Check Cilium Management with
  - `kubectl get cep -A`
