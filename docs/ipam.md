# Microsoft Azure Container Networking

## IP Address Management
Azure VNET IPAM plugins manage IP address assignments to containers.

Currently, these IP addresses need to be pre-allocated from Azure VNET to each container host's network interface before becoming available to containers running on that host.

For operator-managed pools on standalone Linux VMs, see
[Cilium Operator pools with Azure CNI](cilium-operator-ipam.md). Cilium Operator
allocates NIC secondaries while Azure CNI owns local assignments and networking;
CNS and the Cilium agent/dataplane are not used.

## Allocating IP addresses for containers
This section describes how to allocate IP addresses for containers running on individual Azure IaaS VMs. If you are planning to deploy an ACS cluster, see [ACS](acs.md) instead.

Each network interface is automatically assigned a primary IP address during creation. More (secondary) IP addresses can be added to network interfaces using the following options:

* CLI: [Assigning multiple IP addresses using CLI](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-network-multiple-ip-addresses-cli)

* PowerShell: [Assigning multiple IP addresses using PowerShell](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-network-multiple-ip-addresses-powershell)

    ```PowerShell
    Add-AzureRmNetworkInterfaceIpConfig -Name $IpConfigName -NetworkInterface $Nic -Subnet $Subnet
    ```

* Portal: [Assigning multiple IP addresses using Azure Portal](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-network-multiple-ip-addresses-portal)

* Template: [Assigning multiple IP addresses using templates](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-network-multiple-ip-addresses-template)
