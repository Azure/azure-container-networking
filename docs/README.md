# Microsoft Azure Container Networking

## Table of Contents
* [CNI plugin](cni.md) - describes how to setup Azure CNI plugins.
* [Azure CNI Powered By Cilium](cilium.md) - describes the next generation of Azure CNI Plugin powered by Cilium dataplane.
* [Azure CNI Overlay Mode for AKS](overlay-for-aks.md) - describes a mode of the Azure CNI Plugin to provide a Pod network from an overlay address space with no encapsulation.
* [Azure CNI Transparent-tunnel Mode](transparent-tunnel.md) - describes transparent-tunnel datapath rules for same-node NSG enforcement.
* [ACS](acs.md) - describes how to use the plugins with Azure Container Service.
* [Network](network.md) - describes container networks created by plugins.
* [IPAM](ipam.md) - describes how container IP address management is done by plugins.
* [NPM](npm.md) - describes how to setup Azure-NPM (Azure Network Policy Manager).
* [Scripts](scripts.md) - describes how to use the scripts in this repository.

## CNS RNC capability discovery

In Direct channel mode, `GET /rncsupport` (also available at
`/v0.2/rncsupport`) reads the NMAgent monitor's cache without making an NMAgent
request. The monitor shares the HomeAZ polling interval, configured by
`AZRSettings.PopulateHomeAzCacheRetryIntervalSecs` (60 seconds by default).

The endpoint uses the same HTTP 200 and CNS response-code convention as
`GET /homeaz`. A successful discovery returns:

```json
{
  "response": { "ReturnCode": 0, "Message": "RNC support query succeeded" },
  "rncSupportResponse": { "isSupported": true }
}
```

Only read `isSupported` when `ReturnCode` is `Success`. A successful query
without `NetworkManagementRNCSupport` returns `Success` with `isSupported: false`.
An unpopulated cache returns `NotFound`; a discovery failure returns
`NmAgentSupportedApisError`, replacing any earlier successful result. These
errors do not mean RNC is unsupported. HomeAZ failures do not affect RNC discovery.

The Go CNS client exposes `GetRNCSupport(ctx)`. It returns `CNSClientError` for
non-success CNS codes and maps an HTTP 404 from an unavailable endpoint (for
example, an older CNS version) to `UnsupportedAPI`, distinguishable through
`client.IsUnsupportedAPI(err)`. An empty cache is an HTTP 200 CNS `NotFound`
result, not an unavailable endpoint. The existing HomeAZ API is unchanged.

## Code of Conduct
This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
