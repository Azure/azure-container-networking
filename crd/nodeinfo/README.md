# NodeInfo CRDs

This CRD is added to enable VNET multitenancy – which will be watched and managed by the control plane.

NodeInfo objects are created by CNS as part of the node registration flow, and is used to pass any metadata from the VM needed by control plane. E.g.: vmUniqueID etc
