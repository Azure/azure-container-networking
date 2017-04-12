#!/usr/bin/env bash

# Installs azure-vnet CNI plugins on a Linux node.

# Populate these fields.
PLUGIN_VERSION=v0.7
CNI_VERSION=v0.4.0
CNI_BIN_DIR=/opt/cni/bin
CNI_CONF_DIR=/etc/cni/net.d

# Create CNI directories.
mkdir -p ${CNI_BIN_DIR}
mkdir -p ${CNI_CONF_DIR}

# Install ebtables.
if [ ! -e /sbin/ebtables ]
then
    apt-get update
    apt-get install -y ebtables
fi
/sbin/ebtables --list

# Install Azure CNI plugins.
/usr/bin/curl -sSL https://github.com/Azure/azure-container-networking/releases/download/${PLUGIN_VERSION}/azure-vnet-cni-linux-amd64-${PLUGIN_VERSION}.tgz > ${CNI_BIN_DIR}/azure.tgz
tar -xzf ${CNI_BIN_DIR}/azure.tgz -C ${CNI_BIN_DIR}

# Install loopback plugin.
/usr/bin/curl -sSL https://github.com/containernetworking/cni/releases/download/${CNI_VERSION}/cni-amd64-${CNI_VERSION}.tgz > ${CNI_BIN_DIR}/cni.tgz
tar -xzf ${CNI_BIN_DIR}/cni.tgz -C ${CNI_BIN_DIR} ./loopback

# Cleanup.
rm ${CNI_BIN_DIR}/*.tgz
chown root:root ${CNI_BIN_DIR}/*
