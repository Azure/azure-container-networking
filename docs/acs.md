# Microsoft Azure Container Networking

## Azure Container Service
Azure VNET plugins are designed to work with [Azure Container Service](https://azure.microsoft.com/en-us/services/container-service).

The deployment and configuration of plugins is automatic for clusters created by [acs-engine](https://github.com/Azure/acs-engine) when Azure VNET network policy is enabled. See acs-engine [network policy examples](https://github.com/Azure/acs-engine/tree/master/examples/networkpolicy) for sample deployment templates.

> The Azure VNET network policy for Linux containers is currently available as a **public preview** for Kubernetes. Support for Windows containers and other orchestrators is coming soon.

## Deploying an ACS Kubernetes cluster with kubenet vs. Azure VNET plugins
When you deploy a Kubernetes cluster in ACS, by default it is configured to use a networking plugin called [kubenet](http://kubernetes.io/docs/admin/network-plugins). With kubenet, nodes and pods are placed on different IP subnets. The nodes’ subnet is an actual Azure VNET subnet, whereas the pods’ subnet is allocated by Kubernetes Azure cloud provider and is not known to the Azure SDN stack. IP connectivity between the two subnets is achieved by configuring user-defined routes and enabling IP-forwarding on all node network interfaces.

This is a limited solution because the pod subnets and IP addresses are hidden from Azure SDN stack. It requires you to manage two network policies, one for VMs and another one for containers. It also has lower performance because all container traffic needs to be forwarded between pod and node IP interfaces in each direction, introducing computational overhead and networking delay.

Azure VNET plugin attaches all nodes and pods, on both Linux and Windows, to a single flat Azure IP subnet. Container network interfaces are connected directly to Azure VNET and are assigned actual IP addresses from Azure VNET address space. This allows full integration with SDN features such as network security groups and VNET peering, enabling you to manage your VMs and containers with a single unified network policy. Network traffic need not be translated or IP-forwarded in either direction.

## Enabling Azure VNET plugins for an ACS Kubernetes cluster
To deploy an ACS Kubernetes cluster with Azure VNET plugins, follow the ACS [Kubernetes walkthrough](https://github.com/Azure/acs-engine/blob/master/docs/kubernetes.md) and set the "networkPolicy" property in kubernetesConfig to "azure" when editing the json file:

```bash
"orchestratorProfile": {
    "orchestratorType": "Kubernetes",
    "kubernetesConfig": {
        "networkPolicy": "azure"
    },
    ...
}
```

You can also look at acs-engine [network policy examples](https://github.com/Azure/acs-engine/tree/master/examples/networkpolicy) for sample deployment templates.

`acs-engine` will allocate 128 IP addresses on each node by default. Depending on the size of your subnets, you can specify a custom value per agent pool or the master profile by setting the `ipAddressCount` property in `agentPoolProfile` and the `masterProfile`. The custom value must be in range 1-256.

```bash
  "masterProfile": {
    "count": 1,
    "ipAddressCount": 64,
    ...
    },
  "agentPoolProfiles": [
      {
        "name": "agentpool1",
        "count": 8,
        ...
        "ipAddressCount": 150,
        ...
      }
    ],
```

The plugins are located in the `/opt/cni/bin` directory. The logs are in `/var/log/azure-vnet.log` and `/var/log/azure-vnet-ipam.log`.
