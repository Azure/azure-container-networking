# Microsoft Azure Container Networking

## Azure VNET CNM (libnetwork) Plugin
The `azure-vnet` libnetwork plugin implements the Docker libnetwork [network](https://github.com/docker/libnetwork/blob/master/docs/remote.md) and [IPAM](https://github.com/docker/libnetwork/blob/master/docs/ipam.md) remote plugin interfaces.

The plugin is available on both Linux and Windows platforms.

The network and IPAM plugins are designed to work together. The IPAM plugin can also be used by 3rd party software to manage IP addresses from Azure VNET space.

This page describes how to setup the CNM plugin manually on Azure IaaS VMs. If you are planning to deploy an ACS cluster, see [ACS](acs.md) instead.

## Install
Copy the plugin package from the [release](https://github.com/Azure/azure-container-networking/releases) share to your Azure VM, extract the contents and run the plugin in the background.

```bash
# Get the last version from https://github.com/Azure/azure-container-networking/releases
$ PLUGIN_VERSION="v1.x.x"
$ curl -OsSL https://github.com/Azure/azure-container-networking/releases/download/${PLUGIN_VERSION}/azure-vnet-cnm-linux-amd64-${PLUGIN_VERSION}.tgz
$ tar xzvf azure-vnet-cnm-linux-amd64-${PLUGIN_VERSION}.tgz
# Might require sudo if not running as root
$ ./azure-cnm-plugin&
```

The `azure-vnet` plugin also requires the ebtables package when running on Linux. This step is not required on Windows.

```bash
$ apt-get install -y ebtables
```

## Build
The plugin can also be built directly from the source code in this repository.

```bash
$ git clone https://github.com/Azure/azure-container-networking
$ cd azure-container-networking
$ make azure-cnm-plugin
```

This builds the plugin and generates a tar archive. The binaries are placed in the `output` directory.

## Usage
```bash
$ azure-cnm-plugin --help

Usage: azure-cnm-plugin [OPTIONS]

Options:
  -e, --environment=azure      Set the operating environment {azure,mas}
  -u, --api-url                Set the API server URL
  -l, --log-level=info         Set the logging level {info,debug}
  -t, --log-target=logfile     Set the logging target {syslog,stderr,logfile}
  -o, --log-location           Set the logging directory
  -q, --ipam-query-url         Set the IPAM query URL
  -i, --ipam-query-interval    Set the IPAM plugin query interval
  -v, --version                Print version information
  -h, --help                   Print usage information
```

## Examples
To connect your containers to other resources on your Azure VNET, you need to first create a Docker network. A network is a group of uniquely addressable endpoints that can communicate with each other. Pass the plugin name as both the network and IPAM plugin. You also need to specify an Azure VNET subnet for your network.

Create a network:

```bash
$ docker network create --driver=azure-vnet --ipam-driver=azure-vnet --subnet=[subnet] azure
```

When the command succeeds, it will return the network ID. Confirm that the network was created successfully:

```bash
$ docker network ls
NETWORK ID          NAME                DRIVER              SCOPE
3159b0528a83        azure               azure-vnet          local
515779dadc8a        bridge              bridge              local
ed6e704a74ef        host                host                local
b35e3b663cc1        none                null                local
```

Connect containers to your network by specifying the `--net` argument with your network's name when running them:

```bash
$ docker run -it --rm --net=azure ubuntu:latest /bin/bash
```

Finally, once all containers on the network exit, you can delete the network. 

```bash
$ docker network rm azure
```

## Outbound Connectivity from container
You have to add following iptable command to allow outbound(internet) connectivity from container
```bash
iptables -t nat -A POSTROUTING -m addrtype ! --dst-type local ! -d <vnet_address_space> -j MASQUERADE
```
