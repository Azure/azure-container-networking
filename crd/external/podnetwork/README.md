# PodNetwork CRDs

This CRD is added to enable VNET multitenancy – which will be watched and managed by the control plane.

PodNetwork objects need to be created by Orchestrator in the subnet delegation flow.
These represent a Cx subnet already delegated by the customer to the Orchestrator and locked with a Service Association Link (SAL) on network RP.
