# CNS and Azure-ipam dualstack Overlay Proposal 

## Purpose 

This document proposes changes to the azure-ipam contract and implement a new API so that we can work with dualstack clusters between Cilium and Azure CNS. 

## Overview 

With the current DNC/DNC-RC solution for dualstack we are setting the NNC to pass multiple NCs with a CIDR to be unrolled and added to a pool that contains both v6 and v4 IPs. For CNI/CNS we need to be able to ensure that we are able to assign an IP from both of these NCs when creating pods and will do so using the following changes:
1. Create a new contract type to allow for a slice of IPs to be returned
2. Create a new API that acts similar to the current API but uses the new contract as a return type to return multiple IPs
3. Make changes to existing functions in ipam so that one IP is assigned per NC to a pod

## Current Implementation

To communicate to Cilium CNI from Azure CNS we use the azure-ipam plugin. This plugin takes in add commands from the Cilium CNI and reads the CNI network config from stdin. The data from stdin then gets parsed into an [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go) object and is passed to [`RequestIPAddress`](https://github.com/Azure/azure-container-networking/blob/master/cns/client/client.go). From here we call the [`requestIPConfigHandler`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20requestIPConfigHandler(w%20http.ResponseWriter%2C%20r%20*http.Request)%20%7B) in ipam which then retrieves and validates the [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go) and then calls the [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B). The helper function first checks to see if the [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go) already belongs to a pod and returns the existing pod's information. If it doesn't already exist than it checks if there is a desired IP address or not. If there is a desired IP address than we search for that specific IP and if not we call 
[`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B). This function then searches for the first available IP and returns it back up to the handler. The handler then packages the IP into a [`IPConfigResponse`](https://github.com/Azure/azure-container-networking/blob/bd299fe7271a7a23b3d0268d8e14ad812181e076/cns/NetworkContainerContract.go#L419) which gets returned to the client.
## New Implementation

### New Contract Type

To preserve the current contract type but also to allow for the new functionality we will add a new type, `IPConfigsResponse` to the contract. This new type will be similar to the prexisting [`IPConfigResponse`](https://github.com/Azure/azure-container-networking/blob/bd299fe7271a7a23b3d0268d8e14ad812181e076/cns/NetworkContainerContract.go#L419) but replacing the single `PodIpInfo` with a slice. This will allow for a single call to the API to return one IP per NC.

```diff
// IPConfigResponse is used in CNS IPAM mode as a response to CNI ADD
type IPConfigResponse struct {
	PodIpInfo PodIpInfo
	Response  Response
}

+// IPConfigsResponse is used in CNS IPAM mode to return a slice of IP configs as a response to CNI ADD
+type IPConfigsResponse struct {
+	PodIpInfo []PodIpInfo
+	Response  Response
+}
```

### New API

A new API will called `RequestIPs` be created for requesting IPs when running in Overlay. This will work similar to the current [`RequestIPAddress`](https://github.com/Azure/azure-container-networking/blob/master/cns/client/client.go) and will accept the same [`IPConfigRequest`](https://github.com/Azure/azure-container-networking/blob/master/cns/NetworkContainerContract.go), the main difference will be that it will return the new contract type `IPConfigsResponse` with the slice of `PodIpInfo`. 

```diff
+// RequestIPs calls the RequestIPConfigs in CNS
+func (c *Client) RequestIPs(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigsResponse, error)
}
```

If for some reason this function returns a 404 error we will then try again but use the original [`RequestIPAddress`](https://github.com/Azure/azure-container-networking/blob/master/cns/client/client.go) API instead. This will also make it so that the new azure-ipam will be backwards compatible with previous versions of CNS that do not have the new API .

### IPAM Changes

In IPAM we will need to have a new handler that is called by the new API called `requestIPsHandler`. This new handler will act similarly as the current [`requestIPConfigHandler`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20requestIPConfigHandler(w%20http.ResponseWriter%2C%20r%20*http.Request)%20%7B) handler with the exception of using the new contract `IPConfigsResponse`. This new function will also call [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B) to retrieve an IP however the return type will be changed from a single `PodIpInfo` to a slice so that we can get all IPs needed with one call. [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B) will still call [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B) but will now be expecting for a slice of IPs to be returned. Within [`AssignAnyAvailableIPConfig`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20AssignAnyAvailableIPConfig(podInfo%20cns.PodInfo)%20(cns.PodIpInfo%2C%20error)%20%7B) instead of just getting the first available IP, it will now fill a slice with one IP from each NC provided on the NNC. This will then get returned back up to the handler function and then sent back to the client.[`GetExistingIPConfig`](https://github.com/Azure/azure-container-networking/blob/9dc035473b01ea4b72f5c3a3cb21d5a4fbc3d9cd/cns/restserver/ipam.go#L481) and [`AssignDesiredIPConfig`](https://github.com/Azure/azure-container-networking/blob/9dc035473b01ea4b72f5c3a3cb21d5a4fbc3d9cd/cns/restserver/ipam.go#L504) will also be updated to support multiple IPs on a pod by being able to return a slice of IP configs for an existing pod and for allowing a slice of IPs to be desired when creating a pod respectively.

```diff
+// used to request an IPConfig for each NC from the CNS state
+func (service *HTTPRestService) requestIPsHandler(w http.ResponseWriter, r *http.Request)
```

```diff
-func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigRequest) (cns.PodIpInfo, error) 
+func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigRequest) ([]cns.PodIpInfo, error) 
```

```diff
-func (service *HTTPRestService) AssignAnyAvailableIPConfig(podInfo cns.PodInfo) (cns.PodIpInfo, error) 
+func (service *HTTPRestService) AssignAnyAvailableIPConfig(podInfo cns.PodInfo) ([]cns.PodIpInfo, error) 
```

To keep backwards compatibility the current handler, [`requestIPConfigHandler`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20(service%20*HTTPRestService)%20requestIPConfigHandler(w%20http.ResponseWriter%2C%20r%20*http.Request)%20%7B) will still accept the slice from [`requestIPConfigHelper`](https://github.com/Azure/azure-container-networking/blob/master/cns/restserver/ipam.go#:~:text=func%20requestIPConfigHelper(service%20*HTTPRestService%2C%20req%20cns.IPConfigRequest)%20(cns.PodIpInfo%2C%20error)%20%7B) but will then have to retrieve the first index of the slice so that it can be used in the old version of the contract, [`IPConfigResponse`](https://github.com/Azure/azure-container-networking/blob/bd299fe7271a7a23b3d0268d8e14ad812181e076/cns/NetworkContainerContract.go#L419).
