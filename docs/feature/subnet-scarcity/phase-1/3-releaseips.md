# CNS releases IPs back to Exhausted Subnets [[Phase 1 Design]](../proposal.md#1-3-ips-are-released-by-cns)

CNS will watch the `ClusterSubnetState` CRD and will update its internal state with the Subnet's exhaustion status. When the Subnet is exhausted, CNS will ignore the configured Batch size from the `NodeNetworkConfig`, and internally will use a Batch size of $1$. As the IPAM Pool Monitor reconciles the Pool, the changes to the Batch size will get picked up and applied to the subsequent Pool Scaling and target `RequestedIPCount`.

```mermaid
sequenceDiagram
participant IPAM Pool Monitor
participant ClusterSubnet Watcher
participant Kubernetes
Kubernetes->>ClusterSubnet Watcher: ClusterSubnet Update
alt Exhausted
ClusterSubnet Watcher->>IPAM Pool Monitor: Batch size = 1
else Un-exhausted
ClusterSubnet Watcher->>IPAM Pool Monitor: Batch size = 16
end
loop
IPAM Pool Monitor->>IPAM Pool Monitor: Recalculate RequestedIPCount
Note right of IPAM Pool Monitor: Request = Batch * X
IPAM Pool Monitor->>Kubernetes: Update NodeNetworkConfig CRD Spec
Kubernetes->>IPAM Pool Monitor: Update NodeNetworkConfig CRD Status
end
```
