#!/usr/bin/env bash

# Installs azure-vnet CNI plugins on a Linux node.

# Arguments.
PLUGIN_VERSION=$1
CNI_VERSION=$2
CNI_BIN_DIR=/opt/cni/bin
CNI_NETCONF_DIR=/etc/cni/net.d

function usage
{
    printf "Installs azure-vnet CNI plugins.\n"
    printf "See https://github.com/Azure/azure-container-networking/releases for versions\n"
    printf "Usage: install-cni-plugin version [pluginVersion]\n"
}

if [ "$PLUGIN_VERSION" = "" ]; then
    usage
    exit 1
fi

if [ "$CNI_VERSION" = "" ]; then
    CNI_VERSION=v1.1.1
fi

# Create CNI directories.
printf "Creating CNI directories.\n"
mkdir -p $CNI_BIN_DIR
mkdir -p $CNI_NETCONF_DIR

# Install ebtables.
if [ ! -e /sbin/ebtables ]
then
    printf "Installing ebtables package..."
    apt-get update
    apt-get install -y ebtables
    printf "done.\n"
else
    echo "Package ebtables is already installed."
fi
/sbin/ebtables --list > /dev/null

# Install azure-vnet CNI plugins.
printf "Installing azure-vnet CNI plugin version $PLUGIN_VERSION to $CNI_BIN_DIR..."
/usr/bin/curl -sSL https://github.com/Azure/azure-container-networking/releases/download/$PLUGIN_VERSION/azure-vnet-cni-linux-amd64-$PLUGIN_VERSION.tgz > $CNI_BIN_DIR/azure-vnet.tgz
tar -xzf $CNI_BIN_DIR/azure-vnet.tgz -C $CNI_BIN_DIR
printf "done.\n"

# Install azure-vnet CNI network configuration file.
printf "Installing azure-vnet CNI network configuration file to $CNI_NETCONF_DIR..."
mv $CNI_BIN_DIR/*.conflist $CNI_NETCONF_DIR
printf "done.\n"

# Install loopback plugin.
printf "Installing loopback CNI plugin version $CNI_VERSION to $CNI_BIN_DIR..."
/usr/bin/curl -sSL https://github.com/containernetworking/plugins/releases/download/$CNI_VERSION/cni-plugins-linux-amd64-$CNI_VERSION.tgz > $CNI_BIN_DIR/cni.tgz
tar -xzf $CNI_BIN_DIR/cni.tgz -C $CNI_BIN_DIR ./loopback
printf "done.\n"

# Cleanup.
rm $CNI_BIN_DIR/*.tgz
chown root:root $CNI_BIN_DIR/*

printf "azure-vnet CNI plugin is successfully installed.\n"
