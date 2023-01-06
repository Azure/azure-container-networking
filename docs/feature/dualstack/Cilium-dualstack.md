# Cilium Azure-ipam Dual Stack Overlay Proposal 

## Purpose 

This document proposes changes to the azure-ipam contract so that we can work with dual stack clusters between Cilium and Azure CNS. 

## Overview 

With the current CNS solution for dual stack we are setting the nnc to pass multiple NCs with CIDR to be unrolled and added to a pool that contains both v6 and v4 ips. Since we are planning to use Cilium CNI for the linux dual stack solution we need to make changes to azure-ipam to allow for a dual stack cluster to get multiple IPs. Currently azure-Ipam handles the [`CmdAdd`](https://github.com/Azure/azure-container-networking/blob/master/azure-ipam/ipam.go#:~:text=func%20(p%20*IPAMPlugin)%20CmdAdd(args%20*cniSkel.CmdArgs)%20error%20%7B) command from Cilium CNI and sends an IP request to the CNS. The CNS then in overlay then looks through the pool and picks the first IP found that isn’t used. For dual stack we need to change two things:
1. Azure-ipam needs to be able to assign multiple IPs
2. CNS needs to be able to determine what type of IP it is returning so that we can have one IPv4 and one IPv6. 

## Current Implementation

When we use Cilium CNI with Azure CNS we use an Azure-ipam plugin to communicate from the Cilium CNI and CNS. This plugin takes in add commands from the Cilium CNI and reads the CNI network config from stdin. The data from stdin then gets parsed into an [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go) object and is passed to [`RequestIPAddress`](https://github.com/Azure/azure-container-networking/blob/master/cns/client/client.go). From here we call the [`requestIPConfigHandler`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20requestIPConfigHandler(w%20http.ResponseWriter%2C%20r%20*http.Request)%20%7B) in ipam using which then retrieves and validates the `IPConfigRequest` and then calls the [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B). The helper function first checks to see if the `IPConfigRequest` already belongs to a pod and returns the existing pod's information. If it doesn't already exist than it checks if there is a desiredIPaddress or not. If there is a desired IP address than we search for that specific IP and if not it calls 
[`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B)
## New Implementation

### NNC change

For the CNS to know that we are in dual stack we will use a flag that will be passed in using the NNC. This will be filled in from the DNC-RC side and then sent to the CNS over the CRD in a flag called `IPMode`. The IPMode flag will be of type IPMode and will have two options "SingleStack" which when passed in will have the CNS work as it currently does and "Dual Stack" which tell the CNS to look for two IPs instead of one.

```diff
// NodeNetworkConfigStatus defines the observed state of NetworkConfig
type NodeNetworkConfigStatus struct {
	AssignedIPCount   int                `json:"assignedIPCount,omitempty"`
	Scaler            Scaler             `json:"scaler,omitempty"`
	Status            Status             `json:"status,omitempty"`
+   IPMode            IPMode             `json:"IPmode,omitempty"`
	NetworkContainers []NetworkContainer `json:"networkContainers,omitempty"`
}
```

```diff
+type IPMode string
+
+const (
+	SingleStack Status = "SingleStack"
+	DualStack   Status = "DualStack"
+)
```

In the CNS will flag then be used in the [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B) function to determine whether or not are creating a pod in single stack or dual stack. 

### Contract IP change

We will continue to send only one `IPConfigRequest` when we are running in dual stack but when in dual stack we will expect the repsonse to include a slice of `PodIpInfo`. When running the [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B) function with the new flag for dual stack if we are running in dual stack we will have the function pick the first availabe IPv6 and first available IPv4 IP that it can find. These IPs will now be returned as a slice from `AssignAnyAvailableIPConfig` and used to populate the `IPConfigResponse` struct and returned back to the azure-ipam. The azure-ipam will then cycle the length of the slice to populate the message the is sent to the Cilium-CNI.

```
type IPConfigRequest struct {
    DesiredIPAddress    string
    PodInterfaceID      string
    InfraContainerID    string
    OrchestratorContext json.RawMessage
    Ifname              string // Used by delegated IPAM
}
```

```diff
type IPConfigResponse struct {
-   PodIpInfo PodIpInfo 
+   PodIpInfo []PodIpInfo 
    Response  Response
}
```


