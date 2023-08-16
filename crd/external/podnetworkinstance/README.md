Pod Network Instance (PNI)

PNIs represent optional requirements, or behavior configurations for how we setup the pod networking. They should map 1:1 and follow the lifetime of a customer workload.

The object points to the PodNetwork for the delegated subnet to use and defines allocation requirements (e.g.: for IPs to reserve for pod endpoints). Orchestrator can map the deployments with these requirements to the PNI object through labels on the pod spec pointing to this object identifier. 
