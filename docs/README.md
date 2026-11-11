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

## CNS capability discovery

Clients can call `GET /supportedapis` (also available at `/v0.2/supportedapis`)
to discover a curated set of CNS capabilities for feature detection. No request
body is required. The initial list is intentionally empty:

```json
{"supportedApis":[]}
```

The endpoint returns HTTP 200 with a `supportedApis` array, including when no
capabilities are advertised. Entries are capability identifiers, not registered
HTTP routes. Route registration, channel mode, and NMAgent's cached supported
APIs do not populate this list. Capabilities must be explicitly advertised as
they are added. Other HTTP methods return HTTP 405 with `Allow: GET`.

Go callers can use `client.SupportedAPIs(ctx)` from `cns/client`, which returns
the capability identifiers as `[]string`. Older CNS versions without this
endpoint return HTTP 404, surfaced as `*client.FailedHTTPRequest`.

## Code of Conduct
This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
