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
  - Ideally: conflist is azure cni conflist (not chained). For New Clusters: It is possible conflist is cilium conflist (not chained)-- this is fine-- you just might need to restart the node after adding the cilium unmanaged ds
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

### Existing Cluster Only: Apply Unmanaged Cilium Daemonset with Alt Healthz Bind Port
Unmanaged daemonset uses Cilium v1.18.6 (matches AKS managed Cilium for k8s 1.34 overlay clusters).
```
kubectl apply -f test/integration/manifests/cilium/v1.18/unmanaged/daemonset-alt-healthz-port.yaml
```
- This is the same as the normal unmanaged cilium ds except we set the healthz port to 50257 to not conflict with kube proxy on the unmanaged nodes
- Cilium should come up successfully
- The managed and unmanaged Cilium daemonsets both have kube-proxy replacement enabled

### Existing Cluster Only: Remove kube-proxy
- Cilium should be able to take on the role of kube proxy
- After removing kube-proxy succeeds, we can start to apply the normal unmanaged cilium ds below

### All: Apply Unmanaged Cilium Daemonset
Assuming k8s 1.34. Unmanaged daemonset uses Cilium v1.18.6 (matches AKS managed Cilium for k8s 1.34 overlay clusters).
```
kubectl apply -f test/integration/manifests/cilium/v1.18/unmanaged/daemonset.yaml
```

- We override Cilium configmap defaults via args on the `cilium-agent` container in the unmanaged daemonset.

### All: Swiftv1 Connectivity should work at this point
If pods are stuck in creating, try restarting the node. After creating a pod you should be able to contact the cluster dns and other services.


### All: Quick Summary
- Apply conflist installer to update conflist on BYON
- Apply unmanaged cilium ds and override CM defaults via args on the `cilium-agent` container

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
