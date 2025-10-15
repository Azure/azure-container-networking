# Swiftv2 Cilium Setup Guide

## Steps
### Clone repo + checkout branch for *.yamls
```
git clone https://github.com/Azure/azure-container-networking.git
git checkout jpayne3506/conflist-generation < TODO Change before merge >
```

### Apply cilium config
```
export DIR=1.17
export CILIUM_VERSION_TAG=v1.17.7-250927
export CILIUM_IMAGE_REGISTRY=mcr.microsoft.com/containernetworking
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-config/cilium-chained-config.yaml
```

- Remove `kube-proxy-replacement-healthz-bind-address: "0.0.0.0:10256"` from configmap if kube-proxy is current on nodes

### Apply cilium Agent + Operator
```
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-operator/files
kubectl apply -f test/integration/manifests/cilium/v${DIR}/cilium-agent/files
```

### Apply/Edit CNS configmap
```
kubectl apply -f test/integration/manifests/cnsconfig/azcnichainedciliumconfigmap.yaml
```
#### Must have configmap values
```
"ProgramSNATIPTables": false
"CNIConflistScenario": "azurecni-chained-cilium"
"CNIConflistFilepath": "/etc/cni/net.d/05-azure-chained-cilium.conflist"
```

### Update CNS image
Leverage a cns build from branch or use `acnpublic.azurecr.io/azure-cns:v1.7.5-2-g94c36c070` < TODO Change before merge >
- This will install our chained conflist through the use of `"CNIConflistScenario": "azurecni-chained-cilium"` and it will be installed on the node here `"CNIConflistFilepath": "/etc/cni/net.d/05-azure-chained-cilium.conflist"`

> NOTE: if your current conflist file name starts with `05` then change our previous filename to one with higher priority to ensure that it is consumed on restart. I.e. `03-azure-chained-cilium.conflist`

### If kube-proxy was present
#### Remove kube-proxy
> NOTE: Reapply `kube-proxy-replacement-healthz-bind-address: "0.0.0.0:10256"` to cilium configmap if previously removed

This can be done either by editing the node-selectors or deleting the ds. Both work...

#### Restart Cilium
kubectl rollout restart ds -n kube-system cilium


### Quick Summary
- Apply/Edit Cilium Config with
  - `cni-chaining-mode: generic-veth`
  - remove `kube-proxy-replacement-healthz-bind-address`
    - You do not need to remove if node does not have kube-proxy enabled
  - If applied before agent is in ready state then no need to restart agent
- Apply Agent + Operator
- Apply/Edit CNS config with
  - "ProgramSNATIPTables": false
  - "CNIConflistScenario": "azurecni-chained-cilium"
  - "CNIConflistFilepath": "/etc/cni/net.d/05-azure-chained-cilium.conflist"
- Update CNS image with build from branch or < TODO IMAGE NAME >
  - This will install chained conflist

#### If kube-proxy was present
- Reapply `kube-proxy-replacement-healthz-bind-address: "0.0.0.0:10256"` to cilium configmap
- Remove Kube-proxy
- Restart Cilium


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
