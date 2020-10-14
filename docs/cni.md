# Microsoft Azure Container Networking

## Azure VNET CNI Plugins
`azure-vnet` CNI plugin implements the [CNI network plugin interface](https://github.com/containernetworking/cni/blob/master/SPEC.md).

`azure-vnet-ipam` CNI plugin implements the [CNI IPAM plugin interface](https://github.com/containernetworking/cni/blob/master/SPEC.md#ip-address-management-ipam-interface).

The plugins are available on both Linux and Windows platforms.

The network and IPAM plugins are designed to work together. The IPAM plugin can also be used by 3rd party software to manage IP addresses from Azure VNET space.

This page describes how to setup the CNI plugins manually on Azure IaaS VMs. If you are planning to deploy an ACS cluster, see [ACS](acs.md) instead.

## Install
Copy the plugin package from the [release](https://github.com/Azure/azure-container-networking/releases) share to your Azure VM and extract the contents to the CNI directories.

You can also install by running the `install-cni-plugin.sh` (Linux) or `install-cni-plugin.ps1` (Windows) scripts provided in the scripts directory of this repository.

```bash
$ scripts/install-cni-plugin.sh [version]
```

```PowerShell
PS> scripts\install-cni-plugin.ps1 [version]
```

## Version
To see the version of CNI running on your vm, run the following command:

Linux
```
/opt/cni/bin/azure-vnet --version 
```
Windows
```
C:\k\azurecni\bin\azure-vnet.exe --version
```

The plugin package comes with a simple network configuration file that works out of the box. See the [network configuration](https://github.com/Azure/azure-container-networking/blob/master/docs/cni.md#network-configuration) section below for customization options.

## Build
Plugins can also be built directly from the source code in this repository.

```bash
make azure-vnet
make azure-vnet-ipam
make azure-cni-plugins
```

The first two commands build an individual plugin, whereas the third one builds both and generates a tar archive. The binaries are placed in the `output` directory.

## Network Configuration
Network configuration for CNI plugins is described in JSON format. The default location for configuration files is `/etc/cni/net.d` for Linux and `c:\k\azurecni\` for Windows.

```json
{
  "cniVersion": "0.2.0",
  "name": "azure",
  "type": "azure-vnet",
  "master": "eth0",
  "bridge": "azure0",
  "logLevel": "info",
  "ipam": {
    "type": "azure-vnet-ipam",
    "environment": "azure"
  }
}
```

The following fields are well-known and have the following meaning:

Network plugin
* `cniVersion`: Azure plugins currently support versions 0.3.0 and 0.3.1 of the [CNI spec](https://github.com/containernetworking/cni/blob/master/SPEC.md). Support for new spec versions will be added shortly after each CNI release.
* `name`: Name of the network. This property can be set to any unique value.
* `type`: Name of the network plugin. This property should always be set to `azure-vnet`.
* `mode`: Operational mode. This field is optional. See the [operational modes](https://github.com/Azure/azure-container-networking/blob/master/docs/network.md) for more details.
* `master`: Name of the host network interface that will be used to connect containers to a VNET. This field is optional. If omitted, the plugin will automatically pick a suitable host network interface. Typically, the primary host interface name is `"Ethernet"` on Windows and `"eth0"` on Linux.
* `bridge`: Name of the bridge that will be used to connect containers to a VNET. This field is optional. If omitted, the plugin will automatically pick a unique name based on the master interface index.
* `logLevel`: Log verbosity. Valid values are `info` and `debug`. This field is optional. If omitted, the plugin will log at `info` level.

IPAM plugin
* `type`: Name of the IPAM plugin. This property should always be set to `azure-vnet-ipam`.
* `environment`: Name of the environment. Valid values are `azure` for [Azure](https://azure.microsoft.com) and `mas` for [Microsoft Azure Stack](https://azure.microsoft.com/en-us/overview/azure-stack/). This field is optional. The default value is `azure`.

You can create multiple network configuration files to connect containers to multiple networks.

Network configuration files are processed in lexical order during container creation, and in the reverse-lexical order during container deletion.

## Dynamic Plugin specific fields (Capabilities / Runtime Configuration)
Plugins can request that the runtime insert dynamic configuration by explicitly listing their `capabilities` in the network configuration. Dynamic information (i.e. data that a runtime fills out) should be placed in a `runtimeConfig` section. See the [Capabilities](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md) section for more information about well known capabilities .

`azure-vnet` CNI plugin currently supports following capabilities. 

| Capability | Purpose | Spec and Example | Supported Platform |
| ---------- | ------- | ---------------- | ------------------ |
| `portMappings` | Pass mapping from ports on the host to ports in the container network namespace. | A list of portmapping entries.<br/>  <pre>[<br/>  { "hostPort": 8080, "containerPort": 80, "protocol": "tcp" },<br />  { "hostPort": 8000, "containerPort": 8001, "protocol": "udp" }<br />]<br /></pre> | Windows |
| `dns` | Dynamically configure dns according to runtime | Dictionary containing a list of `servers` (string entries), a list of `searches` (string entries), a list of `options` (string entries). <pre>{ <br> "searches" : [ "internal.yoyodyne.net", "corp.tyrell.net" ] <br> "servers": [ "8.8.8.8", "10.0.0.10" ] <br />} </pre> | Windows |

## Logs
Logs generated by `azure-vnet` plugin are available in `/var/log/azure-vnet.log` on Linux and `c:\k\azure-vnet.log` on Windows.

Logs generated by `azure-vnet-ipam` plugin are available in `/var/log/azure-vnet.log` on Linux and `c:\k\azure-vnet-ipam.log` on Windows.

## Upgrading CNI on existing kubernetes cluster deployed using acs-engine

1. ssh into a master node
```bash
$ ssh username@masternodeipaddress
```

2. Cordon the agent nodes using below command
```bash
$ kubectl get nodes -o name | cut -d / -f 2 |  xargs -I{} -n1 kubectl cordon  {}
```

3. Upgrade all nodes one by one to v1.0.11 using the below command 
```bash
$ kubectl get nodes -o name | cut -d / -f 2 | xargs -I{}  -n1 ssh -tt {} -t 'wget -O /tmp/upgrade-cni.sh https://raw.githubusercontent.com/Azure/azure-container-networking/master/scripts/install-cni-plugin.sh; chmod 755 /tmp/upgrade-cni.sh; ls -l /tmp/upgrade-cni.sh; sudo /tmp/upgrade-cni.sh v1.0.11; echo 'upgraded node ' {}; echo 'sleeping for 5 seconds before moving on to next node... press ctrl-c if you want to abort';  sleep 5'
```
 
4. Uncordon all agent nodes using below command
```bash
$ kubectl get nodes -o name | cut -d / -f 2 |  xargs -I{} -n1 kubectl uncordon  {}
```

## Using CNI in Non-AKS Environment (Linux)
### Outbound Connectivity from pods
If you have deployed kubernetes cluster via other sources(not using aks/aks-engine), you have to add following iptable command to allow outbound(internet) connectivity from pod
```bash
iptables -t nat -A POSTROUTING -m addrtype ! --dst-type local ! -d <vnet_address_space> -j MASQUERADE
```
### IP Forwarding Setting
1. IP Forwarding has to be enabled in VM. Check by running this cmd:
```bash 
sysctl net.ipv4.ip_forward
```
If it returns 1, then ip forwarding is enabled else turn on ip forwarding by running 
```bash 
sysctl -w net.ipv4.ip_forward=1
``` 
or by editing `/etc/sysctl.conf` to persist even after reboot. 

2. If default policy of FORWARD chain in filter table is ACCEPT ignore this step. You can find this by running cmd:
```bash 
sudo iptables -t filter -L FORWARD
``` 
1st line of ouptut should show default policy for that chain. If its DROP, add the following cmd: 
```bash 
sudo iptables -t filter -I FORWARD 1 -j ACCEPT
```
