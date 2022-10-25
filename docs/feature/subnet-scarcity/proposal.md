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
DNC-RC will poll DNC's SubnetState API on a fixed interval to check the Subnet Utilization. If the Subnet Utilization crosses some configurable lower and upper thresholds, RC will consider that Subnet un-exhausted or exhausted, respectively, and will write the exhaustion state to the ClusterSubnet CRD.

#### [[1-3]](phase-1/3-releaseips.md) IPs are released by CNS
CNS will watch the `ClusterSubnet` CRD, scaling down and releasing IPs when the Subnet is marked as Exhausted.

### Phase 2
IPs are not assigned to a new Node until CNS requests them, allowing Nodes to start safely even in very constrained subnets. CNS scaling math is improved, and CNS Scalar properties come from the ClusterSubnet CRD instead of the NodeNetworkConfig CRD.

#### [[2-1]](phase-2/1-emptync.md) DNC-RC creates NCs with no Secondary IPs
DNC-RC will create the NNC for a new Node with an initial IP Request of 0. An empty NC (containing a Primary, but no Secondary IPs) will be created via normal DNC API calls. The empty NC will be written to the NNC, allowing CNS to start. CNS will make the initial IP request according to the Subnet Exhaustion State.

DNC-RC will continue to poll the `SubnetState` API periodically to check the Subnet utilization, and write the exhaustion to the `ClusterSubnet` CRD.

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
CNS watches Pods and adjusts the SecondaryIP Count immediately in reaction to Pod IP demand changes. The NNC is revised to cut weight and prepare for the dynamic batch size (or multi-nc) future.


#### [[3-1]](phase-3/1-watchpods.md) CNS watches Pods
CNS will Watch for Pod events on its Node, and use the number of scheduled Pods to calculate the target Requested IP Count.


#### [[3-2]](phase-3/2-nncbeta.md) Revise the NNC to v1beta1
With the Scaler migration in [[Phase 2-3]](#2-3-scaler-properties-move-to-the-clustersubnet-crd), the NodeNetworkConfig will be revised to remove this object and optimize.
