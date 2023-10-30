List of included CRDs

# MultitenantPodNetworkConfig CRDs

MTPNC objects represent the network configuration goal state for a pod running a multitenant networked container and are created and managed by control plane as part of the network configuration, during Pod lifecycle events.

# NodeInfo CRDs

This CRD is added to enable VNET multitenancy – which will be watched and managed by the control plane.

NodeInfo objects are created by CNS as part of the node registration flow, and is used to pass any metadata from the VM needed by control plane. E.g.: vmUniqueID etc

# PodNetwork CRDs

This CRD is added to enable VNET multitenancy – which will be watched and managed by the control plane.

PodNetwork objects need to be created by Orchestrator in the subnet delegation flow.
These represent a Cx subnet already delegated by the customer to the Orchestrator and locked with a Service Association Link (SAL) on network RP.

# Pod Network Instance (PNI)

PNIs represent optional requirements, or behavior configurations for how we setup the pod networking. They should map 1:1 and follow the lifetime of a customer workload.

The object points to the PodNetwork for the delegated subnet to use and defines allocation requirements (e.g.: for IPs to reserve for pod endpoints). Orchestrator can map the deployments with these requirements to the PNI object through labels on the pod spec pointing to this object identifier. 


