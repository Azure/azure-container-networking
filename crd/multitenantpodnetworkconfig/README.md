# MultitenantPodNetworkConfig CRDs

This CRD is added to enable SWIFT multitenancy – which will be watched and managed by the MT-DNC-RC controller.

MPNC objects are created and managed by Swift / DNC components as part of the network configuration, during Pod lifecycle events (see Scenarios).  
 
These represent the network configuration goal state for a pod running a multitenant networked container and will be created by MT-DNC-RC on pod object creation as the request for NIC + IP allocation.  

MPNC maps 1:1 and follow the lifetime of the pod – Swift / DNC will create these when a new pod is created, and on pod termination the MPNC will be deleted by k8 as a child resource, which will trigger request deallocation and cleanup from DNC.  
