# Cilium Azure-ipam Dualstack Overlay Proposal 

## Purpose 

This document proposes changes to the azure-ipam contract so that we can work with dualstack clusters between Cilium and Azure CNS. 

## Overview 

With the current CNS solution for dualstack we are setting the nnc to pass multiple NCs with CIDR to be unrolled and added to a pool that contains both v6 and v4 ips. Since we are planning to use Cilium CNI for the linux dualstack solution we need to make changes to azure-ipam to allow for a dualstack cluster to get multiple IPs. Currently azure-Ipam handles the [`CmdAdd`](https://github.com/Azure/azure-container-networking/blob/master/azure-ipam/ipam.go) command from Cilium CNI and sends an IP request to the CNS. The CNS hen in overlay then looks through the pool and picks the first IP found that isn’t used. For dualstack we need to change two things: 1) Azure-ipam needs to be able to assign multiple IPs and 2) CNS needs to be able to determine what type of IP it is returning so that we can have one IPv4 and one IPv6. 

## Proposal 

For this issue two solutions are being proposed.  

The first is having azure-ipam send two separate requests for both an IPv4 and an IPv6. The add command would need to be able to recognize that the pod is dualstack, create two [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go)s with an added type field to specify either v4 or v6. These two IPs would then both be added to the array of IPs in `cniResult` and return them to the Cilium CNI. 

The second solution is to send one request from azure-ipam and have it specify in its `IPConfigRequest` that it wants a dualstack pod. With solution instead of calling CNS twice to request two IPs it would only call the CNS once and would return both IPs requested in an array. This would most likely require a much more extensive code change as requesting an IP would now return an array of IPs instead of just the one IP.  

For both of these proposals there would also need to be some changes to how IPs are assigned in the CNS. When we go through the pool to find an available IP in [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go) a type check would need to be added to see if it is either v4 or v6 and the type that we want to check for can be passed into the function.This type check 

## Further Questions 

For any errors what should we do if we can obtain one ip but not the other? 