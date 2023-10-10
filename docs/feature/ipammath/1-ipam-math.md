## CNS IPAM Scaling v2

### Scaling Math

The Pool scaling process can be improved by directly calculating the target Pool size based on the current IP usage on the Node. Using this idempotent algorithm, we will calculate the correct target Pool size in a single step based on the current IP usage, instead of free, future-free, etc.

The O(1) Pool scaling formula is:

$$
Request = B \times \lceil mf + \frac{U}{B} \rceil
$$

> Note: $\lceil ... \rceil$ is the ceiling function.

where $U$ is the number of Assigned (Used) IPs on the Node, $B$ is the Batch size, and $mf$ is the Minimum Free Fraction, as discussed in the [Background](0-background.md#background).

The resulting IP Count is forward looking without effecting the correctness of the Request: it represents the target quantity of IP addresses that CNS should have at any instant in time based on the current real IP demand, and does not in any way depend on what the current or previous Requested IP count is or whether there are unsatisfied requests currently in-flight.

A concrete example:

$$
\displaylines{
    \text{Given: }\quad B=16\quad mf=0.5 \quad U=25 \text{ scheduled Pods}\\
    Request = 16 \times \lceil 0.5 + \frac{25}{16} \rceil\\
    Request = 16 \times \lceil 0.5 + 1.5625 \rceil\\
    Request = 16 \times \lceil 2.0625 \rceil\\
    Request = 16 \times 3 \\
    Request = 48
}
$$

As shown, if the demand is for $25$ IPs, and the Batch is $16$, and the Min Free is $8$ (0.5 of the Batch), then the Request must be $48$. $32$ is too few, as $32-25=7 < 8$. The resulting request is also (and will always be) immediately aligned to a multiple of the Batch ($3B$)

This algorithm will significantly improve the time-to-pod-ready for large changes in the quantity of scheduled Pods on a Node, due to eliminating all iterations required for CNS to converge on the final Requested IP Count.


### Including PrimaryIPs

The IPAM Pool scaling operates only on NC SecondaryIPs. However, CNS is allocated an additional `PrimaryIP` for every NC as a prerequisite of that NC's existence. Therefore, to align the **real allocated** IP Count to the Batch size, CNS should deduct those PrimaryIPs from its Requested (Secondary) IP Count.

This makes the RequestedIPCount:

$$
RequestedIPCount = B \times \lceil mf + \frac{U}{B} \rceil - PrimaryIPCount
$$
