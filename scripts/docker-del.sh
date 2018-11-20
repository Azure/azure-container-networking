#!/bin/bash

# Delete a docker container with network namespace cleaned up by the Azure CNI plugin.
# Example usage: ./docker-run.sh ubuntu default

if [ $# -ne 2 ]; then
	echo "usage: docker-run.sh <container-name> <namespace>"
	exit 1
fi

pid=$(docker inspect -f '{{ .State.Pid }}' $1)
netnspath=/proc/$pid/ns/net
contid=$(docker inspect -f '{{ .Id }}' $1)
export CNI_CONTAINERID=$contid

netns=netnspath
export CNI_PATH='/opt/cni/bin'
export CNI_COMMAND='DEL'
export PATH=$CNI_PATH:$PATH
export CNI_NETNS=$netns
args=$(printf "K8S_POD_NAMESPACE=%s;K8S_POD_NAME=%s" $2 $1)
export CNI_ARGS=$args
export CNI_IFNAME='eth0'


config=$(jq '.plugins[0]' /etc/cni/net.d/10-azure.conflist)
name=$(jq -r '.name' /etc/cni/net.d/10-azure.conflist)
config=$(echo $config | jq --arg name $name '. + {name: $name}')
cniVersion=$(jq -r '.cniVersion' /etc/cni/net.d/10-azure.conflist)
config=$(echo $config | jq --arg cniVersion $cniVersion '. + {cniVersion: $cniVersion}')

res=$(echo $config | azure-vnet)


if [ $? -ne 0 ]; then
	errmsg=$(echo $res | jq -r '.msg')
	if [ -z "$errmsg" ]; then
		errmsg=$res
	fi
	echo "${name} : error executing $CNI_COMMAND: $errmsg"
	exit 1
elif [[ ${DEBUG} -gt 0 ]]; then
	echo ${res} | jq -r .
fi


docker rm -f $1
