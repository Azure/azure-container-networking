# Swiftv2 Cilium Upgrade Guide

## Steps
### Clone repo + checkout branch for *.yamls
```
git clone https://github.com/Azure/azure-container-networking.git
git checkout jpayne3506/conflist-generation < TODO Change before merge >
```

### Update Conflist
Leverage a cni build from branch or use `acnpublic.azurecr.io/azure-cni:linux-amd64-v1.7.5-3-g93d32acd0` < TODO Change before merge >
- This will install our chained conflist through the use of `test/integration/manifests/cni/conflist-installer.yaml`

```
export CONFLIST=azure-chained-cilium.conflist
export CONFLIST_PRIORITY=05
export CNI_IMAGE=acnpublic.azurecr.io/azure-cni:linux-amd64-v1.7.5-3-g93d32acd0
envsubst '${CONFLIST},${CONFLIST_PRIORITY},${CNI_IMAGE}' < test/integration/manifests/cni/conflist-installer.yaml | kubectl apply -f -
```

> NOTE: if your current conflist file name starts with `05` then change our previous filename to one with higher priority to ensure that it is consumed. i.e. `03-azure-chained-cilium.conflist`


### Apply cilium config
```
export DIR=1.17
export CILIUM_VERSION_TAG=v1.17.7-250927
export CILIUM_IMAGE_REGISTRY=mcr.microsoft.com/containernetworking
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-config/cilium-chained-config.yaml
```

- Remove `kube-proxy-replacement-healthz-bind-address: "0.0.0.0:10256"` from configmap if kube-proxy is current on nodes

### Apply cilium Agent + Operator + RBAC
```
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-operator/files
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-agent/files
envsubst '${CILIUM_VERSION_TAG},${CILIUM_IMAGE_REGISTRY}' < test/integration/manifests/cilium/v${DIR}/cilium-agent/templates/daemonset.yaml | kubectl apply -f -
envsubst '${CILIUM_VERSION_TAG},${CILIUM_IMAGE_REGISTRY}' < test/integration/manifests/cilium/v${DIR}/cilium-operator/templates/deployment.yaml | kubectl apply -f -
```


### Quick Summary
- Apply conflist installer to update conflist on BYON
- Apply/Edit Cilium Config with
  - `cni-chaining-mode: generic-veth`
  - remove `kube-proxy-replacement-healthz-bind-address`
    - You do not need to remove if node does not have kube-proxy enabled
  - If applied before agent is in ready state then no need to restart agent
- Apply Agent + Operator + RBAC


## Quick Vaildation testing
- Create pods from deploy
  - test/integration/manifests/swiftv2/mt-deploy.yaml
  - Creates `container-*` pods on default namespace
- Create Cilium Network Policies
  - test/integration/manifests/cilium/netpol/default-allow.yaml
  - Will only allow cilium managed endpoints to transmit traffic through default namespace
- Check Cilium Management with
  - `kubectl get cep -A`
  - `kubectl get cnp -A`
- Check connectivity
  - exec -it <container-*> -- sh
  - ip a
    - look for delegatedNIC IP
  - ping <IP>
  - confirm CNP working by attempting to ping coredns pods
    - should fail if both are being maintained by cilium
    - confirm with `kubectl get cep -A`
