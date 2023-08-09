# MultitenantPodNetworkConfig CRDs

This CRD is added to enable SWIFT multitenancy – which will be watched and managed by the DNC-RC controller.

MPNC objects represent the network configuration goal state for a pod running a multitenant networked container and are created and managed by Swift / DNC components as part of the network configuration, during Pod lifecycle events.

MPNC maps 1:1 and follow the lifetime of the pod – Swift / DNC will create these when a new pod is created, and on pod termination the MPNC will be deleted by k8 as a child resource.
