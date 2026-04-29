# Swiftv2 Managed Cilium Setup Guide
Use when the system nodes have managed cilium on them initially.
This guide adds unmanaged cilium to the un-managed byo nodes.
At no point should connectivity to services like core dns fail.

## Steps
- Existing Cluster Only: Follow these steps if you are starting with a cluster as described below
- All: Follow these steps if this is a new OR existing cluster

### Existing Cluster Only: I am assuming you are starting with these components
- System nodes:
  - azure dataplane
  - azure cni
  - azure cns
  - conflist is azure conflist (not chained)
  - npm
  - kube-proxy
- BYO Nodes:
  - azure cni
  - unmanaged azure cns
  - conflist is azure cni conflist (not chained)
  - no npm
  - unmanaged kube-proxy
- no cilium anywhere

### Existing Cluster Only: Upgrade to managed cilium (should remove NPM automatically)
Run the aks command like
`az aks update --name <cluster name> --resource-group <rg> --network-dataplane cilium`

### All: Checkpoint
From this point on, I am assuming you have the following
- System nodes: 
  - managed cilium agent
  - managed cilium operator
  - azure cns
  - conflist is cilium conflist (not chained)
  - no npm
  - no kube-proxy
- BYO Nodes:
  - azure cni
  - unmanaged azure cns
  - Ideally: conflist is azure cni conflist (not chained). For New Clusters: It is possible conflist is cilium conflist (not chained)-- this is fine-- you just might need to restart the node after adding the cilium watcher
  - no npm
  - Existing Cluster Only: unmanaged kube-proxy
  - For New Clusters: no kube-proxy
  - no cilium operator

### Existing Cluster Only: Create service account and cluster role binding for kube proxy 
This is optional-- do this if you want kube proxy to come back up if it gets deleted or the node restarts for some reason. Cilium once it comes up will be taking over the job of kube proxy.

### All: Clone repo + checkout branch for *.yamls
```
git clone https://github.com/Azure/azure-container-networking.git
cd azure-container-networking
git checkout master
```

### All: Update Conflist
> [!NOTE]
> You can replace `acnpublic.azurecr.io/public/containernetworking/azure-cni:v1.7.5-3` with an mcr image in prod that has the chained cilium conflist. The installer only installs the conflist (not cni) so the above image should be sufficient.
```
export CONFLIST=azure-chained-cilium.conflist
export CONFLIST_PRIORITY=05
export CNI_IMAGE=acnpublic.azurecr.io/public/containernetworking/azure-cni:v1.7.5-3
envsubst '${CONFLIST},${CONFLIST_PRIORITY},${CNI_IMAGE}' < test/integration/manifests/cni/conflist-installer-byon.yaml | kubectl apply -f -
```

### Existing Cluster Only: Apply Watcher with Alt Healthz Bind Port
```
kubectl apply -f test/integration/manifests/cilium/watcher/deployment-alt-healthz-port.yaml
```
- This is the same as the normal watcher except we set the healthz port to 50257 to not conflict with kube proxy on the unmanaged nodes
- Cilium should come up successfully
- Both daemonsets have kube proxy replacement true

### Existing Cluster Only: Remove kube-proxy
- Cilium should be able to take on the role of kube proxy
- After removing kube-proxy succeeds, we can start to apply the normal watcher below

### All: Apply Watcher
```
kubectl apply -f test/integration/manifests/cilium/watcher/deployment.yaml
```

- Watcher obtains existing Cilium Daemonset from managed node
- We overwrite Cilium Configmap values through the use of args on the `cilium-agent` container within the watcher deployment.

### All: Swiftv1 Connectivity should work at this point
If pods are stuck in creating, try restarting the node. After creating a pod you should be able to contact the cluster dns and other services.


### All: Quick Summary
- Apply conflist installer to update conflist on BYON
- Apply Watcher and Overwrite existing CM values through `cilium-agent` container

### All: Checkpoint
- System nodes: 
  - managed cilium agent
  - managed cilium operator
  - azure cns
  - conflist is cilium conflist (not chained)
  - no npm
  - no kube-proxy
- BYO Nodes:
  - azure cni
  - unmanaged azure cns
  - unmanaged cilium
  - conflist is chained cilium conflist
  - no npm
  - no unmanaged kube-proxy
  - no cilium operator

## Quick Validation testing
Check Cilium Management with
- `kubectl get cep -A`
