# Subnet Scarcity 
Dynamic SWIFT IP Overhead Reduction (aka IP Reaping)

## Abstract
AKS clusters using Azure CNI assign VNET IPs to Pods such that those Pods are reachable on the VNET.
In Dynamic mode (SWIFT), IP addresses are reserved out of a customer specified Pod Subnet and allocated to the cluster Nodes, and then assigned to Pods as they are created. IPs are allocated to Nodes in batches, based on the demand for Pod IPs on that Node.

Since the IPs are allocated in batches, there is always some overhead of IPs allocated to a Node but unused by any Pod. This over-reservations of IPs from the Subnet will eventually lead to IP exhaustion in the Pod Subnet, even though the number of IPs assigned to Pods is lower than the Pod Subnet capacity.

The intent of this feature is to reduce the IP wastage by reclaiming unassigned IPs from the Nodes as the Subnet utilization increases.

## Background
In SWIFT, IPs are allocated to Nodes in batches $B$ according to the request for Pod IPs on that Node. CNS runs on the Node and is the IPAM for that Node. As Pods are scheduled, the CNI requests IPs from CNS. CNS assigns IPs from its allocated IPAM Pool, and dynamically scales the pool according to utilization as follows:
- If the unassigned IPs in the Pool falls below a threshold ( $m$ , the minimum free IPs), CNS requests a batch of IPs from DNC-RC.
- If the unassigned IPs in the Pool exceeds a threshold ( $M$ , the maximum free IPs), CNS releases a batch of IPs back to the subnet.

The minimum and maximum free IPs are calculated using a fraction of the Batch size. The minimum free IP quantity is the minimum free fraction ( $mf$ ) of the batch size, and the maximum free IP quantity is the maximum free fraction ( $Mf$ ) of the batch size. For convergent scaling behavior, the maximum free fraction must be greater than 1 + the minimum free fraction.

Therefore the scaling thresholds $m$ and $M$ can be described by:

$$
m = mf \times B \text{ , } M = Mf \times B \text{ , and } Mf = mf + 1
$$

For $B > 1$, this means that for a cluster of size $N$ Nodes, there is at least $m * N$ wastage of IPs at steady-state, and at most $M * N$.

$$
m \times N \lt \text{Wasted IPs} \lt M \times N
$$ 

For total Subnet capacity ( $Q$ ) and reserved Subnet capacity ( $R$ ), CNS may be unable to request additional IPs and thus Kubernetes may be unable to start additional Pods if the Subnet's unreserved capacity is insufficient:

$$
Q - R < B
$$

In this scenario, no Node’s request for IPs can be fulfilled as there are less than $B$ IPs left unreserved in the Subnet. However, for any $B>1$, the Reserved capacity is not the actual assigned Pod IPs, and unassigned IPs could be reclaimed from Nodes which have reserved them and reallocated to Nodes which need them to provide assignable capacity.

Thus, to allow real full utilization of all usable IPs in the Pod Subnet, these parameters (primarily $B$) need to be tuned at runtime according to the ongoing subnet utilization.

## Solutions and Complications
The following solutions are proposed to address the IP wastage and reclaim unassigned IPs from Nodes.

### Phase 1
Subnet utilization is cached by DNC, exhaustion is calculated by DNC-RC which writes it to a ClusterSubnetState CRD, which is read by CNS to trigger the release of IPs.

#### [[1-1]](phase-1/1-subnetstate.md) Subnet utilization is cached by DNC 
DNC (which maintains the state of the Subnet in its database) will cache the reserved IP count $R$ 
per Subnet. DNC will also expose an API to query $R$ of the Subnet, the `SubnetState` API.

#### [[1-2]](phase-1/2-exhaustion.md) Subnet Exhaustion is calculated by DNC-RC
DNC-RC will poll the `SubnetState` API to periodically check the Subnet utilization. DNC-RC will be configured with a lower and upper threshold ( $t$ and $T$ ) as fractions of the Subnet capacity $Q$. If the Subnet utilization crosses the upper threshold, DNC-RC will consider the Subnet "exhausted". If the Subnet utilization falls below the lower threshold, DNC-RC will consider the Subnet "not exhausted". Two values are necessary to induce hysteresis and avoid continous oscillation between the two states.

$$
E = !E \text{(toggle exhaustion) when}\begin{cases}
R \gt T \times Q &\text{if not exhausted}\\
R \lt t \times Q &\text{if exhausted}
\end{cases}
$$

If the Subnet is exhausted, DNC-RC will write an additional per-subnet CRD, the [`ClusterSubnetState`](https://github.com/Azure/azure-container-networking/blob/master/crd/clustersubnetstate/api/v1alpha1/clustersubnetstate.go), with a Status of `exhausted=true`. When the Subnet is not exhausted, DNC-RC will write the Status as `exhausted=false`.

#### [[1-3]](phase-1/3-releaseips.md) IPs are released by CNS
CNS will watch the `ClusterSubnetState` CRD and will update its internal state with the Subnet's exhaustion status. When the Subnet is exhausted, CNS will ignore the configured Batch size from the `NodeNetworkConfig`, and instead will scale in Batches of 1 IP. This will have the effect of releasing almost every unassigned IP back to the Subnet - 1 free IP will be kept in the Node's IPAM Pool, and scaling up or down will be done in increments of 1 IP.

### Phase 2
The batch size $B$ is dynamically adjusted based on the current subnet utilization. The batch size is increased when the subnet utilization is low, and decreased when the subnet utilization is high. IPs are not assigned to a new Node until CNS requests them, allowing Nodes to start safely even in very constrained subnets.

#### [[2-1]](phase-2/1-emptync.md) DNC-RC creates NCs with no Secondary IPs
DNC-RC will create the NNC for a new Node with an initial IP Request of 0. An empty NC (containing a Primary, but no Secondary IPs) will be created via normal DNC API calls. The empty NC will be written to the NNC, allowing CNS to start. CNS will make the initial IP request according to the Subnet Exhaustion State.

DNC-RC will continue to poll the `SubnetState` API periodically to check the Subnet utilization, and write the exhaustion to the `ClusterSubnetState` CRD.

#### [[2-2]](phase-2/2-scalingmath.md) CNS scales IPAM pool idempotently
Instead of increasing/decreasing the Pool size by 1 Batch at a time to try to satisfy the min/max free IP constraints, CNS will calculate the correct target Requested IP Count using a single O(1) algorithm.

This idempotent Pool scaling formula is:

$$
Request = B \times \lceil mf + \frac{U}{B} \rceil
$$

where $U$ is the number of Assigned (Used) IPs on the Node.

CNS will include the NC Primary IP(s) as IPs that it has been allocated, and will subtract them from its real Requested IP Count such that the _total_ number of IPs allocated to CNS is a multiple of the Batch.

#### [[2-3]](phase-2/3-subnetscaler.md) Scaler properties move to the ClusterSubnet CRD
The Scaler properties from the v1alpha/NodeNetworkConfig `Status.Scaler` definition are moved to the ClusterSubnet CRD, and CNS will use the Scaler from this CRD as priority when it is available, and fall back to the NNC Scaler otherwise. The `.Spec` field of the CRD may serve as an "overrides" location for runtime reconfiguration.

### Phase 3
#### [[3-1]](phase-3/1-watchpods.md) CNS watches Pods


#### CNS stops watching the ClusterSubnetState
#### DNC-RC iteratively adjusts the Batch size
