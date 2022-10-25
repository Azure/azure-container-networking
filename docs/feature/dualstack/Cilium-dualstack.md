# Cilium Azure-ipam Dualstack Overlay Proposal 

## Purpose 

This document proposes changes to the azure-ipam contract so that we can work with dualstack clusters between Cilium and Azure CNS. 

## Overview 

With the current CNS solution for dualstack we are setting the nnc to pass multiple NCs with CIDR to be unrolled and added to a pool that contains both v6 and v4 ips. Since we are planning to use Cilium CNI for the linux dualstack solution we need to make changes to azure-ipam to allow for a dualstack cluster to get multiple IPs. Currently azure-Ipam handles the [`CmdAdd`](https://github.com/Azure/azure-container-networking/blob/master/azure-ipam/ipam.go#:~:text=func%20(p%20*IPAMPlugin)%20CmdAdd(args%20*cniSkel.CmdArgs)%20error%20%7B) command from Cilium CNI and sends an IP request to the CNS. The CNS then in overlay then looks through the pool and picks the first IP found that isn’t used. For dualstack we need to change two things:
1. Azure-ipam needs to be able to assign multiple IPs
2. CNS needs to be able to determine what type of IP it is returning so that we can have one IPv4 and one IPv6. 

## Current Implementation

When we use Cilium CNI with Azure CNS we use an Azure-ipam plugin to communicate from the Cilium CNI and CNS. This plugin takes in add commands from the Cilium CNI and reads the CNI network config from stdin. The data from stdin then gets parsed into an [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go) object and is passed to [`RequestIPAddress`](https://github.com/Azure/azure-container-networking/blob/master/cns/client/client.go). From here we call the [`requestIPConfigHandler`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20requestIPConfigHandler(w%20http.ResponseWriter%2C%20r%20*http.Request)%20%7B) in ipam using which then retrieves and validates the `IPConfigRequest` and then calls the [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B). The helper function first checks to see if the `IPConfigRequest` already belongs to a pod and returns the existing pod's information. If it doesn't already exist than it checks if there is a desiredIPaddress or not. If there is a desired IP address than we search for that specific IP and if not it calls 
[`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B)
## Proposal 

For this issue two solutions are being proposed.  

The first is having azure-ipam send two separate requests for both an IPv4 and an IPv6. The add command would need to be able to recognize that the pod is dualstack, create two `IPConfigRequest`s with an added type field to specify either v4 or v6 and call `RequestIPAddress` twice to get both IPs. These two IPs would then both be added to the array of IPs in `cniResult` and return them to the Cilium CNI.

```diff
type IPConfigRequest struct {
    DesiredIPAddress    string
    PodInterfaceID      string
    InfraContainerID    string
    OrchestratorContext json.RawMessage
    Ifname              string // Used by delegated IPAM
+   IPType              string // designating 'IPv4' or 'IPv6'. If not filled than we will return first IP of any type
}
```

The second solution is to send one request from azure-ipam and have it specify in its `IPConfigRequest` that it wants a dualstack pod. With solution instead of calling CNS twice to request two IPs it would only call the CNS once and would return both IPs requested in an array. This would most likely require a much more extensive code change as requesting an IP would now return an array of IPs instead of just the one IP.  

```diff
type IPConfigRequest struct {
    DesiredIPAddress    string
    PodInterfaceID      string
    InfraContainerID    string
    OrchestratorContext json.RawMessage
    Ifname              string // Used by delegated IPAM
+   PodType             string // would designate that a pod is 'dualstack' and search for two IPs instead of just one 
}
```

```diff
type IPConfigResponse struct {
-   PodIpInfo PodIpInfo 
+   PodIpInfo []PodIpInfo 
    Response  Response
}
```

For both of these proposals there would also need to be some changes to how IPs are assigned in the CNS. When we go through the pool to find an available IP in [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go) a type check would need to be added to see if it is either v4 or v6 and the type that we want to check for can be passed into the function.This type check 

## Further Questions 

For any errors what should we do if we can obtain one ip but not the other? 