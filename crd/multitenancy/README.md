List of included CRDs

# MultitenantPodNetworkConfig CRDs

MTPNC objects represent the network configuration goal state for a pod running a multitenant networked container and are created and managed by control plane as part of the network configuration, during Pod lifecycle events.


# NodeInfo CRDs

This CRD is added to enable VNET multitenancy – which will be watched and managed by the control plane.

NodeInfo objects are created by CNS as part of the node registration flow, and is used to pass any metadata from the VM needed by control plane. E.g.: vmUniqueID etc

