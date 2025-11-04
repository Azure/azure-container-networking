# Swiftv2 Cilium Setup Guide

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


### Apply Watcher
```
kubectl apply -f test/integration/manifests/cilium/watcher/deployment.yaml
```

- Watcher obtains existing RBAC and DS from managed node
  - We overwrite CM values through the use of DS args on the `cilium-agent` container
i.e. overwrites `--cni-chaining-mode`
```
yq eval '.spec.template.spec.containers[0].args += ["--cni-chaining-mode=generic-veth"]' -i "$temp_file"
```



### Quick Summary
- Apply conflist installer to update conflist on BYON
- Apply Watcher and Overwrite existing CM values through `cilium-agent` container

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
